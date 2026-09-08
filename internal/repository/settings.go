// Datei speichert allgemeine Anwendungseinstellungen im Repository und liest sie typisiert aus.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"bearstack/internal/sqlutil"
)

// GetSettings reads a coherent snapshot in one SQLite statement.
func (r *Repository) GetSettings(ctx context.Context, keys ...string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return values, ctx.Err()
	}
	args := make([]any, len(keys))
	for i, key := range keys {
		args[i] = strings.TrimSpace(key)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT key,value FROM settings WHERE key IN (`+sqlutil.Placeholders(len(keys))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

const saveSettingSQL = `INSERT INTO settings(key, value, updated_at)
	VALUES (?, ?, ?)
	ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`

// SaveSettings commits a form as a unit. Stable key order also makes failures
// reproducible; normalized duplicate keys are rejected before any writes.
func (r *Repository) SaveSettings(ctx context.Context, values map[string]string) error {
	normalized := make(map[string]string, len(values))
	keys := make([]string, 0, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			return errors.New("setting key is empty")
		}
		if _, exists := normalized[key]; exists {
			return errors.New("duplicate setting key")
		}
		normalized[key] = value
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ctx.Err()
	}
	sort.Strings(keys)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, saveSettingSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()
	updatedAt := formatTime(time.Now().UTC())
	for _, key := range keys {
		if _, err := stmt.ExecContext(ctx, key, normalized[key], updatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) GetSetting(ctx context.Context, key string) (string, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, errors.New("setting key is empty")
	}
	var value string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (r *Repository) SaveSetting(ctx context.Context, key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("setting key is empty")
	}
	_, err := r.db.ExecContext(ctx, saveSettingSQL,
		key,
		value,
		formatTime(time.Now().UTC()),
	)
	return err
}
