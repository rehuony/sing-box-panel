// SPDX-License-Identifier: GPL-3.0-or-later

package fleet_test

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/fleet"
)

// pipeListener accepts in-memory connections from a chosen remote address.
type pipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
}

func (listener *pipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-listener.conns:
		return conn, nil
	case <-listener.closed:
		return nil, net.ErrClosed
	}
}

func (listener *pipeListener) Close() error { close(listener.closed); return nil }
func (listener *pipeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 3000}
}

type remoteConn struct {
	net.Conn
	remote net.Addr
}

func (conn remoteConn) RemoteAddr() net.Addr { return conn.remote }

func TestSplitListenerRoutesByFirstBytesAndPeer(t *testing.T) {
	listener := &pipeListener{conns: make(chan net.Conn), closed: make(chan struct{})}
	web, nodes := fleet.SplitListener(listener)
	defer func() {
		for _, closer := range []io.Closer{web, nodes.TLS, nodes.Cleartext} {
			_ = closer.Close()
		}
	}()
	open := func(remote string, opening string) net.Conn {
		client, server := net.Pipe()
		listener.conns <- remoteConn{Conn: server, remote: &net.TCPAddr{IP: net.ParseIP(remote), Port: 40000}}
		go func() { _, _ = io.WriteString(client, opening) }()
		return client
	}
	expect := func(target net.Listener, opening string) {
		t.Helper()
		accepted := make(chan net.Conn, 1)
		go func() {
			conn, err := target.Accept()
			if err == nil {
				accepted <- conn
			}
		}()
		select {
		case conn := <-accepted:
			buffer := make([]byte, len(opening))
			if _, err := io.ReadFull(conn, buffer); err != nil || string(buffer) != opening {
				t.Fatalf("replayed %q, %v; want %q", buffer, err, opening)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%q was not routed", opening)
		}
	}
	const preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"
	open("192.0.2.10", "POST /api/v1/nodes HTTP/1.1\r\n")
	expect(web, "POST /api/v1/nodes HTTP/1.1\r\n")
	open("192.0.2.10", "\x16\x03\x01")
	expect(nodes.TLS, "\x16\x03\x01")
	open("127.0.0.1", preface)
	expect(nodes.Cleartext, preface)

	client := open("192.0.2.10", preface)
	_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := client.Read(make([]byte, 1)); err != io.EOF && err != io.ErrClosedPipe {
		t.Fatalf("cleartext HTTP/2 from a remote peer was not closed: %v", err)
	}
}
