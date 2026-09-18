package mailimport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"bearstack/internal/mailmime"
	"bearstack/internal/uploadlimit"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

func TestFetchMessageBoundsIMAPDownload(t *testing.T) {
	const maxUploadBytes = 8
	limit := uploadlimit.EnvelopeLimit(maxUploadBytes)
	for _, tc := range []struct {
		name     string
		size     int64
		body     string
		tooLarge bool
	}{
		{name: "normal", size: 12, body: "mail fixture"},
		{name: "exact limit", size: limit, body: strings.Repeat("x", int(limit))},
		{name: "oversized metadata", size: limit + 1, tooLarge: true},
		{name: "oversized body despite metadata", size: 1, body: strings.Repeat("x", int(limit+1)), tooLarge: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			t.Cleanup(func() { _ = clientConn.Close(); _ = serverConn.Close() })
			_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
			done := make(chan error, 1)
			go func() {
				defer serverConn.Close()
				_, err := io.WriteString(serverConn, "* PREAUTH [CAPABILITY IMAP4rev1] ready\r\n")
				reader := bufio.NewReader(serverConn)
				for step := 0; err == nil && step < 2; step++ {
					line, readErr := reader.ReadString('\n')
					if step == 1 && tc.size > limit && errors.Is(readErr, io.EOF) {
						done <- nil // Rejected before requesting any body bytes.
						return
					}
					if readErr != nil {
						err = readErr
						break
					}
					tag, command, _ := strings.Cut(strings.TrimSpace(line), " ")
					want := "UID FETCH 42 (UID RFC822.SIZE)"
					if step == 1 {
						want = fmt.Sprintf("UID FETCH 42 (UID BODY.PEEK[]<0.%d>)", limit+1)
					}
					if command != want || (step == 1 && tc.size > limit) {
						err = fmt.Errorf("command = %q, want %q", command, want)
						break
					}
					if step == 0 {
						_, err = fmt.Fprintf(serverConn, "* 1 FETCH (UID 42 RFC822.SIZE %d)\r\n%s OK fetched\r\n", tc.size, tag)
					} else {
						_, err = fmt.Fprintf(serverConn, "* 1 FETCH (UID 42 BODY[]<0> {%d}\r\n%s)\r\n%s OK fetched\r\n", len(tc.body), tc.body, tag)
					}
				}
				done <- err
			}()
			c, err := client.New(clientConn)
			if err != nil {
				t.Fatal(err)
			}
			c.Timeout = 5 * time.Second
			c.SetState(imap.SelectedState, &imap.MailboxStatus{Name: "INBOX"})
			body, fetchErr := FetchMessage(c, 42, maxUploadBytes)
			_ = c.Terminate()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if tc.tooLarge {
				if !errors.Is(fetchErr, ErrMessageTooLarge) || body != nil {
					t.Fatalf("body = %v, error = %v", body, fetchErr)
				}
				return
			}
			if fetchErr != nil {
				t.Fatal(fetchErr)
			}
			got, err := io.ReadAll(body)
			if err != nil || string(got) != tc.body {
				t.Fatalf("body = %q, error = %v", got, err)
			}
		})
	}
}

func TestImportRejectsDeepMIMEWithoutConsumingAttachment(t *testing.T) {
	for _, depth := range []int{mailmime.MaxMultipartDepth, mailmime.MaxMultipartDepth + 1, 256} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			raw := nestedImportMail(depth)
			reader := strings.NewReader(raw)
			called := false
			_, err := ImportAttachmentsFromMessage(reader, "", 1<<20, func(att Attachment) error {
				called = true
				_, err := io.Copy(io.Discard, att.Reader)
				return err
			}, nil)
			if depth <= mailmime.MaxMultipartDepth {
				if err != nil || !called {
					t.Fatalf("allowed nesting: callback = %v, error = %v", called, err)
				}
			} else {
				if !errors.Is(err, mailmime.ErrMultipartTooDeep) || called {
					t.Fatalf("excessive nesting: callback = %v, error = %v", called, err)
				}
				if reader.Len() < 1<<20 {
					t.Fatal("rejected MIME body was drained")
				}
			}
		})
	}
}

func nestedImportMail(depth int) string {
	var b strings.Builder
	b.WriteString("From: scanner@example.com\r\n")
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=level%d\r\n\r\n--level%d\r\n", i, i)
	}
	b.WriteString("Content-Type: application/pdf; name=invoice.pdf\r\n\r\n%PDF-1.4\n")
	b.WriteString(strings.Repeat("x", 2<<20))
	for i := depth - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "\r\n--level%d--\r\n", i)
	}
	return b.String()
}
