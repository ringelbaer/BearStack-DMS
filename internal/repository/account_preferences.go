// Datei speichert kontobezogene, nicht sicherheitskritische UI-Praeferenzen.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	AccountSourceConfig   = "config"
	AccountSourceDatabase = "database"
)

var ErrAccountPreferenceConflict = errors.New("Kontoeinstellungen wurden zwischenzeitlich geaendert")

type AccountPreference struct {
	Source                  string
	Subject                 string
	CustomPDFPreviewEnabled bool
	RowVersion              int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type SaveAccountPreferenceParams struct {
	Source                  string
	Subject                 string
	CustomPDFPreviewEnabled bool
	ExpectedRowVersion      int64
}

func (r *Repository) ListAccountPreferences(ctx context.Context) ([]AccountPreference, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT account_source, account_subject, custom_pdf_preview_enabled, row_version, created_at, updated_at
		FROM account_preferences
		ORDER BY account_source, account_subject`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AccountPreference, 0)
	for rows.Next() {
		preference, err := scanAccountPreference(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, preference)
	}
	return result, rows.Err()
}

func (r *Repository) AccountPreference(ctx context.Context, source, subject string) (AccountPreference, error) {
	source = strings.TrimSpace(source)
	subject = strings.TrimSpace(subject)
	if err := validateAccountPreferenceKey(source, subject); err != nil {
		return AccountPreference{}, err
	}
	return scanAccountPreference(r.db.QueryRowContext(ctx, `
		SELECT account_source, account_subject, custom_pdf_preview_enabled, row_version, created_at, updated_at
		FROM account_preferences WHERE account_source = ? AND account_subject = ?`, source, subject))
}

func (r *Repository) SaveAccountPreference(ctx context.Context, params SaveAccountPreferenceParams) (AccountPreference, error) {
	params.Source = strings.TrimSpace(params.Source)
	params.Subject = strings.TrimSpace(params.Subject)
	if err := validateAccountPreferenceKey(params.Source, params.Subject); err != nil {
		return AccountPreference{}, err
	}
	now := formatTime(time.Now().UTC())
	if params.ExpectedRowVersion == 0 {
		_, err := r.db.ExecContext(ctx, `
			INSERT INTO account_preferences(
				account_source, account_subject, custom_pdf_preview_enabled, row_version, created_at, updated_at
			) VALUES (?, ?, ?, 1, ?, ?)`,
			params.Source, params.Subject, boolInt(params.CustomPDFPreviewEnabled), now, now)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
				return AccountPreference{}, ErrAccountPreferenceConflict
			}
			return AccountPreference{}, err
		}
	} else if params.ExpectedRowVersion > 0 {
		result, err := r.db.ExecContext(ctx, `
			UPDATE account_preferences
			SET custom_pdf_preview_enabled = ?, row_version = row_version + 1, updated_at = ?
			WHERE account_source = ? AND account_subject = ? AND row_version = ?`,
			boolInt(params.CustomPDFPreviewEnabled), now, params.Source, params.Subject, params.ExpectedRowVersion)
		if err != nil {
			return AccountPreference{}, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return AccountPreference{}, err
		}
		if affected != 1 {
			return AccountPreference{}, ErrAccountPreferenceConflict
		}
	} else {
		return AccountPreference{}, ErrAccountPreferenceConflict
	}
	return r.AccountPreference(ctx, params.Source, params.Subject)
}

func (r *Repository) DeleteAccountPreference(ctx context.Context, source, subject string) error {
	source = strings.TrimSpace(source)
	subject = strings.TrimSpace(subject)
	if err := validateAccountPreferenceKey(source, subject); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM account_preferences WHERE account_source = ? AND account_subject = ?`, source, subject)
	return err
}

func validateAccountPreferenceKey(source, subject string) error {
	source = strings.TrimSpace(source)
	subject = strings.TrimSpace(subject)
	if source != AccountSourceConfig && source != AccountSourceDatabase {
		return errors.New("ungueltige Kontoquelle")
	}
	if subject == "" {
		return errors.New("leeres Kontosubjekt")
	}
	return nil
}

type accountPreferenceScanner interface{ Scan(...any) error }

func scanAccountPreference(scanner accountPreferenceScanner) (AccountPreference, error) {
	var preference AccountPreference
	var enabled int
	var createdAt, updatedAt string
	if err := scanner.Scan(&preference.Source, &preference.Subject, &enabled, &preference.RowVersion, &createdAt, &updatedAt); err != nil {
		return AccountPreference{}, err
	}
	preference.CustomPDFPreviewEnabled = enabled != 0
	var err error
	preference.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return AccountPreference{}, err
	}
	preference.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return AccountPreference{}, err
	}
	return preference, nil
}

var _ accountPreferenceScanner = (*sql.Row)(nil)
