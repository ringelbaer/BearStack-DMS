package mailimport

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bearstack/internal/document"
)

func TestDialBoundsEntireSetup(t *testing.T) {
	for _, phase := range []string{"greeting", "capability", "starttls command", "starttls handshake", "tls handshake"} {
		t.Run(phase, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			peerDone := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					peerDone <- err
					return
				}
				defer conn.Close()
				// The test peer also has a deadline so a regression cannot hang tests.
				_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
				reader := bufio.NewReader(conn)
				if phase != "greeting" && phase != "tls handshake" {
					greeting := "* OK [CAPABILITY IMAP4rev1 STARTTLS] ready\r\n"
					if phase == "capability" {
						greeting = "* OK ready\r\n"
					}
					_, err = io.WriteString(conn, greeting)
					if err == nil {
						var line string
						line, err = reader.ReadString('\n')
						want := "STARTTLS"
						if phase == "capability" {
							want = "CAPABILITY"
						}
						tag, command, _ := strings.Cut(strings.TrimSpace(line), " ")
						if err == nil && command != want {
							err = fmt.Errorf("command = %q, want %q", command, want)
						}
						if err == nil && phase == "starttls handshake" {
							_, err = fmt.Fprintf(conn, "%s OK begin TLS\r\n", tag)
						}
					}
				}
				if err == nil {
					// Wait for the client to close, without completing this setup phase.
					_, err = io.Copy(io.Discard, reader)
				}
				peerDone <- err
			}()
			security := document.MailImportSecuritySTARTTLS
			if phase == "tls handshake" {
				security = document.MailImportSecurityTLS
			}
			started := time.Now()
			c, err := dial(document.MailImportSettings{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Security: security}, 100*time.Millisecond)
			elapsed := time.Since(started)
			if c != nil {
				_ = c.Terminate()
			}
			if err == nil || c != nil || elapsed > time.Second {
				t.Errorf("setup returned client=%v error=%v after %v; want failure within one second", c != nil, err, elapsed)
			}
			if err := <-peerDone; err != nil {
				t.Errorf("client did not cleanly close failed setup: %v", err)
			}
		})
	}
}

func TestDialKeepsEstablishedConnectionOpen(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, err = io.WriteString(conn, "* OK [CAPABILITY IMAP4rev1] ready\r\n")
		if err == nil {
			var line string
			line, err = bufio.NewReader(conn).ReadString('\n')
			tag, command, _ := strings.Cut(strings.TrimSpace(line), " ")
			if err == nil && command != "NOOP" {
				err = fmt.Errorf("command = %q, want NOOP", command)
			}
			if err == nil {
				_, err = fmt.Fprintf(conn, "%s OK alive\r\n", tag)
			}
		}
		if err == nil {
			_, err = io.Copy(io.Discard, conn)
		}
		done <- err
	}()
	const setupTimeout = 100 * time.Millisecond
	c, err := dial(document.MailImportSettings{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Security: document.MailImportSecurityNone}, setupTimeout)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Terminate()
	if c.Timeout != 2*time.Minute {
		t.Fatalf("command timeout = %v", c.Timeout)
	}
	// A completed setup must cancel the watchdog instead of closing this socket.
	time.Sleep(2 * setupTimeout)
	if err := c.Noop(); err != nil {
		t.Fatal(err)
	}
	_ = c.Terminate()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDialStillVerifiesTLSCertificate(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	c, err := dial(document.MailImportSettings{Host: "127.0.0.1", Port: server.Listener.Addr().(*net.TCPAddr).Port, Security: document.MailImportSecurityTLS}, 2*time.Second)
	if c != nil {
		_ = c.Terminate()
	}
	var certificateError *tls.CertificateVerificationError
	if c != nil || !errors.As(err, &certificateError) {
		t.Fatalf("untrusted TLS certificate: client=%v error=%v", c != nil, err)
	}
}
