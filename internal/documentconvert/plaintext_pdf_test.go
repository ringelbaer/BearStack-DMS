package documentconvert

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentClassificationSeparatesPlainTextAndLibreOffice(t *testing.T) {
	if !IsPlainTextDocument("note.txt", "") || !IsPlainTextDocument("note.md", "") {
		t.Fatal("txt/md should be plain text documents")
	}
	if IsLibreOfficeDocument("note.txt", "text/plain") || IsLibreOfficeDocument("note.md", "text/markdown") {
		t.Fatal("txt/md should not be LibreOffice documents")
	}
	for _, name := range []string{"note.rtf", "note.doc", "note.docx", "note.pages"} {
		if !IsLibreOfficeDocument(name, "") {
			t.Fatalf("%s should be a LibreOffice document", name)
		}
		if IsPlainTextDocument(name, "") {
			t.Fatalf("%s should not be a plain text document", name)
		}
	}
}

func TestPlainTextPDFEscapesAndWrapsContent(t *testing.T) {
	pdf, err := PlainTextPDF("# Heading <script>\n" + strings.Repeat("longword ", 80))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) {
		t.Fatalf("pdf header = %q", pdf[:8])
	}
	if !bytes.Contains(pdf, []byte("# Heading <script>")) {
		t.Fatalf("plain markdown/html text was not preserved: %s", string(pdf))
	}
	if bytes.Contains(pdf, []byte("<h1>")) || bytes.Contains(pdf, []byte("<script></script>")) {
		t.Fatalf("plain text PDF appears to render markdown/html: %s", string(pdf))
	}
	if bytes.Count(pdf, []byte(" Tj")) < 3 {
		t.Fatalf("expected wrapped text lines in PDF: %s", string(pdf))
	}
}

func TestConvertPlainTextToPDFWritesFile(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "note.md")
	target := filepath.Join(dir, "note.pdf")
	if err := os.WriteFile(source, []byte("# Heading\n\nMarkdown stays plain."), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ConvertPlainTextToPDF(source, target); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("generated PDF is empty")
	}
}
