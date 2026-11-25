// SPDX-License-Identifier: GPL-3.0-or-later

package nodeapi

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
)

// The endpoints of the node protocol sit under the panel's base path beside
// its Web application, so one reverse proxy location serves both. Each
// endpoint is a WebSocket whose binary messages are its frames, one
// protobuf message each.
const (
	// PathPrefix leads every endpoint path.
	PathPrefix = "/api/node/v1"
	// EnrollPath exchanges one EnrollRequest for one EnrollResponse: a
	// one-time enrollment token and the node's public key for a node
	// identity.
	EnrollPath = PathPrefix + "/enroll"
	// ConnectPath carries the control session: ConnectRequest frames from
	// the node and ConnectResponse frames from the panel. The node sends
	// Hello and Proof, the panel sends Challenge and Welcome, and only then
	// do commands, results and events flow. A new authenticated session for
	// a node replaces the old one.
	ConnectPath = PathPrefix + "/connect"
	// ArtifactPath streams one cached core artifact: the node sends one
	// FetchArtifactRequest, authorized by a grant issued in its current
	// session, and the panel answers with FetchArtifactResponse frames.
	ArtifactPath = PathPrefix + "/artifact"
	// ReleasePath streams the panel's node release for the node's platform
	// with its own signed handshake, apart from the session, so a running
	// node and the node's update command can both use it: the node sends
	// Hello and Proof, the panel sends Challenge, then ReleaseHeader and the
	// binary in chunks.
	ReleasePath = PathPrefix + "/release"
)

const (
	// MaximumFrameBytes bounds one frame in either direction.
	MaximumFrameBytes = 4 << 20
	// KeepaliveInterval and KeepaliveTimeout detect a control session whose
	// peer stopped answering. Each side pings the other end to end, through
	// any reverse proxy between them.
	KeepaliveInterval = 30 * time.Second
	KeepaliveTimeout  = 10 * time.Second
	// writeTimeout bounds sending one frame.
	writeTimeout = time.Minute
	// dialTimeout bounds opening an endpoint.
	dialTimeout = 15 * time.Second
	// closeStatusBase is the WebSocket close status below the first Code: a
	// Code travels as closeStatusBase plus its value.
	closeStatusBase = 4000
	// maximumCloseReason is the longest reason a close frame carries.
	maximumCloseReason = 123
)

// Code classifies why one side ended an exchange. It travels as the
// WebSocket close status closeStatusBase plus its value.
type Code int

const (
	CodeInvalidArgument Code = 1 + iota
	CodeUnauthenticated
	CodePermissionDenied
	CodeNotFound
	CodeFailedPrecondition
	CodeAborted
	CodeDeadlineExceeded
	CodeUnavailable
	CodeInternal
)

var codeNames = [...]string{
	CodeInvalidArgument: "invalid_argument", CodeUnauthenticated: "unauthenticated", CodePermissionDenied: "permission_denied",
	CodeNotFound: "not_found", CodeFailedPrecondition: "failed_precondition", CodeAborted: "aborted",
	CodeDeadlineExceeded: "deadline_exceeded", CodeUnavailable: "unavailable", CodeInternal: "internal",
}

func (code Code) String() string {
	if code > 0 && int(code) < len(codeNames) {
		return codeNames[code]
	}
	return fmt.Sprintf("code_%d", int(code))
}

// Error is a failure one side reported to the other in the close frame that
// ended the exchange.
type Error struct {
	Code    Code
	Message string
}

func (err *Error) Error() string { return err.Code.String() + ": " + err.Message }

