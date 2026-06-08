// Datei speichert und liest Audit-Logs fuer nachvollziehbare Aenderungen im Repository.
package repository

import (
	"context"
	"strings"
	"time"

	"bearstack/internal/document"
)

func (r *Repository) SaveAuditLog(ctx context.Context, entry document.AuditLogEntry) error {
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = time.Now().UTC()
	}
	entry.Actor = truncateString(strings.TrimSpace(entry.Actor), 120)
	entry.Method = truncateString(strings.TrimSpace(entry.Method), 16)
	entry.Path = truncateString(strings.TrimSpace(entry.Path), 500)
	entry.Route = truncateString(strings.TrimSpace(entry.Route), 500)
	entry.Action = truncateString(strings.TrimSpace(entry.Action), 160)
	entry.Target = truncateString(strings.TrimSpace(entry.Target), 500)
	entry.RemoteAddr = truncateString(strings.TrimSpace(entry.RemoteAddr), 160)
	entry.UserAgent = truncateString(strings.TrimSpace(entry.UserAgent), 300)

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO audit_logs(occurred_at, actor, method, path, route, action, target, status, remote_addr, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		formatTime(entry.OccurredAt),
		entry.Actor,
		entry.Method,
		entry.Path,
		entry.Route,
		entry.Action,
		entry.Target,
		entry.Status,
		entry.RemoteAddr,
		entry.UserAgent,
	)
	return err
}

func (r *Repository) PruneAuditLogs(ctx context.Context, before time.Time) error {
	if before.IsZero() {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM audit_logs WHERE occurred_at < ?`, formatTime(before))
	return err
}

func (r *Repository) ListAuditLogs(ctx context.Context, limit, offset int) ([]document.AuditLogEntry, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, occurred_at, actor, method, path, route, action, target, status, remote_addr, user_agent
		FROM audit_logs
		ORDER BY occurred_at DESC, id DESC
		LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]document.AuditLogEntry, 0, limit)
	for rows.Next() {
		var entry document.AuditLogEntry
		var occurredAt string
		if err := rows.Scan(
			&entry.ID,
			&occurredAt,
			&entry.Actor,
			&entry.Method,
			&entry.Path,
			&entry.Route,
			&entry.Action,
			&entry.Target,
			&entry.Status,
			&entry.RemoteAddr,
			&entry.UserAgent,
		); err != nil {
			return nil, err
		}
		entry.OccurredAt, err = time.Parse(time.RFC3339, occurredAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
