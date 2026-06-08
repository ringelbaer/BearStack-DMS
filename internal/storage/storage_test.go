package storage

import (
	"bytes"
	"errors"
	"mime/multipart"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSafeFilename(t *testing.T) {
	got := SafeFilename("../../Rechnung: 2024?.pdf")
	if got != "Rechnung- 2024-.pdf" {
		t.Fatalf("SafeFilename() = %q", got)
	}
}

func TestSafeFilenameTruncatesUnicodeAtRuneBoundary(t *testing.T) {
	got := SafeFilename(strings.Repeat("😀", 60) + ".pdf")
	if !utf8.ValidString(got) {
		t.Fatalf("SafeFilename returned invalid UTF-8: %q", got)
	}
	if len(got) > 180 {
		t.Fatalf("SafeFilename length = %d, want <= 180", len(got))
	}
	if filepath.Ext(got) != ".pdf" {
		t.Fatalf("SafeFilename extension = %q", filepath.Ext(got))
	}
	if strings.Count(strings.TrimSuffix(got, ".pdf"), "😀") != 44 {
		t.Fatalf("SafeFilename emoji count = %d, name = %q", strings.Count(strings.TrimSuffix(got, ".pdf"), "😀"), got)
	}
}

func TestReceiveAndCommitPDF(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	part := multipartPart(t, "files", "2024-01-31_Rechnung.pdf", []byte("%PDF-1.7\ncontent"))
	candidate, err := store.Receive(part, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.MIMEType != "application/pdf" {
		t.Fatalf("MIMEType = %q", candidate.MIMEType)
	}

	rel, err := store.Commit(candidate, time.Date(2024, 2, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if rel != "2024/02/2024-01-31_Rechnung.pdf" {
		t.Fatalf("rel = %q", rel)
	}
	abs, err := store.Resolve(rel)
	if err != nil {
		t.Fatal(err)
	}
	assertCommittedDocumentMode(t, abs)
}

func TestReceiveAcceptsTextAndOfficeDocuments(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		content []byte
	}{
		{name: "note.txt", content: []byte("plain text")},
		{name: "note.md", content: []byte("# Heading\n\nMarkdown text")},
		{name: "note.rtf", content: []byte(`{\rtf1\ansi RTF text}`)},
		{name: "note.doc", content: append([]byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}, []byte("doc")...)},
		{name: "note.docx", content: []byte{'P', 'K', 3, 4, 'd', 'o', 'c', 'x'}},
		{name: "note.pages", content: []byte{'P', 'K', 3, 4, 'p', 'a', 'g', 'e', 's'}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			candidate, err := store.ReceiveReader(tc.name, bytes.NewReader(tc.content), 1024)
			if err != nil {
				t.Fatal(err)
			}
			if candidate.MIMEType == "" {
				t.Fatal("MIMEType is empty")
			}
		})
	}
}

func TestReceiveRejectsOversize(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	part := multipartPart(t, "files", "doc.pdf", []byte("%PDF-1.7\ncontent"))
	_, err = store.Receive(part, 4)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestCommitCreatesUniqueNamesForConcurrentSameFilename(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	const count = 32
	candidates := make([]Candidate, 0, count)
	for i := 0; i < count; i++ {
		candidate, err := store.ReceiveReader("rechnung.pdf", bytes.NewReader([]byte("%PDF-1.7\ncontent")), 1024)
		if err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, candidate)
	}

	now := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	paths := make(chan string, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		wg.Add(1)
		go func(candidate Candidate) {
			defer wg.Done()
			rel, err := store.Commit(candidate, now)
			if err != nil {
				errs <- err
				return
			}
			paths <- rel
		}(candidate)
	}
	wg.Wait()
	close(paths)
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for rel := range paths {
		if _, ok := seen[rel]; ok {
			t.Fatalf("duplicate committed path %q", rel)
		}
		seen[rel] = struct{}{}
		if _, err := store.Resolve(rel); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != count {
		t.Fatalf("committed %d files, want %d", len(seen), count)
	}
}

func TestResolveRejectsSymlinkComponents(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	link := filepath.Join(root, "2026")
	createSymlinkOrSkip(t, outside, link)

	if _, err := store.Resolve("2026/secret.pdf"); err == nil {
		t.Fatal("expected symlinked directory to be rejected")
	}
}

func TestResolveRejectsSymlinkFile(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.pdf")
	if err := os.WriteFile(outside, []byte("%PDF-1.7\nsecret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "2026", "05"), 0o750); err != nil {
		t.Fatal(err)
	}
	createSymlinkOrSkip(t, outside, filepath.Join(root, "2026", "05", "secret.pdf"))

	if _, err := store.Resolve("2026/05/secret.pdf"); err == nil {
		t.Fatal("expected symlinked file to be rejected")
	}
}

func TestReceiveRejectsSymlinkTempDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".tmp")); err != nil {
		t.Fatal(err)
	}
	createSymlinkOrSkip(t, t.TempDir(), filepath.Join(root, ".tmp"))

	_, err = store.ReceiveReader("doc.pdf", bytes.NewReader([]byte("%PDF-1.7\ncontent")), 1024)
	if err == nil {
		t.Fatal("expected symlinked temp directory to be rejected")
	}
}

func TestCommitRejectsSymlinkedTargetDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.ReceiveReader("doc.pdf", bytes.NewReader([]byte("%PDF-1.7\ncontent")), 1024)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	createSymlinkOrSkip(t, outside, filepath.Join(root, "2026"))

	_, err = store.Commit(candidate, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected symlinked target directory to be rejected")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "05", "doc.pdf")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside file stat = %v", statErr)
	}
}

func TestCopyTempFileNoReplaceSetsCommittedPermissions(t *testing.T) {
	root := t.TempDir()
	tempPath := filepath.Join(root, "upload.tmp")
	targetPath := filepath.Join(root, "target.pdf")
	if err := os.WriteFile(tempPath, []byte("%PDF-1.7\ncontent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyTempFileNoReplace(tempPath, targetPath); err != nil {
		t.Fatal(err)
	}
	assertCommittedDocumentMode(t, targetPath)
}

func assertCommittedDocumentMode(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != committedDocumentFilePerm {
		t.Fatalf("mode = %#o, want %#o", mode, committedDocumentFilePerm)
	}
}

func createSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
}

func multipartPart(t *testing.T, field, filename string, content []byte) *multipart.Part {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	reader := multipart.NewReader(&body, writer.Boundary())
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	return part
}
