package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
	"bearstack/internal/testutil"
)

func TestHandlePreviewConvertsLibreOfficeDocumentToPDF(t *testing.T) {
	testutil.InstallSoffice(t)
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}

	storedPath := writeStoredTestFile(t, store, "2026/05/note.rtf", []byte(`{\rtf1\ansi preview text}`))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "note.rtf",
		StoredPath:   storedPath,
		Title:        "Note",
		MIMEType:     "application/rtf",
		SizeBytes:    24,
		SHA256:       "preview-office",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server.templates = templates
	req := httptest.NewRequest(http.MethodGet, "/documents/"+strconv.FormatInt(docID, 10)+"/preview", nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handlePreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/pdf") {
		t.Fatalf("content type = %q", got)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "inline") {
		t.Fatalf("content disposition = %q", rec.Header().Get("Content-Disposition"))
	}
	previewAbs, err := store.Resolve(documentOfficePreviewPath(docID))
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(previewAbs); err != nil || info.Size() == 0 {
		t.Fatalf("preview cache stat = %v size=%d", err, fileSize(info))
	}

	t.Setenv("PATH", t.TempDir())
	second := httptest.NewRecorder()
	server.handlePreview(second, req)
	if second.Code != http.StatusOK {
		t.Fatalf("cached preview status = %d body = %s", second.Code, second.Body.String())
	}
}

func TestHandlePreviewConvertsPlainTextDocumentWithoutSoffice(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}

	storedPath := writeStoredTestFile(t, store, "2026/05/note.md", []byte("# Heading\n\nMarkdown stays plain."))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "note.md",
		StoredPath:   storedPath,
		Title:        "Note",
		MIMEType:     "text/markdown; charset=utf-8",
		SizeBytes:    30,
		SHA256:       "preview-plain-text",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		apps:  serverApplications{documents: documentApplication{thumbnails: thumbnailRunnerStub{}}},
	}
	req := httptest.NewRequest(http.MethodGet, "/documents/"+strconv.FormatInt(docID, 10)+"/preview", nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	previewService := server.officePreviewService()
	server.handlePreview(rec, req)
	if server.officePreviewService() != previewService {
		t.Fatal("office preview fallback was recreated")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/pdf") {
		t.Fatalf("content type = %q", got)
	}
	previewAbs, err := store.Resolve(documentOfficePreviewPath(docID))
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := os.ReadFile(previewAbs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pdf), "# Heading") || strings.Contains(string(pdf), "<h1>") {
		t.Fatalf("markdown was not preserved as plain text: %s", string(pdf))
	}
}

func TestHandlePreviewReportsMissingSofficeForLibreOfficeDocument(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}

	storedPath := writeStoredTestFile(t, store, "2026/05/note.rtf", []byte(`{\rtf1\ansi preview text}`))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "note.rtf",
		StoredPath:   storedPath,
		Title:        "Note",
		MIMEType:     "application/rtf",
		SizeBytes:    24,
		SHA256:       "preview-missing-soffice",
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server.templates = templates
	req := httptest.NewRequest(http.MethodGet, "/documents/"+strconv.FormatInt(docID, 10)+"/preview", nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handlePreview(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content type = %q", got)
	}
	if strings.Contains(rec.Body.String(), `class="page`) || strings.Contains(rec.Body.String(), "BearStack") {
		t.Fatalf("preview error rendered full app shell: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Vorschau nicht verfuegbar") {
		t.Fatalf("preview error missing standalone message: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "soffice") {
		t.Fatalf("preview error leaked tool details: %s", rec.Body.String())
	}
	storedAbs, err := store.Resolve(storedPath)
	if err != nil {
		t.Fatalf("stored file was affected: %v", err)
	}
	if _, err := os.Stat(storedAbs); err != nil {
		t.Fatalf("stored file stat = %v", err)
	}
}

func fileSize(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
}
