// Datei verwaltet Schema-Versionen und Migrationsmarken fuer SQLite-Datenbanken.
package sqlutil

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func CheckSchemaVersion(ctx context.Context, db *sql.DB, component string, supportedVersion int) error {
	if err := validateSchemaVersionInput(component, supportedVersion); err != nil {
		return err
	}
	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		return err
	}
	version, found, err := CurrentSchemaVersion(ctx, db, component)
	if err != nil {
		return err
	}
	if found && version > supportedVersion {
		return newerSchemaVersionError(component, version, supportedVersion)
	}
	return nil
}

func RecordSchemaVersion(ctx context.Context, db *sql.DB, component string, version int) error {
	if err := validateSchemaVersionInput(component, version); err != nil {
		return err
	}
	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		return err
	}
	current, found, err := CurrentSchemaVersion(ctx, db, component)
	if err != nil {
		return err
	}
	if found && current > version {
		return newerSchemaVersionError(component, current, version)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO schema_migrations(component, version, applied_at)
		VALUES (?, ?, ?)
		ON CONFLICT(component) DO UPDATE SET
			version = excluded.version,
			applied_at = excluded.applied_at
		WHERE schema_migrations.version <> excluded.version`,
		component,
		version,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func CurrentSchemaVersion(ctx context.Context, db *sql.DB, component string) (int, bool, error) {
	component = strings.TrimSpace(component)
	if component == "" {
		return 0, false, fmt.Errorf("schema component must not be empty")
	}
	if err := ensureSchemaMigrationsTable(ctx, db); err != nil {
		return 0, false, err
	}
	var version int
	err := db.QueryRowContext(ctx, `
		SELECT version
		FROM schema_migrations
		WHERE component = ?`, component).Scan(&version)
	if err == nil {
		return version, true, nil
	}
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	return 0, false, err
}

func ensureSchemaMigrationsTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		component TEXT PRIMARY KEY,
		version INTEGER NOT NULL,
		applied_at TEXT NOT NULL
	)`)
	return err
}

func validateSchemaVersionInput(component string, version int) error {
	if strings.TrimSpace(component) == "" {
		return fmt.Errorf("schema component must not be empty")
	}
	if version < 1 {
		return fmt.Errorf("schema version must be positive")
	}
	return nil
}

func newerSchemaVersionError(component string, current, supported int) error {
	return fmt.Errorf("schema %q version %d is newer than supported version %d", component, current, supported)
}
