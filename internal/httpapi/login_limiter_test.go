// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestLoginClientNormalizesPeers(t *testing.T) {
	for input, want := range map[string]string{
		"192.0.2.1:123":                 "192.0.2.1",
		"[::ffff:192.0.2.1]:456":        "192.0.2.1",
		"[2001:db8:abcd:1::1]:123":      "2001:db8:abcd:1::/64",
		"[2001:db8:abcd:1:ffff::2]:456": "2001:db8:abcd:1::/64",
		"[fe80::1%en0]:123":             "fe80::/64",
		"invalid":                       "unknown",
	} {
		if got := loginClient(input); got != want {
			t.Errorf("%q: got %q want %q", input, got, want)
		}
	}
}

func TestLoginLimiterAtomicAdmission(t *testing.T) {
	limiter := newLoginLimiter()
	var admitted atomic.Int32
	var group sync.WaitGroup
	for range 100 {
		group.Go(func() {
			if ok, _ := limiter.begin("one-peer"); ok {
				admitted.Add(1)
				limiter.finish()
			}
		})
	}
	group.Wait()
	if n := admitted.Load(); n == 0 || n > loginAttemptLimit {
		t.Fatalf("admitted %d attempts", n)
	}
	for i := int(admitted.Load()); i < loginAttemptLimit; i++ {
		if ok, _ := limiter.begin("one-peer"); !ok {
			t.Fatal("concurrency rejections consumed the peer budget")
		}
		limiter.finish()
	}
	if ok, _ := limiter.begin("one-peer"); ok {
		t.Fatal("exceeded peer budget")
	}
	if ok, _ := limiter.begin("other-peer"); !ok {
		t.Fatal("work slot leaked")
	}
	limiter.finish()
}

func TestLoginLimiterGlobalBudgetAndExpiry(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Now()
	limiter.now = func() time.Time { return now }
	for i := range loginGlobalLimit {
		if ok, _ := limiter.begin(fmt.Sprint(i)); !ok {
			t.Fatalf("rejected peer %d", i)
		}
		limiter.finish()
	}
	if ok, retry := limiter.begin("fresh-peer"); ok || retry != time.Minute {
		t.Fatalf("global limit: %v %v", ok, retry)
	}
	now = now.Add(time.Minute)
	if ok, _ := limiter.begin("fresh-peer"); !ok {
		t.Fatal("expired global budget did not recover")
	}
	limiter.finish()
}

func TestLoginLimiterDoesNotEvictActiveKeys(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Now()
	limiter.now = func() time.Time { return now }
	for i := range maxLoginClients {
		if ok, _ := limiter.fingerprint(fmt.Sprint(i)); !ok {
			t.Fatalf("rejected key %d", i)
		}
	}
	if ok, _ := limiter.fingerprint("new-key"); ok {
		t.Fatal("accepted a key past capacity")
	}
	if ok, _ := limiter.fingerprint("0"); !ok {
		t.Fatal("lost existing key")
	}
	if len(limiter.entries) != maxLoginClients {
		t.Fatal("live entries were evicted")
	}
	now = now.Add(time.Minute)
	if ok, _ := limiter.fingerprint("new-key"); !ok {
		t.Fatal("expired entries were not cleaned")
	}
}

func TestLoginFingerprintAndPeerBudgetsCannotReplaceEachOther(t *testing.T) {
	for _, mode := range []string{"rotate-peer", "rotate-fingerprint", "missing-fingerprint", "rotate-ipv6"} {
		t.Run(mode, func(t *testing.T) {
			handler := testHandler(t)
			for i := range loginAttemptLimit + 1 {
				request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader("{"))
				request.Header.Set("Origin", "http://example.com")
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(loginFingerprintHeader, strings.Repeat("a", 32))
				request.RemoteAddr = "192.0.2.1:1234"
				switch mode {
				case "rotate-peer":
					request.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", i+1)
				case "rotate-fingerprint":
					request.Header.Set(loginFingerprintHeader, fmt.Sprintf("%032x", i))
				case "missing-fingerprint":
					request.Header.Del(loginFingerprintHeader)
				case "rotate-ipv6":
					request.RemoteAddr = fmt.Sprintf("[2001:db8::%x]:1234", i+1)
					request.Header.Del(loginFingerprintHeader)
				}
				request.Header.Set("Forwarded", fmt.Sprintf("for=198.51.100.%d", i+1))
				request.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				want := http.StatusBadRequest
				if i == loginAttemptLimit {
					want = http.StatusTooManyRequests
				}
				if response.Code != want {
					t.Fatalf("attempt %d: status=%d want=%d", i, response.Code, want)
				}
			}
		})
	}
}