// Errorf returns an Error with code and a formatted message.
func Errorf(code Code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ErrorCode returns the Code of the Error in err's chain, or 0 without one.
func ErrorCode(err error) Code {
	var failure *Error
	if errors.As(err, &failure) {
		return failure.Code
	}
	return 0
}

// Stream exchanges one endpoint's frames over a WebSocket: it sends Out
// frames and receives In frames. Send may be called from several goroutines
// at once, Recv from one at a time. Recv returns io.EOF once the peer closed
// the stream normally and an Error once it closed it with a failure.
type Stream[Out, In proto.Message] struct {
	conn *websocket.Conn
	// ctx ends when the stream closes; reads and writes use it, so they
	// never close the connection on behalf of a caller's context.
	ctx    context.Context
	cancel context.CancelFunc
	// failure, when set, is why the stream failed on its own, such as a
	// keepalive ping without an answer; Recv returns it in place of the
	// connection error.
	failure atomic.Pointer[error]
	sendMu  sync.Mutex
}

// The stream of each endpoint, from the panel's side and from the node's.
type (
	EnrollServer   = Stream[*EnrollResponse, *EnrollRequest]
	ConnectServer  = Stream[*ConnectResponse, *ConnectRequest]
	ConnectClient  = Stream[*ConnectRequest, *ConnectResponse]
	ArtifactServer = Stream[*FetchArtifactResponse, *FetchArtifactRequest]
	ArtifactClient = Stream[*FetchArtifactRequest, *FetchArtifactResponse]
	ReleaseServer  = Stream[*FetchReleaseResponse, *FetchReleaseRequest]
	ReleaseClient  = Stream[*FetchReleaseRequest, *FetchReleaseResponse]
)

func newStream[Out, In proto.Message](conn *websocket.Conn) *Stream[Out, In] {
	conn.SetReadLimit(MaximumFrameBytes)
	ctx, cancel := context.WithCancel(context.Background())
	return &Stream[Out, In]{conn: conn, ctx: ctx, cancel: cancel}
}

// Context ends when the stream closes.
func (stream *Stream[Out, In]) Context() context.Context { return stream.ctx }

// Send writes one frame.
func (stream *Stream[Out, In]) Send(frame Out) error {
	data, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(stream.ctx, writeTimeout)
	defer cancel()
	stream.sendMu.Lock()
	defer stream.sendMu.Unlock()
	if err := stream.conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		return stream.connectionError(err)
	}
	return nil
}

// Recv waits for the next frame.
func (stream *Stream[Out, In]) Recv() (In, error) { return stream.recv(stream.ctx) }

// RecvWithin waits at most timeout for the next frame; a frame that does not
// arrive in time fails the stream.
func (stream *Stream[Out, In]) RecvWithin(timeout time.Duration) (In, error) {
	ctx, cancel := context.WithTimeout(stream.ctx, timeout)
	defer cancel()
	frame, err := stream.recv(ctx)
	if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) && stream.failure.Load() == nil {
		return frame, Errorf(CodeDeadlineExceeded, "no frame arrived within %s", timeout)
	}
	return frame, err
}

func (stream *Stream[Out, In]) recv(ctx context.Context) (In, error) {
	var none In
	kind, data, err := stream.conn.Read(ctx)
	if err != nil {
		return none, stream.connectionError(err)
	}
	if kind != websocket.MessageBinary {
		return none, Errorf(CodeInvalidArgument, "frames must be binary messages")
	}
	frame := none.ProtoReflect().New().Interface().(In)
	if err := proto.Unmarshal(data, frame); err != nil {
		return none, Errorf(CodeInvalidArgument, "a frame could not be decoded")
	}
	return frame, nil
}

// KeepAlive pings the peer every KeepaliveInterval until the stream closes
// and fails the stream when a ping gets no answer within KeepaliveTimeout.
// Answers arrive through Recv, so the stream needs a reader meanwhile.
func (stream *Stream[Out, In]) KeepAlive() {
	go func() {
		ticker := time.NewTicker(KeepaliveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stream.ctx.Done():
				return
			case <-ticker.C:
			}
			ctx, cancel := context.WithTimeout(stream.ctx, KeepaliveTimeout)
			err := stream.conn.Ping(ctx)
			cancel()
			if err == nil {
				continue
			}
			if stream.ctx.Err() == nil {
				failure := Errorf(CodeUnavailable, "the peer did not answer a keepalive ping within %s", KeepaliveTimeout)
				stream.failure.CompareAndSwap(nil, &failure)
				_ = stream.conn.CloseNow()
			}
			return
		}
	}()
}

// Close ends the stream and tells the peer why: nil closes it normally, an
// Error carries its code and message, and any other error is reported as
// an internal failure. Later calls do nothing.
func (stream *Stream[Out, In]) Close(err error) error {
	defer stream.cancel()
	status, reason := closeStatus(err)
	return stream.conn.Close(status, reason)
}

func (stream *Stream[Out, In]) connectionError(err error) error {
	if failure := stream.failure.Load(); failure != nil {
		return *failure
	}
	var closed websocket.CloseError
	if errors.As(err, &closed) {
		code := int(closed.Code)
		switch {
		case closed.Code == websocket.StatusNormalClosure:
			return io.EOF
		case code > closeStatusBase && code < closeStatusBase+len(codeNames):
			return &Error{Code: Code(code - closeStatusBase), Message: closed.Reason}
		}
	}
	return err
}

