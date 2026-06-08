package documentocr

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizeText(t *testing.T) {
	got := normalizeText(" Zeile 1\r\n\r\n\r\nZeile 2\f\n")
	want := "Zeile 1\n\nZeile 2"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPageNumber(t *testing.T) {
	if got := pageNumber("/tmp/page-12.png"); got != 12 {
		t.Fatalf("page = %d", got)
	}
}

func TestParsePDFPageCount(t *testing.T) {
	got, err := parsePDFPageCount("Title: Scan\nPages:          42\nEncrypted:      no\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != 42 {
		t.Fatalf("pages = %d", got)
	}
}

func TestPDFBulkFallbackMessageUsesConfiguredTimeoutAndLimit(t *testing.T) {
	message := pdfBulkProgressMessage()
	if !strings.Contains(message, "20 Minuten") || strings.Contains(message, "10 Minuten") {
		t.Fatalf("timeout message = %q", message)
	}
	if !strings.Contains(message, strconv.Itoa(pdfBulkFallbackMaxPages)) {
		t.Fatalf("limit missing in message = %q", message)
	}
}

func TestPDFBulkFallbackCapsRenderedPages(t *testing.T) {
	args := pdfBulkCommandArgs("scan.pdf", "/tmp/page")
	limitIndex := slices.Index(args, "-l")
	if limitIndex < 0 || limitIndex+1 >= len(args) {
		t.Fatalf("missing -l in args: %#v", args)
	}
	if got, want := args[limitIndex+1], strconv.Itoa(pdfBulkFallbackMaxPages+1); got != want {
		t.Fatalf("bulk fallback page limit arg = %q, want %q", got, want)
	}
}
