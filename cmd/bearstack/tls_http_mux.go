package main

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const tlsHandshakeRecordType byte = 0x16
const muxFirstByteReadTimeout = 5 * time.Second

type tlsHTTPMux struct {
	base         net.Listener
	tlsListener  *connQueueListener
	httpListener *connQueueListener
	closeOnce    sync.Once
	closeErr     error
}

func newTLSHTTPMux(addr string) (*tlsHTTPMux, error) {
	base, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := &tlsHTTPMux{base: base}
	mux.tlsListener = newConnQueueListener(base.Addr(), mux.Close)
	mux.httpListener = newConnQueueListener(base.Addr(), mux.Close)
	go mux.acceptLoop()
	return mux, nil
}

func (m *tlsHTTPMux) TLSListener() net.Listener {
	return m.tlsListener
}

func (m *tlsHTTPMux) HTTPListener() net.Listener {
	return m.httpListener
}

func (m *tlsHTTPMux) Close() error {
	m.closeOnce.Do(func() {
		if err := m.base.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			m.closeErr = err
		}
		m.tlsListener.shutdown()
		m.httpListener.shutdown()
	})
	return m.closeErr
}

func (m *tlsHTTPMux) acceptLoop() {
	for {
		conn, err := m.base.Accept()
		if err != nil {
			_ = m.Close()
			return
		}
		go m.routeConn(conn)
	}
}

func (m *tlsHTTPMux) routeConn(conn net.Conn) {
	if err := conn.SetReadDeadline(time.Now().Add(muxFirstByteReadTimeout)); err != nil {
		_ = conn.Close()
		return
	}

	var first [1]byte
	if _, err := io.ReadFull(conn, first[:]); err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	wrapped := &firstByteConn{
		Conn:      conn,
		firstByte: first[0],
		pending:   true,
	}
	if first[0] == tlsHandshakeRecordType {
		if !m.tlsListener.enqueue(wrapped) {
			_ = wrapped.Close()
		}
		return
	}
	if !m.httpListener.enqueue(wrapped) {
		_ = wrapped.Close()
	}
}

type connQueueListener struct {
	addr    net.Addr
	conns   chan net.Conn
	closeFn func() error
	done    chan struct{}
	once    sync.Once
}

func newConnQueueListener(addr net.Addr, closeFn func() error) *connQueueListener {
	return &connQueueListener{
		addr:    addr,
		conns:   make(chan net.Conn),
		closeFn: closeFn,
		done:    make(chan struct{}),
	}
}

func (l *connQueueListener) Accept() (net.Conn, error) {
	select {
	case conn, ok := <-l.conns:
		if !ok {
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *connQueueListener) Close() error {
	if l.closeFn != nil {
		return l.closeFn()
	}
	l.shutdown()
	return nil
}

func (l *connQueueListener) Addr() net.Addr {
	return l.addr
}

func (l *connQueueListener) shutdown() {
	l.once.Do(func() {
		close(l.done)
		close(l.conns)
	})
}

func (l *connQueueListener) enqueue(conn net.Conn) bool {
	select {
	case <-l.done:
		return false
	case l.conns <- conn:
		return true
	}
}

type firstByteConn struct {
	net.Conn
	firstByte byte
	pending   bool
}

func (c *firstByteConn) Read(p []byte) (int, error) {
	if !c.pending {
		return c.Conn.Read(p)
	}
	if len(p) == 0 {
		return 0, nil
	}
	p[0] = c.firstByte
	c.pending = false
	if len(p) == 1 {
		return 1, nil
	}
	n, err := c.Conn.Read(p[1:])
	return n + 1, err
}