func closeStatus(err error) (websocket.StatusCode, string) {
	var failure *Error
	switch {
	case err == nil:
		return websocket.StatusNormalClosure, ""
	case errors.As(err, &failure):
	case errors.Is(err, context.DeadlineExceeded):
		failure = &Error{Code: CodeDeadlineExceeded, Message: "the exchange timed out"}
	case errors.Is(err, context.Canceled):
		failure = &Error{Code: CodeUnavailable, Message: "the exchange was canceled"}
	default:
		failure = &Error{Code: CodeInternal, Message: "the exchange failed"}
	}
	reason := failure.Message
	for len(reason) > maximumCloseReason {
		_, size := utf8.DecodeLastRuneInString(reason)
		reason = reason[:len(reason)-size]
	}
	return websocket.StatusCode(closeStatusBase + int(failure.Code)), reason
}

// Accept opens the stream a request asks for. It answers a request that is
// not a WebSocket handshake itself, and refuses one a browser sends from
// another origin. The stream lives until it is closed, whatever happens to
// the request's context.
func Accept[Out, In proto.Message](w http.ResponseWriter, request *http.Request) (*Stream[Out, In], error) {
	conn, err := websocket.Accept(w, request, nil)
	if err != nil {
		return nil, err
	}
	return newStream[Out, In](conn), nil
}

// Client opens the endpoints of the panel at a base URL.
type Client struct {
	base string
	http *http.Client
}

// NewClient returns a client of the panel at panelURL, which ValidPanelURL
// accepts. HTTPS is verified with the system's trusted certificate
// authorities.
func NewClient(panelURL string) (*Client, error) {
	if err := ValidPanelURL(panelURL); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13}
	return &Client{base: panelURL, http: &http.Client{Transport: transport}}, nil
}

// Close releases the client's idle connections.
func (client *Client) Close() { client.http.CloseIdleConnections() }

// Enroll exchanges request for the node's identity.
func (client *Client) Enroll(ctx context.Context, request *EnrollRequest) (*EnrollResponse, error) {
	stream, err := dial[*EnrollRequest, *EnrollResponse](ctx, client, EnrollPath)
	if err != nil {
		return nil, err
	}
	defer stream.Close(nil)
	if err := stream.Send(request); err != nil {
		return nil, err
	}
	return stream.Recv()
}

// Connect opens the control session. It closes normally when ctx ends.
func (client *Client) Connect(ctx context.Context) (*ConnectClient, error) {
	return dial[*ConnectRequest, *ConnectResponse](ctx, client, ConnectPath)
}

// FetchArtifact opens the stream of one cached core artifact.
func (client *Client) FetchArtifact(ctx context.Context, request *FetchArtifactRequest) (*ArtifactClient, error) {
	stream, err := dial[*FetchArtifactRequest, *FetchArtifactResponse](ctx, client, ArtifactPath)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(request); err != nil {
		_ = stream.Close(nil)
		return nil, err
	}
	return stream, nil
}

// FetchRelease opens the stream of the panel's node release.
func (client *Client) FetchRelease(ctx context.Context) (*ReleaseClient, error) {
	return dial[*FetchReleaseRequest, *FetchReleaseResponse](ctx, client, ReleasePath)
}

// dial opens the endpoint at path. The stream closes normally when ctx
// ends.
func dial[Out, In proto.Message](ctx context.Context, client *Client, path string) (*Stream[Out, In], error) {
	opening, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	conn, response, err := websocket.Dial(opening, client.base+path, &websocket.DialOptions{HTTPClient: client.http})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, dialError(response, err)
	}
	stream := newStream[Out, In](conn)
	go func() {
		select {
		case <-ctx.Done():
			_ = stream.Close(nil)
		case <-stream.ctx.Done():
		}
	}()
	return stream, nil
}

// dialError describes a handshake the panel or a proxy before it refused,
// with the start of its answer.
func dialError(response *http.Response, err error) error {
	if response == nil || response.StatusCode == http.StatusSwitchingProtocols {
		return err
	}
	detail := ""
	if response.Body != nil {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 256))
		detail = strings.Join(strings.Fields(string(body)), " ")
	}
	if detail == "" {
		return fmt.Errorf("the panel answered HTTP %d instead of opening a WebSocket", response.StatusCode)
	}
	return fmt.Errorf("the panel answered HTTP %d instead of opening a WebSocket: %s", response.StatusCode, detail)
}
