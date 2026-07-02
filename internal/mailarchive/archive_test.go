package mailarchive

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildPrefersPlainTextAndListsNonPDFAttachments(t *testing.T) {
	raw := strings.Join([]string{
		"From: Billing <billing@example.com>",
		"To: archive@example.com",
		"Subject: Rechnung Juli",
		"Date: Wed, 01 Jul 2026 10:20:00 +0200",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="mixed"`,
		"",
		"--mixed",
		`Content-Type: multipart/alternative; boundary="alt"`,
		"",
		"--alt",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<strong>HTML body</strong>",
		"--alt",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Plain body wins.",
		"--alt--",
		"--mixed",
		`Content-Type: image/png; name="logo.png"`,
		`Content-Disposition: attachment; filename="logo.png"`,
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte("png")),
		"--mixed--",
		"",
	}, "\r\n")

	result, err := Build(context.Background(), "original.eml", strings.NewReader(raw), Options{MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if result.Title != "Rechnung Juli" || result.BodySource != "text/plain" {
		t.Fatalf("result title/source = %q/%q", result.Title, result.BodySource)
	}
	if result.DocumentDate == nil || result.DocumentDate.Format("2006-01-02") != "2026-07-01" {
		t.Fatalf("document date = %#v", result.DocumentDate)
	}
	if !strings.HasPrefix(result.Filename, "2026-07-01_E-Mail - Rechnung Juli") {
		t.Fatalf("filename = %q", result.Filename)
	}
	if len(result.OtherAttachments) != 1 || result.OtherAttachments[0].Filename != "logo.png" || result.PDFs != 0 {
		t.Fatalf("attachments = pdfs:%d other:%#v", result.PDFs, result.OtherAttachments)
	}
	content := readFileString(t, result.Path)
	if !strings.Contains(content, "Plain body wins.") || strings.Contains(content, "HTML body") {
		t.Fatalf("pdf content = %s", content)
	}
}

func TestBuildFallsBackToSafeHTMLText(t *testing.T) {
	raw := strings.Join([]string{
		"From: sender@example.com",
		"Subject: HTML",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<style>body{display:none}</style><script>alert(1)</script><p>Hallo <b>Welt</b> &amp; Archiv</p>",
	}, "\r\n")

	result, err := Build(context.Background(), "html.eml", strings.NewReader(raw), Options{MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if result.BodySource != "text/html" {
		t.Fatalf("body source = %q", result.BodySource)
	}
	content := readFileString(t, result.Path)
	if !strings.Contains(content, "Hallo Welt & Archiv") || strings.Contains(content, "alert") || strings.Contains(content, "display:none") {
		t.Fatalf("pdf content = %s", content)
	}
}

func TestBuildMergesPDFsAfterGeneratedMailPages(t *testing.T) {
	raw := strings.Join([]string{
		"From: sender@example.com",
		"Subject: Mit Anlagen",
		"Content-Type: multipart/mixed; boundary=mail",
		"",
		"--mail",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Bitte Anlagen archivieren.",
		"--mail",
		`Content-Type: application/pdf; name="one.pdf"`,
		`Content-Disposition: attachment; filename="one.pdf"`,
		"",
		"%PDF-one",
		"--mail",
		`Content-Type: application/pdf; name="two.pdf"`,
		`Content-Disposition: attachment; filename="two.pdf"`,
		"",
		"%PDF-two",
		"--mail--",
		"",
	}, "\r\n")
	var mergeInputs []string
	merge := func(_ context.Context, output string, inputs []string) error {
		mergeInputs = append([]string(nil), inputs...)
		return os.WriteFile(output, []byte("%PDF-merged"), 0o600)
	}

	result, err := Build(context.Background(), "attachments.eml", strings.NewReader(raw), Options{MaxBytes: 1 << 20, MergePDFs: merge})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if result.PDFs != 2 || len(mergeInputs) != 3 {
		t.Fatalf("pdfs=%d mergeInputs=%#v", result.PDFs, mergeInputs)
	}
	if filepath.Base(mergeInputs[0]) != "message.pdf" {
		t.Fatalf("first merge input = %q", mergeInputs[0])
	}
	if got := readFileString(t, mergeInputs[0]); !strings.Contains(got, "Bitte Anlagen archivieren.") {
		t.Fatalf("generated mail pdf = %s", got)
	}
	if got := readFileString(t, result.Path); got != "%PDF-merged" {
		t.Fatalf("merged output = %q", got)
	}
}

func TestArchiveFilenameWithoutDateUsesSubject(t *testing.T) {
	result, err := Build(context.Background(), "plain.eml", strings.NewReader("Subject: Ohne Datum\r\n\r\nText"), Options{MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if result.DocumentDate != nil {
		t.Fatalf("document date = %s", result.DocumentDate.Format(time.RFC3339))
	}
	if result.Filename != "E-Mail - Ohne Datum.pdf" {
		t.Fatalf("filename = %q", result.Filename)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