func TestLoginRejectsInvalidFingerprintsBeforeReadingBody(t *testing.T) {
	for _, values := range [][]string{{""}, {"arbitrary"}, {strings.Repeat("A", 32)}, {strings.Repeat("a", 32), strings.Repeat("a", 32)}} {
		t.Run(fmt.Sprint(values), func(t *testing.T) {
			handler := testHandler(t)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", nil)
			request.Header[loginFingerprintHeader] = values
			request.Body = unreadLoginBody{t}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d", response.Code)
			}
		})
	}
}

type unreadLoginBody struct{ t *testing.T }

func (b unreadLoginBody) Read([]byte) (int, error) {
	b.t.Error("rejected request body was read")
	return 0, io.EOF
}
func (unreadLoginBody) Close() error { return nil }

type blockedLoginBody struct {
	entered chan struct{}
	release <-chan struct{}
}

func (b blockedLoginBody) Read([]byte) (int, error) { close(b.entered); <-b.release; return 0, io.EOF }
func (blockedLoginBody) Close() error               { return nil }

func TestConcurrentLoginsRejectBeforeReadingBody(t *testing.T) {
	handler := testHandler(t)
	release := make(chan struct{})
	var group sync.WaitGroup
	// Cleanup releases blocked readers even when an assertion fails.
	defer group.Wait()
	defer close(release)
	for range maxConcurrentLogins {
		entered := make(chan struct{})
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", nil)
		request.Header.Set("Origin", "http://example.com")
		request.Header.Set("Content-Type", "application/json")
		request.Body = blockedLoginBody{entered, release}
		group.Go(func() { handler.ServeHTTP(httptest.NewRecorder(), request) })
		<-entered
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", nil)
	request.RemoteAddr = "192.0.2.200:1234"
	request.Body = unreadLoginBody{t}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "1" {
		t.Fatalf("concurrent login: %d %s", response.Code, response.Body.String())
	}
}

func TestSlowLoginResponsesReleaseSlotsAndBoundRejections(t *testing.T) {
	handler := testHandler(t)
	synctest.Test(t, func(t *testing.T) {
		var peers []net.Conn
		var group sync.WaitGroup
		defer func() {
			for _, peer := range peers {
				_ = peer.Close()
			}
			group.Wait()
		}()
		var completed []chan struct{}
		var responses []*blockedLoginResponse
		for attempt := range maxConcurrentLogins + 1 {
			writer, reader := net.Pipe()
			peers = append(peers, writer, reader)
			response := &blockedLoginResponse{ResponseRecorder: httptest.NewRecorder(), Conn: writer, entered: make(chan struct{})}
			responses = append(responses, response)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader("{"))
			request.Header.Set("Origin", "http://example.com")
			request.Header.Set("Content-Type", "application/json")
			done := make(chan struct{})
			completed = append(completed, done)
			group.Go(func() { handler.ServeHTTP(response, request); close(done) })
			// Each client stops reading even a small response. The third response
			// is rejected before admission and must still have a write deadline.
			<-response.entered
			want := http.StatusBadRequest
			if attempt == maxConcurrentLogins {
				want = http.StatusTooManyRequests
			}
			if response.Code != want {
				t.Fatalf("attempt %d: status=%d want=%d", attempt, response.Code, want)
			}
		}
		time.Sleep(loginTimeout)
		synctest.Wait()
		for index, done := range completed {
			select {
			case <-done:
				if !errors.Is(responses[index].writeError, os.ErrDeadlineExceeded) {
					t.Errorf("response %d: write error=%v, want deadline exceeded", index, responses[index].writeError)
				}
			default:
				t.Errorf("response %d remained blocked beyond the login deadline", index)
			}
		}
	})
	response := loginWithPassword(t, handler, "test-administrator-password")
	if response.Code != http.StatusOK {
		t.Fatalf("login after stalled responses: status=%d", response.Code)
	}
}

