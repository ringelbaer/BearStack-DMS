package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneAuditLogsEnforcesHardEntryCap(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "audit-hardcap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	const excess = 37
	total := auditLogMaxEntries + excess
	occurredAt := formatTime(time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC))
	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO audit_logs(occurred_at, actor, method, path, action, status)
		VALUES (?, ?, 'POST', '/login', ?, 403)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	for i := 1; i <= total; i++ {
		if _, err := stmt.ExecContext(ctx, occurredAt, fmt.Sprintf("actor-%d", i), fmt.Sprintf("event-%d", i)); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			t.Fatalf("insert audit entry %d: %v", i, err)
		}
	}
	if err := stmt.Close(); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	cutoff := time.Date(2026, 8, 9, 11, 0, 0, 0, time.UTC)
	if err := repo.PruneAuditLogs(ctx, cutoff); err != nil {
		t.Fatal(err)
	}
	var count int
	var minID, maxID int64
	if err := repo.db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(id), MAX(id) FROM audit_logs`).Scan(&count, &minID, &maxID); err != nil {
		t.Fatal(err)
	}
	if count != auditLogMaxEntries {
		t.Fatalf("audit row count = %d, want hard cap %d", count, auditLogMaxEntries)
	}
	if minID != int64(excess+1) || maxID != int64(total) {
		t.Fatalf("retained audit id range = %d..%d, want newest %d entries", minID, maxID, auditLogMaxEntries)
	}
	logs, err := repo.ListAuditLogs(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Action != fmt.Sprintf("event-%d", total) {
		t.Fatalf("newest audit entry was not retained: %#v", logs)
	}
}
