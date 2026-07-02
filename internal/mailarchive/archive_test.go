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

func TestBuildRendersHTMLPartAndListsNonPDFAttachments(t *testing.T) {
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
	renderer := &captureHTMLRenderer{}
	merger := &capturePDFMerger{}

	result, err := Build(context.Background(), "original.eml", strings.NewReader(raw), Options{MaxBytes: 1 << 20, RenderHTML: renderer.render, MergePDFs: merger.merge})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if result.Title != "Rechnung Juli" || result.BodySource != "text/html" {
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
	if !strings.Contains(renderer.html, "<strong>HTML body</strong>") || strings.Contains(renderer.html, "Plain body wins.") || strings.Contains(renderer.html, "logo.png") || strings.Contains(renderer.html, "E-Mail-Archiv") {
		t.Fatalf("rendered html = %s", renderer.html)
	}
	if len(merger.inputs) != 2 || filepath.Base(merger.inputs[0]) != "cover.pdf" || filepath.Base(merger.inputs[1]) != "message.pdf" {
		t.Fatalf("merge inputs = %#v", merger.inputs)
	}
	if got := readFileString(t, merger.inputs[0]); !strings.Contains(got, "E-Mail-Archiv") || !strings.Contains(got, "logo.png") {
		t.Fatalf("cover pdf = %s", got)
	}
}

func TestBuildRendersSafeHTML(t *testing.T) {
	raw := strings.Join([]string{
		"From: sender@example.com",
		"Subject: HTML",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<style>body{display:none}</style><script>alert(1)</script><p>Hallo <b>Welt</b> &amp; Archiv</p>",
	}, "\r\n")
	renderer := &captureHTMLRenderer{}
	merger := &capturePDFMerger{}

	result, err := Build(context.Background(), "html.eml", strings.NewReader(raw), Options{MaxBytes: 1 << 20, RenderHTML: renderer.render, MergePDFs: merger.merge})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if result.BodySource != "text/html" {
		t.Fatalf("body source = %q", result.BodySource)
	}
	if !strings.Contains(renderer.html, "<p>Hallo <b>Welt</b> &amp; Archiv</p>") || strings.Contains(renderer.html, "alert") {
		t.Fatalf("rendered html = %s", renderer.html)
	}
}

func TestBuildRendersHTMLCoverSeparatelyFromMailStyles(t *testing.T) {
	raw := strings.Join([]string{
		"From: sender@example.com",
		"Subject: Global CSS",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<style>body{display:none}.cover{display:none}</style><p>Mailinhalt</p>",
	}, "\r\n")
	renderer := &captureHTMLRenderer{}
	merger := &capturePDFMerger{}

	result, err := Build(context.Background(), "global-css.eml", strings.NewReader(raw), Options{MaxBytes: 1 << 20, RenderHTML: renderer.render, MergePDFs: merger.merge})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if len(merger.inputs) != 2 || filepath.Base(merger.inputs[0]) != "cover.pdf" || filepath.Base(merger.inputs[1]) != "message.pdf" {
		t.Fatalf("merge inputs = %#v", merger.inputs)
	}
	if strings.Contains(renderer.html, "E-Mail-Archiv") || strings.Contains(renderer.html, "PDF-Anhaenge") {
		t.Fatalf("mail html contains cover content: %s", renderer.html)
	}
	cover := readFileString(t, merger.inputs[0])
	if !strings.Contains(cover, "E-Mail-Archiv") || strings.Contains(cover, "display:none") || strings.Contains(cover, ".cover") {
		t.Fatalf("cover pdf = %s", cover)
	}
}

func TestBuildRendersPlainTextThatContainsHTML(t *testing.T) {
	raw := strings.Join([]string{
		"From: sender@example.com",
		"Subject: Plain HTML",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hallo<br/><span style=\"font-weight: 500;\">Welt</span>",
	}, "\r\n")
	renderer := &captureHTMLRenderer{}
	merger := &capturePDFMerger{}

	result, err := Build(context.Background(), "plain-html.eml", strings.NewReader(raw), Options{MaxBytes: 1 << 20, RenderHTML: renderer.render, MergePDFs: merger.merge})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Cleanup()

	if result.BodySource != "text/plain-html" {
		t.Fatalf("body source = %q", result.BodySource)
	}
	if !strings.Contains(renderer.html, "Hallo<br/><span style=\"font-weight: 500;\">Welt</span>") {
		t.Fatalf("rendered html = %s", renderer.html)
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

func TestNormalizeBrowserPDFStabilizesDates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "browser.pdf")
	raw := "%PDF\n/CreationDate (D:20260702131648+00'00')\n/ModDate (D:20260702131649+00'00')\n%%EOF\n"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := normalizeBrowserPDF(path); err != nil {
		t.Fatal(err)
	}

	content := readFileString(t, path)
	if strings.Contains(content, "20260702131648") || strings.Contains(content, "20260702131649") {
		t.Fatalf("pdf dates were not normalized: %s", content)
	}
	if strings.Count(content, "D:20000101000000") != 2 {
		t.Fatalf("normalized dates missing: %s", content)
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

type captureHTMLRenderer struct {
	html string
}

func (r *captureHTMLRenderer) render(_ context.Context, htmlContent, output, _ string) error {
	r.html = htmlContent
	return os.WriteFile(output, []byte("%PDF-rendered\n"+htmlContent), 0o600)
}

type capturePDFMerger struct {
	inputs []string
}

func (m *capturePDFMerger) merge(_ context.Context, output string, inputs []string) error {
	m.inputs = append([]string(nil), inputs...)
	return os.WriteFile(output, []byte("%PDF-merged"), 0o600)
}
