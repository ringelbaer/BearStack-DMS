package server

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

func purgeFixture(t *testing.T) (*repository.Repository, *storage.Store, document.Document, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "documents.db")
	repo, err := repository.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	store, err := storage.New(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	stored := writeStoredTestFile(t, store, time.Now().UTC().Format("2006/01")+"/document.pdf", []byte("%PDF-1.7\noriginal"))
	id, err := repo.CreateDocument(ctx, document.Document{OriginalName: "document.pdf", StoredPath: stored, MIMEType: "application/pdf", SHA256: "original", Title: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	doc, err := repo.Purge(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return repo, store, doc, dbPath
}

func TestPurgeIndependentDocumentsAndCancellation(t *testing.T) {
	ctx := context.Background()
	repo, store, first, _ := purgeFixture(t)
	path := writeStoredTestFile(t, store, "second.pdf", []byte("%PDF second"))
	id, err := repo.CreateDocument(ctx, document.Document{OriginalName: "second.pdf", StoredPath: path, MIMEType: "application/pdf", SHA256: "second", Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Purge(ctx, id); err != nil {
		t.Fatal(err)
	}
	release, err := store.AcquireDocumentFiles(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := newTrashService(repo, store, nil, nil, nil)
	blocked, cancel := context.WithCancel(ctx)
	defer cancel()
	waitCtx := &observedWaitContext{Context: blocked, waiting: make(chan struct{})}
	firstDone := make(chan error, 1)
	go func() { firstDone <- svc.DeletePurgedDocumentFiles(waitCtx, first.ID) }()
	awaitTestSignal(t, waitCtx.waiting)
	secondDone := make(chan error, 1)
	go func() { secondDone <- svc.DeletePurgedDocumentFiles(ctx, id) }()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("unrelated document blocked by active renderer")
	}
	assertStoredPathMissing(t, store, path)
	cancel()
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("blocked cleanup: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked cleanup did not cancel")
	}
	if _, err := repo.FileDeletion(ctx, first.ID); err != nil {
		t.Fatalf("canceled job lost: %v", err)
	}
}

func TestPurgeConcurrentSameDocumentRemainsIdempotent(t *testing.T) {
	repo, store, doc, _ := purgeFixture(t)
	svc := newTrashService(repo, store, nil, nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.DeletePurgedDocumentFiles(context.Background(), doc.ID); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	assertStoredPathMissing(t, store, doc.StoredPath)
	if _, err := repo.FileDeletion(context.Background(), doc.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("remaining job: %v", err)
	}
}

func TestPurgeRetriesAfterRestartAndDoesNotReuseReservedFilename(t *testing.T) {
	ctx := context.Background()
	repo, store, doc, dbPath := purgeFixture(t)
	// Simulate a crash after moving the original but before recording the phase.
	if err := store.StageDeletion(doc.StoredPath, ".purge/1/0"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err := repository.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	candidate, err := store.ReceiveReader("document.pdf", strings.NewReader("%PDF-1.7\nnew document"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	importer := newDocumentImporter(repo, store, nil, nil)
	result := importer.ImportCandidate(ctx, candidate, document.UploadWayWeb)
	if result.Error != nil || result.Created == nil {
		t.Fatalf("import: %#v", result)
	}
	if result.Created.Document.StoredPath == doc.StoredPath {
		t.Fatal("pending deletion path was reused")
	}
	svc := newTrashService(repo, store, nil, func(context.Context) (int, error) { return 0, nil }, nil)
	if err := svc.RetryFileDeletions(ctx); err != nil {
		t.Fatal(err)
	}
	if err := svc.RetryFileDeletions(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FileDeletion(ctx, doc.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("pending job: %v", err)
	}
	path, err := store.Resolve(result.Created.Document.StoredPath)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "%PDF-1.7\nnew document" {
		t.Fatalf("new file: %q, %v", content, err)
	}
	assertStoredPathMissing(t, store, ".purge/1/0")
}

func TestPurgeRetainsFailedJobAndContinuesWithLaterJobs(t *testing.T) {
	ctx := context.Background()
	repo, store, doc, _ := purgeFixture(t)
	// A nonempty quarantine directory forces a deterministic unlink failure.
	dir, err := store.EnsureDir(".purge/1/0")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blocker"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.StageFileDeletion(ctx, doc.ID); err != nil {
		t.Fatal(err)
	}
	secondPath := writeStoredTestFile(t, store, "second.pdf", []byte("%PDF second"))
	id, err := repo.CreateDocument(ctx, document.Document{OriginalName: "second.pdf", StoredPath: secondPath, MIMEType: "application/pdf", SHA256: "second", Title: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Purge(ctx, id); err != nil {
		t.Fatal(err)
	}
	svc := newTrashService(repo, store, nil, func(context.Context) (int, error) { return 0, nil }, nil)
	if err := svc.RetryFileDeletions(ctx); err == nil {
		t.Fatal("expected deletion failure")
	}
	if _, err := repo.FileDeletion(ctx, doc.ID); err != nil {
		t.Fatalf("failed job lost: %v", err)
	}
	if _, err := repo.FileDeletion(ctx, id); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("later job not completed: %v", err)
	}
	assertStoredPathMissing(t, store, secondPath)
	if err := os.Remove(filepath.Join(dir, "blocker")); err != nil {
		t.Fatal(err)
	}
	if err := svc.RetryFileDeletions(ctx); err != nil {
		t.Fatal(err)
	}
	// Staged jobs must never touch their original path again.
	path, err := store.Resolve(doc.StoredPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("staged retry touched original: %v", err)
	}
}

func TestPurgeStartupRetryRunsWithRetentionDisabled(t *testing.T) {
	repo, store, doc, _ := purgeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	svc := newTrashService(repo, store, nil, func(context.Context) (int, error) { return 0, nil }, nil)
	go func() { defer close(done); svc.RunRetention(ctx) }()
	defer func() { cancel(); <-done }()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("startup retry did not complete")
		case <-ticker.C:
			if _, err := repo.FileDeletion(ctx, doc.ID); errors.Is(err, sql.ErrNoRows) {
				return
			}
		}
	}
}

func TestPurgeWaitsForDerivedFilesAndRejectsStalePreviewRequests(t *testing.T) {
	ctx := context.Background()
	repo, store, doc, _ := purgeFixture(t)
	// Represent an in-flight renderer that acquired its gate before the purge.
	release, err := store.AcquireDocumentFiles(ctx, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	svc := newTrashService(repo, store, nil, func(context.Context) (int, error) { return 0, nil }, nil)
	done := make(chan error, 1)
	go func() { done <- svc.DeletePurgedDocumentFiles(ctx, doc.ID) }()
	select {
	case err := <-done:
		release()
		t.Fatalf("cleanup passed running renderer: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	thumbnail := writeStoredTestFile(t, store, ".thumbnails/1.jpg", []byte("late thumbnail"))
	preview := writeStoredTestFile(t, store, documentOfficePreviewPath(doc.ID), []byte("late preview"))
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup did not resume")
	}
	assertStoredPathMissing(t, store, thumbnail)
	assertStoredPathMissing(t, store, preview)
	service := newThumbnailService(repo, store, nil, make(chan struct{}, 1))
	if err := service.Ensure(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if _, err := service.EnsureOfficePreview(ctx, doc); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale preview request: %v", err)
	}
	assertStoredPathMissing(t, store, thumbnail)
	assertStoredPathMissing(t, store, preview)
}
