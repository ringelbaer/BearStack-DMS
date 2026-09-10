package documentformat

import "testing"

func TestPreviewClassification(t *testing.T) {
	for _, tc := range []struct {
		name, mime string
		want       Kind
	}{
		{"invoice.bin", " application/RTF ; charset=utf-8", Office},
		{"invoice.DOCX", "application/octet-stream", Office},
		{"note.bin", "\tTEXT/PLAIN\r\n; charset=utf-8", PlainText},
		{"note.MD", "", PlainText},
		{"photo.bin", " IMAGE/PNG ; something=value", Image},
		{"invoice.bin", "APPLICATION/PDF; version=1.7", PDF},
		{"note.doc", "application/pdf", PDF},
		{"note.doc", "text/plain", PlainText},
		{"note.bin", "text/plain-invalid", Unknown},
		{"note.docx.exe", "application/octet-stream", Unknown},
		{"photo.webp", "image/webp", Unknown},
		{"note", "", Unknown},
	} {
		t.Run(tc.name+"/"+tc.mime, func(t *testing.T) {
			if got := Classify(tc.name, tc.mime); got != tc.want {
				t.Fatalf("kind = %v, want %v", got, tc.want)
			}
		})
	}
}

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
