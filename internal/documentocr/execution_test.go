package documentocr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Exercise the actual process boundary without requiring Poppler or Tesseract.
// PATH excludes installed OCR tools; fixtures use shell builtins and /bin/sleep.
func installOCRTools(t *testing.T, scripts map[string]string) string {
	t.Helper()
	bin := t.TempDir()
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nset -eu\n"+script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	t.Cleanup(func() {
		entries, err := os.ReadDir(temp)
		if err != nil || len(entries) != 0 {
			t.Errorf("OCR temporary files remain: %v, err=%v", entries, err)
		}
	})
	return bin
}

func TestLocalEngineChecksRequiredTools(t *testing.T) {
	for _, tc := range []struct {
		name, mime, wantError string
		tools                 map[string]string
	}{
		{"missing tesseract", "image/png", "tesseract", nil},
		{"missing renderer", "application/pdf", "pdftoppm", map[string]string{"tesseract": "exit 0"}},
		{"unsupported format", "text/plain", "nur für PDF- und Bilddateien", map[string]string{"tesseract": "exit 0"}},
		{"image without poppler", "image/png", "", map[string]string{"tesseract": "exit 0"}},
		{"PDF without optional pdfinfo", "application/pdf", "", map[string]string{"tesseract": "exit 0", "pdftoppm": "exit 0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installOCRTools(t, tc.tools)
			err := (LocalEngine{}).CheckAvailable(tc.mime)
			if tc.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestLocalEngineReadsImageAndReportsProgress(t *testing.T) {
	installOCRTools(t, map[string]string{"tesseract": `
[ "$#" -eq 4 ] && [ "$1" = 'scan with spaces.png' ] && [ "$2" = stdout ] && [ "$3" = -l ] && [ "$4" = deu+eng ] || exit 97
printf ' First line\r\n\r\nSecond line\f\n'
`})
	var progress [][2]int
	text, err := (LocalEngine{}).Document(context.Background(), "scan with spaces.png", "image/png", "deu+eng", func(current, total int, message string) error {
		progress = append(progress, [2]int{current, total})
		return nil
	})
	if err != nil || text != "First line\n\nSecond line" {
		t.Fatalf("text = %q, err=%v", text, err)
	}
	if want := [][2]int{{0, 1}, {1, 1}}; !reflect.DeepEqual(progress, want) {
		t.Fatalf("progress = %v, want %v", progress, want)
	}
}

func TestLocalEnginePDFExecution(t *testing.T) {
	for _, tc := range []struct {
		name, pdfinfo, render, ocr, want, wantError string
		total                                       int
	}{
		{
			name: "pages rendered individually", pdfinfo: "printf 'Pages: 2\n'", total: 2,
			render: `[ "$1" = -f ] && [ "$3" = -l ] && [ "$2" = "$4" ] && [ "$5" = -singlefile ] || exit 97
[ "$9" = 'scan with spaces.pdf' ]
printf '%s' "$2" > "${10}.png"`,
			ocr: `IFS= read -r page < "$1" || :; printf 'Page %s\n' "$page"`, want: "Page 1\n\nPage 2",
		},
		{
			name: "missing page count uses numerically sorted fallback", total: 3,
			render: `[ "$1" = -f ] && [ "$2" = 1 ] && [ "$3" = -l ] && [ "$4" = 51 ] || exit 97
[ "$8" = 'scan with spaces.pdf' ]
for page in 10 2 1; do printf '%s' "$page" > "$9-$page.png"; done`,
			ocr: `IFS= read -r page < "$1" || :; printf 'Page %s\n' "$page"`, want: "Page 1\n\nPage 2\n\nPage 10",
		},
		{
			name: "failed page count uses fallback", pdfinfo: "exit 1", total: 1,
			render: `printf 'page' > "$9-1.png"`, ocr: "printf 'Recovered'", want: "Recovered",
		},
		{
			name:   "fallback refuses more than 50 pages",
			render: `page=1; while [ "$page" -le 51 ]; do printf 'page' > "$9-$page.png"; page=$((page+1)); done`,
			ocr:    "exit 99", wantError: "maximal 50 Seiten",
		},
		{name: "fallback without generated pages", render: "exit 0", wantError: "keine PDF-Seiten"},
		{name: "renderer failure", pdfinfo: "printf 'Pages: 1\n'", render: "printf 'broken PDF' >&2; exit 1", wantError: "broken PDF"},
		{name: "empty rendered page", pdfinfo: "printf 'Pages: 1\n'", render: `: > "${10}.png"`, wantError: "leere Seite"},
		{name: "OCR failure", pdfinfo: "printf 'Pages: 1\n'", render: `printf 'page' > "${10}.png"`, ocr: "printf 'bad image' >&2; exit 1", wantError: "bad image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scripts := map[string]string{"pdftoppm": tc.render, "tesseract": tc.ocr}
			if tc.pdfinfo != "" {
				scripts["pdfinfo"] = tc.pdfinfo
			}
			installOCRTools(t, scripts)
			var completed []int
			lastTotal := 0
			text, err := (LocalEngine{}).Document(context.Background(), "scan with spaces.pdf", "application/pdf", "deu", func(current, total int, message string) error {
				lastTotal = total
				if current > 0 && (len(completed) == 0 || completed[len(completed)-1] != current) {
					completed = append(completed, current)
				}
				return nil
			})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || text != "" {
					t.Fatalf("text = %q, err=%v, want error %q without partial text", text, err, tc.wantError)
				}
				return
			}
			if err != nil || text != tc.want {
				t.Fatalf("text = %q, err=%v, want %q", text, err, tc.want)
			}
			if lastTotal != tc.total || len(completed) != tc.total || completed[len(completed)-1] != tc.total {
				t.Fatalf("completed pages = %v, total=%d, want %d", completed, lastTotal, tc.total)
			}
		})
	}
}

func TestLocalEngineStopsOnProgressError(t *testing.T) {
	for _, mime := range []string{"image/png", "application/pdf"} {
		t.Run(mime, func(t *testing.T) {
			bin := installOCRTools(t, map[string]string{
				"pdfinfo":   "printf 'Pages: 2\n'",
				"pdftoppm":  `printf 'page' > "${10}.png"`,
				"tesseract": `printf 'called\n' >> "$PATH/calls"; printf 'Text'`,
			})
			stop := errors.New("cannot persist progress")
			text, err := (LocalEngine{}).Document(context.Background(), "scan", mime, "deu", func(current, total int, message string) error {
				if current == 1 {
					return stop
				}
				return nil
			})
			if !errors.Is(err, stop) || text != "" {
				t.Fatalf("text = %q, err=%v", text, err)
			}
			calls, err := os.ReadFile(filepath.Join(bin, "calls"))
			if err != nil || string(calls) != "called\n" {
				t.Fatalf("OCR continued after progress error: calls=%q err=%v", calls, err)
			}
		})
	}
}

func TestLocalEngineHonorsCanceledContext(t *testing.T) {
	for _, mime := range []string{"image/png", "application/pdf"} {
		t.Run(mime, func(t *testing.T) {
			bin := installOCRTools(t, map[string]string{
				"pdfinfo":   `printf 'called' > "$PATH/calls"`,
				"pdftoppm":  `printf 'called' > "$PATH/calls"`,
				"tesseract": `printf 'called' > "$PATH/calls"`,
			})
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			text, err := (LocalEngine{}).Document(ctx, "scan", mime, "deu", nil)
			if !errors.Is(err, context.Canceled) || text != "" {
				t.Fatalf("text = %q, err=%v", text, err)
			}
			if _, err := os.Stat(filepath.Join(bin, "calls")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("canceled OCR started a tool: %v", err)
			}
		})
	}
}

func TestLocalEngineCancelsRunningTool(t *testing.T) {
	bin := installOCRTools(t, map[string]string{
		"tesseract": `printf 'started' > "$PATH/started"; exec /bin/sleep 30`,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := (LocalEngine{}).Document(ctx, "scan.png", "image/png", "deu", nil)
		result <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		if _, err := os.Stat(filepath.Join(bin, "started")); err == nil {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("tool exited before cancellation: %v", err)
		case <-deadline.C:
			t.Fatal("OCR tool did not start")
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("running tool cancellation = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled OCR tool did not stop")
	}
}
