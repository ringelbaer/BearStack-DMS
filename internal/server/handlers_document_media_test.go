package server

import (
	"context"
	"database/sql"
	"errors"
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
)

type thumbnailRunnerStub struct {
	ensureAll func(context.Context) error
	ensure    func(context.Context, document.Document) error
}

func (s thumbnailRunnerStub) EnsureAll(ctx context.Context) error {
	if s.ensureAll != nil {
		return s.ensureAll(ctx)
	}
	return nil
}

func (s thumbnailRunnerStub) Ensure(ctx context.Context, doc document.Document) error {
	if s.ensure != nil {
		return s.ensure(ctx, doc)
	}
	return nil
}

func TestHandleThumbnailServesExistingDocumentThumbnail(t *testing.T) {
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

	storedPath := writeStoredTestFile(t, store, "2026/05/thumb-source.pdf", []byte("%PDF-1.7\nthumb"))
	thumbPath := writeStoredTestFile(t, store, "thumbnails/2026/05/thumb-source.jpg", []byte("jpg"))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "thumb-source.pdf",
		StoredPath:    storedPath,
		ThumbnailPath: thumbPath,
		Title:         "Thumb Source",
		MIMEType:      "application/pdf",
		SizeBytes:     13,
		SHA256:        "thumb-existing",
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/documents/"+strconv.FormatInt(docID, 10)+"/thumbnail", nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handleThumbnail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content-type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=86400" {
		t.Fatalf("cache-control = %q", got)
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Fatal("thumbnail body is empty")
	}
}

func TestHandleThumbnailGeneratesAndServesThumbnailWhenMissing(t *testing.T) {
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

	storedPath := writeStoredTestFile(t, store, "2026/05/thumb-generate.pdf", []byte("%PDF-1.7\ngenerate"))
	generatedThumbPath := writeStoredTestFile(t, store, "thumbnails/2026/05/thumb-generate.jpg", []byte("jpg"))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "thumb-generate.pdf",
		StoredPath:   storedPath,
		Title:        "Thumb Generate",
		MIMEType:     "application/pdf",
		SizeBytes:    16,
		SHA256:       "thumb-generate",
	})
	if err != nil {
		t.Fatal(err)
	}

	ensureCalls := 0
	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		apps: serverApplications{
			documents: documentApplication{
				thumbnails: thumbnailRunnerStub{
					ensure: func(runCtx context.Context, doc document.Document) error {
						ensureCalls++
						return repo.UpdateThumbnailPath(runCtx, doc.ID, generatedThumbPath)
					},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/documents/"+strconv.FormatInt(docID, 10)+"/thumbnail", nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handleThumbnail(rec, req)

	if ensureCalls != 1 {
		t.Fatalf("ensure calls = %d, want 1", ensureCalls)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(rec.Body.Bytes()) == 0 {
		t.Fatal("thumbnail body is empty")
	}
	updated, err := repo.GetDocumentFile(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ThumbnailPath != generatedThumbPath {
		t.Fatalf("thumbnail path = %q", updated.ThumbnailPath)
	}
}

func TestHandleThumbnailReturnsNotFoundWhenThumbnailStillMissing(t *testing.T) {
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

	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "thumb-missing.pdf",
		StoredPath:   writeStoredTestFile(t, store, "2026/05/thumb-missing.pdf", []byte("%PDF-1.7\nmissing")),
		Title:        "Thumb Missing",
		MIMEType:     "application/pdf",
		SizeBytes:    15,
		SHA256:       "thumb-missing",
	})
	if err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo:      repo,
		store:     store,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: templates,
		apps: serverApplications{
			documents: documentApplication{
				thumbnails: thumbnailRunnerStub{
					ensure: func(context.Context, document.Document) error {
						return nil
					},
				},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/documents/"+strconv.FormatInt(docID, 10)+"/thumbnail", nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handleThumbnail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRenderPreviewHTTPErrorMapsExpectedStatuses(t *testing.T) {
	server := &Server{
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	cases := []struct {
		name        string
		err         error
		status      int
		wantMessage string
		notContains []string
	}{
		{
			name:        "not found",
			err:         os.ErrNotExist,
			status:      http.StatusNotFound,
			wantMessage: "Dokument nicht gefunden.",
		},
		{
			name:        "sql no rows",
			err:         sql.ErrNoRows,
			status:      http.StatusNotFound,
			wantMessage: "Dokument nicht gefunden.",
		},
		{
			name:        "internal",
			err:         errors.New("open /private/tmp/secret.pdf: permission denied"),
			status:      http.StatusInternalServerError,
			wantMessage: "Dokument konnte nicht geladen werden.",
			notContains: []string{"/private/tmp/secret.pdf", "permission denied"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			server.renderPreviewHTTPError(rec, tc.err)
			if rec.Code != tc.status {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, tc.wantMessage) {
				t.Fatalf("body missing %q: %s", tc.wantMessage, body)
			}
			for _, value := range tc.notContains {
				if strings.Contains(body, value) {
					t.Fatalf("body leaked %q: %s", value, body)
				}
			}
		})
	}
}
