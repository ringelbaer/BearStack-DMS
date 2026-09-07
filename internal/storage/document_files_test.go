package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDocumentFileGateCancellationAndIndependentDocuments(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	release, err := store.AcquireDocumentFiles(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.AcquireDocumentFiles(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	other()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := store.AcquireDocumentFiles(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting: %v", err)
	}
	release()
	if len(store.documentGates) != 0 {
		t.Fatalf("idle gates retained: %d", len(store.documentGates))
	}
}

func TestStageDeletionPreservesQuarantineAndRejectsSymlinks(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(store.Root(), "document.pdf")
	if err := os.WriteFile(source, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.StageDeletion("document.pdf", ".purge/1/original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.StageDeletion("document.pdf", ".purge/1/original"); err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(filepath.Join(store.Root(), ".purge/1/original"))
	if err != nil || string(old) != "old" {
		t.Fatalf("quarantine replaced: %q, %v", old, err)
	}
	current, err := os.ReadFile(source)
	if err != nil || string(current) != "new" {
		t.Fatalf("new original removed: %q, %v", current, err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(store.Root(), "outside")); err != nil {
		t.Fatal(err)
	}
	if err := store.StageDeletion("document.pdf", "outside/original"); err == nil {
		t.Fatal("accepted symlinked quarantine")
	}
	if err := store.StageDeletion("missing.pdf", ".purge/2/original"); err != nil {
		t.Fatal(err)
	}
}
