package mailimport

import (
	"encoding/base64"
	"errors"
	"io"
	"net/textproto"
	"strings"
	"testing"

	"bearstack/internal/document"
)

func TestDialRejectsInvalidSecurityBeforeNetwork(t *testing.T) {
	_, err := Dial(document.MailImportSettings{
		Host:     "127.0.0.1",
		Port:     1,
		Security: "invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "ungültig") {
		t.Fatalf("Dial invalid security error = %v", err)
	}
}

func TestImportPDFsFromMessageFiltersSenderAndDecodesAttachment(t *testing.T) {
	raw := strings.Join([]string{
		"From: Scanner <scanner@example.com>",
		"Subject: =?utf-8?q?Rechnung_M=C3=A4rz?=",
		"Content-Type: multipart/mixed; boundary=mail-boundary",
		"",
		"--mail-boundary",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hallo",
		"--mail-boundary",
		"Content-Type: application/pdf; name=\"rechnung.pdf\"",
		"Content-Disposition: attachment; filename=\"rechnung.pdf\"",
		"Content-Transfer-Encoding: base64",
		"",
		"JVBERi0xLjQ=",
		"--mail-boundary--",
		"",
	}, "\r\n")

	var attachments []Attachment
	message, err := ImportPDFsFromMessage(strings.NewReader(raw), "@example.com", 1<<20, func(att Attachment) error {
		attachments = append(attachments, att)
		content, readErr := io.ReadAll(att.Reader)
		if readErr != nil {
			return readErr
		}
		if string(content) != "%PDF-1.4" {
			t.Fatalf("attachment content = %q", string(content))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.Subject != "Rechnung März" || message.From != "scanner@example.com" || message.PDFs != 1 || message.Rejected {
		t.Fatalf("message = %#v", message)
	}
	if len(attachments) != 1 || attachments[0].Filename != "rechnung.pdf" {
		t.Fatalf("attachments = %#v", attachments)
	}
}

func TestImportAttachmentsFromMessageFindsPDFsAndEMLAttachments(t *testing.T) {
	innerEML := strings.Join([]string{
		"From: original@example.com",
		"Subject: Original",
		"Content-Type: application/pdf",
		"",
		"%PDF-inside-eml",
	}, "\r\n")
	raw := strings.Join([]string{
		"From: Scanner <scanner@example.com>",
		"Subject: Import",
		"Content-Type: multipart/mixed; boundary=mail-boundary",
		"",
		"--mail-boundary",
		"Content-Type: application/pdf; name=\"rechnung.pdf\"",
		"Content-Disposition: attachment; filename=\"rechnung.pdf\"",
		"",
		"%PDF-outer",
		"--mail-boundary",
		"Content-Type: message/rfc822; name=\"original.eml\"",
		"Content-Disposition: attachment; filename=\"original.eml\"",
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte(innerEML)),
		"--mail-boundary--",
		"",
	}, "\r\n")

	var pdfs []Attachment
	var emls []Attachment
	message, err := ImportAttachmentsFromMessage(strings.NewReader(raw), "@example.com", 1<<20, func(att Attachment) error {
		pdfs = append(pdfs, att)
		return nil
	}, func(att Attachment) error {
		emls = append(emls, att)
		content, readErr := io.ReadAll(att.Reader)
		if readErr != nil {
			return readErr
		}
		if !strings.Contains(string(content), "Subject: Original") {
			t.Fatalf("eml content = %q", string(content))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.PDFs != 1 || message.EMLs != 1 || message.Rejected {
		t.Fatalf("message = %#v", message)
	}
	if len(pdfs) != 1 || pdfs[0].Filename != "rechnung.pdf" {
		t.Fatalf("pdfs = %#v", pdfs)
	}
	if len(emls) != 1 || emls[0].Filename != "original.eml" {
		t.Fatalf("emls = %#v", emls)
	}
}

func TestImportPDFsFromMessageRejectsDisallowedSenderWithoutWalkingAttachments(t *testing.T) {
	raw := "From: scanner@other.test\r\nSubject: Test\r\nContent-Type: application/pdf\r\n\r\n%PDF"
	message, err := ImportPDFsFromMessage(strings.NewReader(raw), "example.com", 1<<20, func(Attachment) error {
		t.Fatal("attachment handler should not run")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !message.Rejected || message.From != "scanner@other.test" || message.PDFs != 0 {
		t.Fatalf("message = %#v", message)
	}
}

func TestImportPDFsFromMessageEnforcesMessageLimit(t *testing.T) {
	raw := "From: scanner@example.com\r\nSubject: Test\r\n\r\n" + strings.Repeat("x", 64)
	_, err := ImportPDFsFromMessage(strings.NewReader(raw), "", 1, nil)
	if !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("err = %v, want ErrMessageTooLarge", err)
	}
}

func TestWalkPDFsUsesSafeFallbackFilename(t *testing.T) {
	header := textproto.MIMEHeader{
		"Content-Type": []string{"application/pdf"},
	}
	var got Attachment
	if err := WalkPDFs(header, strings.NewReader("%PDF"), func(att Attachment) error {
		got = att
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got.Filename != "attachment.pdf" {
		t.Fatalf("filename = %q", got.Filename)
	}
}
