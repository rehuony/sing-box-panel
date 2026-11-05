// SPDX-License-Identifier: GPL-3.0-or-later

package webauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

const (
	// LoginTimeout bounds a login from admission to its response.
	LoginTimeout = 5 * time.Second
	// LoginFingerprintHeader carries an optional client fingerprint, which
	// adds a budget and never replaces the peer's.
	LoginFingerprintHeader = "X-Client-Fingerprint"
)

var loginFingerprintPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Rejection is why a login was refused before its credentials were checked.
type Rejection struct {
	Status int
	Code   string
	Title  string
	Detail string
	// RetryAfter, when positive, is how long the client should wait.
	RetryAfter time.Duration
}

// SetRetryAfter sets the Retry-After header of a rate-limited rejection.
func (rejection *Rejection) SetRetryAfter(header http.Header) {
	if rejection.RetryAfter > 0 {
		header.Set("Retry-After", strconv.Itoa(max(1, int((rejection.RetryAfter+time.Second-1)/time.Second))))
	}
}

// Login is one admitted login request.
type Login struct {
	// Request carries the login deadline.
	Request *http.Request
	// Body is the login payload, read only after admission.
	Body   []byte
	finish []func()
}

// Finish releases the login's work slot and connection. Call it once the
// response is written, whether or not the login was admitted.
func (login *Login) Finish() {
	for index := len(login.finish) - 1; index >= 0; index-- {
		login.finish[index]()
	}
}

// RateLimited is the rejection of a login over its budget.
func RateLimited(retryAfter time.Duration) *Rejection {
	return &Rejection{
		Status: http.StatusTooManyRequests, Code: "login_rate_limited", Title: "Too many attempts",
		Detail: "Wait before trying to authenticate again.", RetryAfter: retryAfter,
	}
}

// Unavailable is the rejection of a login the server cannot process.
func Unavailable() *Rejection {
	return &Rejection{
		Status: http.StatusServiceUnavailable, Code: "authentication_unavailable", Title: "Authentication unavailable",
		Detail: "The authentication service is temporarily unavailable.",
	}
}

// Admit bounds one login before its credentials are checked: it charges the
// peer's and the fingerprint's budgets, reserves a work slot, requires a
// same-origin JSON request from trustedOrigin (see SameOrigin), and reads at
// most maxBody bytes under LoginTimeout. The returned Login is never nil;
// call its Finish after responding.
func (limiter *LoginLimiter) Admit(w http.ResponseWriter, request *http.Request, trustedOrigin string, maxBody int64) (*Login, *Rejection) {
	login := &Login{Request: request}
	controller := http.NewResponseController(w)
	// A rejected HTTP/1 login must not keep the connection busy draining an
	// attacker-controlled body after the handler has released its work slot.
	if request.ProtoMajor == 1 {
		w.Header().Set("Connection", "close")
		request.Close = true
		login.finish = append(login.finish, func() {
			_ = controller.SetReadDeadline(time.Now())
			_ = request.Body.Close()
		})
	}
	// Bound response writes before admission so rejected logins are protected
	// too. Keep the deadline armed for the server's post-handler buffered flush.
	ctx, cancel := context.WithTimeout(request.Context(), LoginTimeout)
	login.finish = append(login.finish, cancel)
	login.Request = request.WithContext(ctx)
	deadline, _ := ctx.Deadline()
	if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return login, Unavailable()
	}
	if allowed, retryAfter := limiter.Begin(LoginClient(request.RemoteAddr)); !allowed {
		return login, RateLimited(retryAfter)
	}
	login.finish = append(login.finish, limiter.Finish)
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return login, Unavailable()
	}
	fingerprint := request.Header.Get(LoginFingerprintHeader)
	if values := request.Header.Values(LoginFingerprintHeader); len(values) > 1 || (len(values) == 1 && !loginFingerprintPattern.MatchString(fingerprint)) {
		return login, &Rejection{Status: http.StatusBadRequest, Code: "invalid_login", Title: "Invalid login", Detail: "The login fingerprint is invalid."}
	}
	if allowed, retryAfter := limiter.Fingerprint(fingerprint); !allowed {
		return login, RateLimited(retryAfter)
	}
	if !SameOrigin(request, trustedOrigin) || request.Header.Get("Content-Type") != "application/json" {
		return login, &Rejection{Status: http.StatusForbidden, Code: "csrf_failed", Title: "Request rejected", Detail: "A same-origin JSON login request is required."}
	}
	invalidBody := &Rejection{Status: http.StatusBadRequest, Code: "invalid_body", Title: "Invalid request", Detail: "The login payload is invalid."}
	if request.ContentLength > maxBody {
		return login, invalidBody
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBody+1))
	_ = request.Body.Close()
	if err != nil || int64(len(body)) > maxBody {
		return login, invalidBody
	}
	login.Body = body
	return login, nil
}
