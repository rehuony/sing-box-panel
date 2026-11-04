// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"bufio"
	"errors"
	"net"
	"sync"
	"time"
)

// classifyTimeout bounds how long a new connection may stay silent before
// its first bytes decide where it goes.
const classifyTimeout = 10 * time.Second

// NodeListeners carry the node connections split off the Web listener.
type NodeListeners struct {
	// TLS carries connections that open with a TLS handshake: nodes that
	// dial the panel directly and pin its certificate.
	TLS net.Listener
	// Cleartext carries cleartext HTTP/2 from loopback: a reverse proxy on
	// this host that terminates TLS for nodes and forwards with grpc_pass.
	Cleartext net.Listener
}

// SplitListener shares listener between the Web server and node sessions.
// Each connection goes by its first bytes: a TLS handshake to nodes.TLS, the
// HTTP/2 cleartext preface to nodes.Cleartext when the peer is on loopback,
// and anything else to web. Cleartext HTTP/2 from other peers is closed, so
// node traffic never crosses a network unencrypted. The shared listener
// closes once web and both node listeners are closed.
func SplitListener(listener net.Listener) (web net.Listener, nodes NodeListeners) {
	shared := &sharedListener{listener: listener, open: 3}
	webListener, tlsListener, cleartextListener := shared.virtual(), shared.virtual(), shared.virtual()
	go shared.accept(webListener, tlsListener, cleartextListener)
	return webListener, NodeListeners{TLS: tlsListener, Cleartext: cleartextListener}
}

type sharedListener struct {
	listener net.Listener
	mu       sync.Mutex
	open     int
}

func (shared *sharedListener) virtual() *virtualListener {
	return &virtualListener{shared: shared, conns: make(chan net.Conn), done: make(chan struct{})}
}

func (shared *sharedListener) accept(web, tls, cleartext *virtualListener) {
	backoff := 5 * time.Millisecond
	for {
		conn, err := shared.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			for _, target := range []*virtualListener{web, tls, cleartext} {
				target.shut()
			}
			return
		}
		if err != nil {
			// Temporary failures such as running out of file descriptors.
			time.Sleep(backoff)
			backoff = min(2*backoff, time.Second)
			continue
		}
		backoff = 5 * time.Millisecond
		go classify(conn, web, tls, cleartext)
	}
}

func (shared *sharedListener) release() {
	shared.mu.Lock()
	defer shared.mu.Unlock()
	shared.open--
	if shared.open == 0 {
		_ = shared.listener.Close()
	}
}

func classify(conn net.Conn, web, tls, cleartext *virtualListener) {
	_ = conn.SetReadDeadline(time.Now().Add(classifyTimeout))
	reader := bufio.NewReader(conn)
	first, err := reader.Peek(1)
	if err != nil {
		_ = conn.Close()
		return
	}
	target := web
	switch first[0] {
	case 0x16: // TLS handshake record
		target = tls
	case 'P': // PRI (the HTTP/2 preface), PUT, POST or PATCH
		prefix, err := reader.Peek(4)
		if err != nil {
			_ = conn.Close()
			return
		}
		if string(prefix) == "PRI " {
			if !loopback(conn.RemoteAddr()) {
				_ = conn.Close()
				return
			}
			target = cleartext
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	target.deliver(&peekedConn{Conn: conn, reader: reader})
}

func loopback(address net.Addr) bool {
	tcp, ok := address.(*net.TCPAddr)
	return ok && tcp.IP.IsLoopback()
}

// virtualListener hands out the connections classified for one server.
type virtualListener struct {
	shared *sharedListener
	conns  chan net.Conn
	done   chan struct{}
	once   sync.Once
}

func (listener *virtualListener) Accept() (net.Conn, error) {
	select {
	case conn := <-listener.conns:
		return conn, nil
	case <-listener.done:
		return nil, net.ErrClosed
	}
}

func (listener *virtualListener) Close() error {
	listener.once.Do(func() {
		close(listener.done)
		listener.shared.release()
	})
	return nil
}

// shut ends the listener after the shared listener closed.
func (listener *virtualListener) shut() {
	listener.once.Do(func() { close(listener.done) })
}

func (listener *virtualListener) Addr() net.Addr { return listener.shared.listener.Addr() }

func (listener *virtualListener) deliver(conn net.Conn) {
	select {
	case listener.conns <- conn:
	case <-listener.done:
		_ = conn.Close()
	}
}

// peekedConn replays the bytes read while classifying the connection.
type peekedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (conn *peekedConn) Read(data []byte) (int, error) { return conn.reader.Read(data) }
