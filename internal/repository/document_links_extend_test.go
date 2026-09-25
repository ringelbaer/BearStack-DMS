package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"bearstack/internal/document"
)

func TestExtendDocumentLinks(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ids := make([]int64, 6)
	for i := range ids {
		ids[i], err = repo.CreateDocument(ctx, document.Document{OriginalName: fmt.Sprintf("%d.pdf", i), StoredPath: fmt.Sprintf("%d.pdf", i), SHA256: fmt.Sprint(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	link := func(values ...int64) {
		t.Helper()
		if err := repo.LinkDocuments(ctx, values); err != nil {
			t.Fatal(err)
		}
	}
	assertLinks := func(id int64, want ...int64) {
		t.Helper()
		docs, err := repo.LinkedDocuments(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		found := map[int64]bool{}
		for _, doc := range docs {
			found[doc.ID] = true
		}
		if len(found) != len(want) {
			t.Fatalf("links for %d: %v, want %v", id, found, want)
		}
		for _, other := range want {
			if !found[other] {
				t.Fatalf("missing link %d -> %d", id, other)
			}
		}
	}
	link(ids[0], ids[1], ids[2])
	// A separate link of a partner must not be traversed recursively.
	link(ids[2], ids[4])
	link(ids[0], ids[5])
	if err := repo.SoftDelete(ctx, ids[5]); err != nil {
		t.Fatal(err)
	}
	affected, err := repo.ExtendDocumentLinks(ctx, []int64{ids[3], ids[0], ids[3]})
	if err != nil {
		t.Fatal(err)
	}
	if len(affected) != 4 {
		t.Fatalf("affected = %v", affected)
	}
	assertLinks(ids[3], ids[0], ids[1], ids[2])
	assertLinks(ids[4], ids[2])
	assertLinks(ids[1], ids[0], ids[2], ids[3])
	for _, selection := range [][]int64{{ids[0]}, {ids[0], ids[3]}, {ids[0], ids[1], ids[2]}} {
		if _, err := repo.ExtendDocumentLinks(ctx, selection); !errors.Is(err, ErrDocumentLinkSelectionChanged) {
			t.Fatalf("selection %v: %v", selection, err)
		}
	}
	if _, err := repo.ExtendDocumentLinks(ctx, []int64{ids[0], 999999}); !errorsIsNoRows(err) {
		t.Fatalf("missing document: %v", err)
	}
	assertLinks(ids[3], ids[0], ids[1], ids[2])
}

func TestExtendDocumentLinksRollback(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ids := make([]int64, 3)
	for i := range ids {
		ids[i], err = repo.CreateDocument(ctx, document.Document{OriginalName: fmt.Sprint(i), StoredPath: fmt.Sprint(i), SHA256: fmt.Sprint(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.ExtendDocumentLinks(ctx, ids[:2]); !errors.Is(err, ErrDocumentLinkSelectionChanged) {
		t.Fatalf("unlinked pair: %v", err)
	}
	if err := repo.LinkDocuments(ctx, ids[:2]); err != nil {
		t.Fatal(err)
	}
	// Abort the second new edge and prove the first edge is rolled back too.
	_, err = repo.db.Exec(fmt.Sprintf(`CREATE TRIGGER fail_extension BEFORE INSERT ON document_links WHEN NEW.source_document_id = %d AND NEW.target_document_id = %d BEGIN SELECT RAISE(ABORT, 'test failure'); END`, ids[1], ids[2]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExtendDocumentLinks(ctx, []int64{ids[0], ids[2]}); err == nil {
		t.Fatal("expected failure")
	}
	links, err := repo.LinkedDocuments(ctx, ids[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("partial write: %v", links)
	}
}

func TestExtendMultipleDocumentLinks(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ids := make([]int64, 5)
	for i := range ids {
		ids[i], err = repo.CreateDocument(ctx, document.Document{OriginalName: fmt.Sprint(i), StoredPath: fmt.Sprint(i), SHA256: fmt.Sprint(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.LinkDocuments(ctx, ids[:2]); err != nil {
		t.Fatal(err)
	}
	affected, err := repo.ExtendDocumentLinks(ctx, []int64{ids[2], ids[0], ids[3], ids[4]})
	if err != nil {
		t.Fatal(err)
	}
	if len(affected) != 5 {
		t.Fatalf("affected = %v", affected)
	}
	for _, id := range ids {
		links, err := repo.LinkedDocuments(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(links) != 4 {
			t.Fatalf("links for %d = %v", id, links)
		}
	}
}