// net.Pipe supplies real deadline behavior without relying on OS send-buffer
// sizes to create backpressure. No client reads the response body.
type blockedLoginResponse struct {
	*httptest.ResponseRecorder
	net.Conn
	entered    chan struct{}
	writeError error
}

func (response *blockedLoginResponse) Write(data []byte) (int, error) {
	close(response.entered)
	written, err := response.Conn.Write(data)
	response.writeError = err
	return written, err
}

func TestLoginRejectsUnreadNetworkBodiesPromptly(t *testing.T) {
	for _, reason := range []string{"rate", "fingerprint", "size", "origin"} {
		t.Run(reason, func(t *testing.T) {
			handler := testHandler(t)
			if reason == "rate" {
				for range loginAttemptLimit {
					if ok, _ := handler.logins.begin("127.0.0.1"); !ok {
						t.Fatal("fixture admission failed")
					}
					handler.logins.finish()
				}
			}
			server := httptest.NewServer(handler)
			defer server.Close()
			conn, err := net.Dial("tcp", server.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			origin := server.URL
			extra := ""
			length := 100
			want := http.StatusTooManyRequests
			switch reason {
			case "fingerprint":
				extra = "X-Client-Fingerprint: invalid\r\n"
				want = http.StatusBadRequest
			case "size":
				length = maxLoginBody + 1
				want = http.StatusBadRequest
			case "origin":
				origin = "https://untrusted.example"
				want = http.StatusForbidden
			}
			_, err = fmt.Fprintf(conn, "POST /api/v1/auth/session HTTP/1.1\r\nHost: %s\r\nOrigin: %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\n%s\r\n", server.Listener.Addr(), origin, length, extra)
			if err != nil {
				t.Fatal(err)
			}
			// Deliberately send no body: even a small declared body must not be drained.
			response, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != want || !response.Close {
				t.Fatalf("response=%d close=%v", response.StatusCode, response.Close)
			}
			if _, err := io.ReadAll(response.Body); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSlowLoginBodyTimesOutAndReleasesSlot(t *testing.T) {
	handler := testHandler(t)
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		handler.ServeHTTP(w, request)
		close(finished)
	}))
	defer server.Close()
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(loginTimeout + 3*time.Second))
	_, err = fmt.Fprintf(conn, "POST /api/v1/auth/session HTTP/1.1\r\nHost: %s\r\nOrigin: %s\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{", server.Listener.Addr(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	// The shared read/write budget may close the transport before a timeout
	// response can be sent. A client-side watchdog timeout must still fail.
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if response != nil {
		defer response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status=%d", response.StatusCode)
		}
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("timed-out login handler did not finish")
	}
	handler.logins.mu.Lock()
	defer handler.logins.mu.Unlock()
	if handler.logins.inFlight != 0 {
		t.Fatal("timed-out login retained its slot")
	}
}

func TestLoginSupportsHTTP2StreamDeadlines(t *testing.T) {
	handler := testHandler(t)
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	for index, password := range []string{"test-administrator-password", "wrong"} {
		if index > 0 {
			// A completed stream's expired deadline must not close the shared
			// connection or prevent the next login from using it.
			<-time.After(loginTimeout + 50*time.Millisecond)
		}
		request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/auth/session", strings.NewReader(fmt.Sprintf(`{"email":"admin@example.com","password":%q}`, password)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", server.URL)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(loginFingerprintHeader, strings.Repeat("a", 32))
		var reused bool
		request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused },
		}))
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		want := http.StatusOK
		if password == "wrong" {
			want = http.StatusUnauthorized
		}
		if response.ProtoMajor != 2 || response.StatusCode != want {
			t.Fatalf("protocol=%s status=%d", response.Proto, response.StatusCode)
		}
		if index > 0 && !reused {
			t.Fatal("HTTP/2 login did not reuse the connection after the previous deadline")
		}
	}
}
