// Datei speichert allgemeine Anwendungseinstellungen im Repository und liest sie typisiert aus.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

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
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings(key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at`,
		key,
		value,
		formatTime(time.Now().UTC()),
	)
	return err
}
