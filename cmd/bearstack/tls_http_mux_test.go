package main

import (
	"net"
	"testing"
	"time"
)

func TestTLSHTTPMuxRoutesPlainHTTP(t *testing.T) {
	mux, err := newTLSHTTPMux("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer mux.Close()

	httpConnCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, acceptErr := mux.HTTPListener().Accept()
		if acceptErr != nil {
			errCh <- acceptErr
			return
		}
		httpConnCh <- conn
	}()

	client, err := net.Dial("tcp", mux.TLSListener().Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("accept failed: %v", err)
	case conn := <-httpConnCh:
		defer conn.Close()
		buf := make([]byte, 1)
		if _, err := conn.Read(buf); err != nil {
			t.Fatal(err)
		}
		if buf[0] != 'G' {
			t.Fatalf("first byte = %q", buf[0])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for HTTP connection")
	}
}

func TestTLSHTTPMuxRoutesTLSHandshake(t *testing.T) {
	mux, err := newTLSHTTPMux("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer mux.Close()

	tlsConnCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, acceptErr := mux.TLSListener().Accept()
		if acceptErr != nil {
			errCh <- acceptErr
			return
		}
		tlsConnCh <- conn
	}()

	client, err := net.Dial("tcp", mux.HTTPListener().Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte{0x16, 0x03, 0x03, 0x00, 0x00}); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("accept failed: %v", err)
	case conn := <-tlsConnCh:
		defer conn.Close()
		buf := make([]byte, 1)
		if _, err := conn.Read(buf); err != nil {
			t.Fatal(err)
		}
		if buf[0] != 0x16 {
			t.Fatalf("first byte = 0x%x", buf[0])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for TLS connection")
	}
}
