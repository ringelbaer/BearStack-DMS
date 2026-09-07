package repository

import (
	"context"
	"path/filepath"
	"testing"

	"bearstack/internal/document"
)

func TestFileDeletionMigrationAndPurgeTransaction(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "migration.db")
	repo, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `DROP TABLE document_file_deletions`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE schema_migrations SET version = 16 WHERE component = ?`, repositorySchemaComponent); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err = Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	id, err := repo.CreateDocument(ctx, document.Document{OriginalName: "one.pdf", StoredPath: "one.pdf", Title: "One", MIMEType: "application/pdf", SHA256: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, id); err != nil {
		t.Fatal(err)
	}
	// Failure after enqueuing must roll back both the document and the job.
	if _, err := repo.db.ExecContext(ctx, `CREATE TRIGGER reject_purge BEFORE DELETE ON documents BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	for _, bulk := range []bool{false, true} {
		if bulk {
			_, err = repo.PurgeTrash(ctx)
		} else {
			_, err = repo.Purge(ctx, id)
		}
		if err == nil {
			t.Fatal("expected purge failure")
		}
		jobs, err := repo.PendingFileDeletions(ctx, 0, 100)
		if err != nil || len(jobs) != 0 {
			t.Fatalf("rolled back jobs: %#v, %v", jobs, err)
		}
		if _, err := repo.GetDocument(ctx, id); err != nil {
			t.Fatalf("document lost: %v", err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, `DROP TRIGGER reject_purge`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PurgeTrash(ctx); err != nil {
		t.Fatal(err)
	}
	job, err := repo.FileDeletion(ctx, id)
	if err != nil || job.StoredPath != "one.pdf" || job.Staged {
		t.Fatalf("job: %#v, %v", job, err)
	}
}
