//go:build linux

package processrun_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"bearstack/internal/documentconvert"
	"bearstack/internal/documentocr"
	"bearstack/internal/processrun"
)

func TestBubblewrapWithInstalledOfficeAndOCR(t *testing.T) {
	for _, tool := range []string{"bwrap", "soffice", "pdfinfo", "pdftoppm", "tesseract"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " not installed")
		}
	}
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "bubblewrap")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	root := t.TempDir()
	source, target := filepath.Join(root, "sample.txt"), filepath.Join(root, "sample.pdf")
	if err := os.WriteFile(source, []byte("BearStack converter isolation regression test."), 0600); err != nil {
		t.Fatal(err)
	}
	if err := documentconvert.ConvertToPDF(ctx, source, target); err != nil {
		t.Fatal(err)
	}
	if _, err := documentocr.PDF(ctx, target, "eng", nil); err != nil {
		t.Fatal(err)
	}
}

func TestBubblewrapWithChromium(t *testing.T) {
	binary := os.Getenv("BEARSTACK_TEST_CHROMIUM")
	if binary == "" {
		t.Skip("set BEARSTACK_TEST_CHROMIUM to a non-Snap Chromium executable")
	}
	t.Setenv("BEARSTACK_CONVERTER_SANDBOX", "bubblewrap")
	root := t.TempDir()
	source, target := filepath.Join(root, "input.html"), filepath.Join(root, "output.pdf")
	if err := os.WriteFile(source, []byte("<html><body>BearStack</body></html>"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--headless", "--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-sync", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage", "--disable-background-networking", "--disable-javascript", "--user-data-dir="+filepath.Join(root, "profile"), "--print-to-pdf="+target, "file://"+source)
	diag, err := processrun.Run(cmd, processrun.Files{Read: []string{source, filepath.Dir(binary)}, Write: []string{root}})
	if err != nil {
		t.Fatalf("chromium: %v: %s", err, diag)
	}
	if info, err := os.Stat(target); err != nil || info.Size() == 0 {
		t.Fatalf("PDF missing: %v", err)
	}
}
