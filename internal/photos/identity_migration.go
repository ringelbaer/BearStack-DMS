package photos

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type retainedColumn struct {
	name, kind   string
	notNull      int
	defaultValue sql.NullString
}

func (c retainedColumn) definition() string {
	ddl := quotePhotoColumn(c.name) + " " + c.kind
	if c.notNull != 0 {
		ddl += " NOT NULL"
	}
	if c.defaultValue.Valid {
		ddl += " DEFAULT " + c.defaultValue.String
	}
	return ddl
}

type schemaQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func quotePhotoColumn(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func photoTableColumns(ctx context.Context, q schemaQuerier, table string) ([]retainedColumn, error) {
	rows, err := q.QueryContext(ctx, `PRAGMA table_info(`+quotePhotoColumn(table)+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []retainedColumn
	for rows.Next() {
		var c retainedColumn
		var cid, pk int
		if err := rows.Scan(&cid, &c.name, &c.kind, &c.notNull, &c.defaultValue, &pk); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func loadRetainedColumns(ctx context.Context, q schemaQuerier) (map[string]string, error) {
	out := map[string]string{}
	for _, spec := range retainedTables {
		cols, err := photoTableColumns(ctx, q, spec.table)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, c := range cols {
			names = append(names, quotePhotoColumn(c.name))
		}
		out[spec.table] = strings.Join(names, ",")
	}
	return out, nil
}

func photoColumnAffinity(kind string) string {
	kind = strings.ToUpper(kind)
	switch {
	case strings.Contains(kind, "INT"):
		return "INTEGER"
	case strings.Contains(kind, "CHAR"), strings.Contains(kind, "CLOB"), strings.Contains(kind, "TEXT"):
		return "TEXT"
	case kind == "", strings.Contains(kind, "BLOB"):
		return "BLOB"
	case strings.Contains(kind, "REAL"), strings.Contains(kind, "FLOA"), strings.Contains(kind, "DOUB"):
		return "REAL"
	default:
		return "NUMERIC"
	}
}

// Every versioned photo migration finishes here. Additive columns inherit their
// declared defaults; drops/type changes require an explicit data migration.
// Existing retained rows and IDs are never rebuilt or discarded implicitly.
func syncRetainedTablesTx(ctx context.Context, tx *sql.Tx) error {
	for _, spec := range retainedTables {
		live, err := photoTableColumns(ctx, tx, spec.table)
		if err != nil {
			return err
		}
		if len(live) == 0 {
			return fmt.Errorf("missing live table %s", spec.table)
		}
		table := "photo_retained_" + spec.table
		old, err := photoTableColumns(ctx, tx, table)
		if err != nil {
			return err
		}
		if len(old) == 0 {
			definitions := []string{"retention_id INTEGER NOT NULL"}
			for _, c := range live {
				definitions = append(definitions, c.definition())
			}
			if _, err := tx.ExecContext(ctx, `CREATE TABLE `+table+` (`+strings.Join(definitions, ",")+`)`); err != nil {
				return err
			}
			old = append([]retainedColumn{{name: "retention_id", kind: "INTEGER"}}, live...)
		}
		known := map[string]retainedColumn{}
		for _, c := range old {
			known[c.name] = c
		}
		if _, ok := known["retention_id"]; !ok {
			return fmt.Errorf("retained table %s is missing retention_id", table)
		}
		delete(known, "retention_id")
		for _, c := range live {
			if before, ok := known[c.name]; ok {
				if photoColumnAffinity(before.kind) != photoColumnAffinity(c.kind) {
					return fmt.Errorf("retained column %s.%s needs explicit type migration", table, c.name)
				}
				delete(known, c.name)
				continue
			}
			ddl := `ALTER TABLE ` + table + ` ADD COLUMN ` + c.definition()
			if _, err := tx.ExecContext(ctx, ddl); err != nil {
				return fmt.Errorf("retained column %s.%s: %w", table, c.name, err)
			}
		}
		if len(known) > 0 {
			return fmt.Errorf("retained table %s needs explicit column removal migration", table)
		}
		for _, column := range []string{"retention_id", spec.key} {
			index := "idx_retained_" + spec.table
			if column != "retention_id" {
				index += "_" + column
			}
			if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS `+index+` ON `+table+`(`+column+`)`); err != nil {
				return err
			}
		}
	}
	return nil
}

func setupIdentityMaintenance(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := syncRetainedTablesTx(ctx, tx); err != nil {
		return err
	}
	if err := setupIdentityPrivacyTx(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS photo_identity_state;
 CREATE INDEX IF NOT EXISTS idx_retained_face_person ON photo_retained_photo_faces(person_id)`); err != nil {
		return err
	}
	for _, spec := range []struct{ table, counts, count string }{{"photo_faces", "photo_face_directories", "face_count"}, {"photo_face_jobs", "photo_face_job_directories", "job_count"}} {
		stmt := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %s_directory_update AFTER UPDATE OF directory ON %s WHEN new.directory<>old.directory BEGIN
 UPDATE %s SET %s=%s-1 WHERE directory=old.directory;
 DELETE FROM %s WHERE directory=old.directory AND %s<=0;
 INSERT INTO %s(directory,%s) VALUES(new.directory,1) ON CONFLICT(directory) DO UPDATE SET %s=%s+1; END`, spec.table, spec.table, spec.counts, spec.count, spec.count, spec.counts, spec.count, spec.counts, spec.count, spec.count, spec.count)
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
