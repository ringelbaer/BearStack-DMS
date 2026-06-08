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
	"testing"

	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

func TestHandlePurgeRemovesStoredFileAndThumbnail(t *testing.T) {
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

	storedPath := writeStoredTestFile(t, store, "2026/05/rechnung.pdf", []byte("%PDF-1.7\nbody"))
	thumbnailPath := writeStoredTestFile(t, store, "thumbnails/2026/05/rechnung.jpg", []byte("jpg"))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "rechnung.pdf",
		StoredPath:    storedPath,
		ThumbnailPath: thumbnailPath,
		Title:         "Rechnung",
		MIMEType:      "application/pdf",
		SizeBytes:     13,
		SHA256:        "purge-files",
	})
	if err != nil {
		t.Fatal(err)
	}
	previewPath := writeStoredTestFile(t, store, documentOfficePreviewPath(docID), []byte("%PDF cached"))
	if err := repo.SoftDelete(ctx, docID); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodPost, "/documents/"+strconv.FormatInt(docID, 10)+"/purge", nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handlePurge(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if _, err := repo.GetDocument(ctx, docID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("document err = %v", err)
	}
	assertStoredPathMissing(t, store, storedPath)
	assertStoredPathMissing(t, store, thumbnailPath)
	assertStoredPathMissing(t, store, previewPath)
}

func TestHandleRestoreMovesDocumentOutOfTrash(t *testing.T) {
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
		OriginalName: "restore.pdf",
		StoredPath:   writeStoredTestFile(t, store, "2026/05/restore.pdf", []byte("%PDF-1.7\nrestore")),
		Title:        "Restore",
		MIMEType:     "application/pdf",
		SizeBytes:    15,
		SHA256:       "restore-doc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, docID); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodPost, "/documents/"+strconv.FormatInt(docID, 10)+"/restore", nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handleRestore(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != documentURL(docID, "", "Dokument wiederhergestellt.") {
		t.Fatalf("location = %q", location)
	}
	doc, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if doc.IsDeleted() {
		t.Fatalf("document still deleted: %#v", doc)
	}
}

func TestHandleEmptyTrashPurgesDeletedDocumentsAndFiles(t *testing.T) {
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

	storedPath := writeStoredTestFile(t, store, "2026/05/empty-trash.pdf", []byte("%PDF-1.7\nempty"))
	thumbPath := writeStoredTestFile(t, store, "thumbnails/2026/05/empty-trash.jpg", []byte("jpg"))
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "empty-trash.pdf",
		StoredPath:    storedPath,
		ThumbnailPath: thumbPath,
		Title:         "Trash",
		MIMEType:      "application/pdf",
		SizeBytes:     13,
		SHA256:        "empty-trash-doc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, docID); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodPost, "/trash/empty", nil)
	rec := httptest.NewRecorder()

	server.handleEmptyTrash(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != withNotice("/trash", "1 Dokument(e) endgültig gelöscht.") {
		t.Fatalf("location = %q", location)
	}
	if _, err := repo.GetDocument(ctx, docID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("document err = %v", err)
	}
	assertStoredPathMissing(t, store, storedPath)
	assertStoredPathMissing(t, store, thumbPath)
}

func writeStoredTestFile(t *testing.T, store *storage.Store, rel string, content []byte) string {
	t.Helper()

	path := filepath.Join(store.Root(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o640); err != nil {
		t.Fatal(err)
	}
	return rel
}

func assertStoredPathMissing(t *testing.T, store *storage.Store, rel string) {
	t.Helper()

	path, err := store.Resolve(rel)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stored path %q exists or stat failed with unexpected error: %v", rel, err)
	}
}
