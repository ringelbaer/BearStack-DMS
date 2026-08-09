// Datei definiert und migriert das SQLite-Schema des Dokument-Repositories.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"bearstack/internal/document"
	"bearstack/internal/sqlutil"
)

const (
	repositorySchemaComponent = "repository"
	repositorySchemaVersion   = 16
)

type repositorySchemaMigration struct {
	Version int
	Name    string
	Always  bool
	Apply   func(context.Context, *Repository) error
}

var repositorySchemaMigrations = []repositorySchemaMigration{
	{Version: 2, Name: "documents.upload_way", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureDocumentUploadWayColumn(ctx)
	}},
	{Version: 3, Name: "tags.group_mode", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureTagGroupModeColumn(ctx)
	}},
	{Version: 4, Name: "tags.list_hidden", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureTagListHiddenColumn(ctx)
	}},
	{Version: 5, Name: "tags.delete_protected", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureTagDeleteProtectedColumn(ctx)
	}},
	{Version: 6, Name: "ocr_jobs.message", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureOCRJobMessageColumn(ctx)
	}},
	{Version: 7, Name: "documents.content_text_source", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureDocumentContentTextSourceColumn(ctx)
	}},
	{Version: 8, Name: "custom_fields.autocomplete_enabled", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureCustomFieldAutocompleteColumn(ctx)
	}},
	{Version: 9, Name: "custom_fields.value_folder_min_documents", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureCustomFieldValueFolderColumn(ctx)
	}},
	{Version: 10, Name: "search_favorites.custom_fields", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureSearchFavoriteCustomFieldsColumn(ctx)
	}},
	{Version: 11, Name: "tag_rules.exclude_keywords", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureTagRuleExcludeKeywordsColumn(ctx)
	}},
	{Version: 12, Name: "document_search.trigram", Always: true, Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureDocumentSearchTable(ctx)
	}},
	{Version: 13, Name: "documents.post_import_pending", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureDocumentPostImportPendingColumn(ctx)
	}},
	{Version: 14, Name: "tags.primary_tag", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureTagPrimaryTagColumn(ctx)
	}},
	{Version: 15, Name: "users", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureUserTables(ctx)
	}},
	{Version: 16, Name: "account_preferences", Apply: func(ctx context.Context, r *Repository) error {
		return r.ensureAccountPreferenceTable(ctx)
	}},
}

func (r *Repository) configure(ctx context.Context) error {
	pragmas := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
	}
	for _, pragma := range pragmas {
		if _, err := r.db.ExecContext(ctx, pragma); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ensureSchema(ctx context.Context) error {
	if err := sqlutil.CheckSchemaVersion(ctx, r.db, repositorySchemaComponent, repositorySchemaVersion); err != nil {
		return err
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			original_name TEXT NOT NULL,
			stored_path TEXT NOT NULL UNIQUE,
			thumbnail_path TEXT NOT NULL DEFAULT '',
			upload_way TEXT NOT NULL DEFAULT 'web',
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			mime_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			document_date TEXT,
			uploaded_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted_at TEXT,
			content_text TEXT NOT NULL DEFAULT '',
			content_text_source TEXT NOT NULL DEFAULT 'none',
			search_version INTEGER NOT NULL DEFAULT 0,
			post_import_pending INTEGER NOT NULL DEFAULT 0,
			post_import_attempted_at TEXT,
			post_import_attempts INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_uploaded_at ON documents(uploaded_at)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_deleted_uploaded_at ON documents(deleted_at, uploaded_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_document_date ON documents(document_date)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_deleted_sort_date ON documents(deleted_at, COALESCE(document_date, substr(uploaded_at, 1, 10)) DESC, uploaded_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_deleted_at ON documents(deleted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_sha256 ON documents(sha256)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_active_sha256 ON documents(sha256, id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_documents_deleted_document_date ON documents(deleted_at, document_date) WHERE document_date IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_documents_thumbnail_candidates ON documents(mime_type, thumbnail_path, uploaded_at, id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_documents_active_original_name ON documents(lower(original_name), id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_documents_trash_original_name ON documents(lower(original_name), id) WHERE deleted_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_documents_active_title ON documents(lower(title), id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_documents_trash_title ON documents(lower(title), id) WHERE deleted_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_documents_active_size ON documents(size_bytes, id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_documents_trash_size ON documents(size_bytes, id) WHERE deleted_at IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS document_links (
			source_document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			target_document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			PRIMARY KEY(source_document_id, target_document_id),
			CHECK(source_document_id < target_document_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_document_links_target ON document_links(target_document_id)`,
		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			color TEXT NOT NULL DEFAULT '#176b87',
			primary_tag INTEGER NOT NULL DEFAULT 0,
			group_mode INTEGER NOT NULL DEFAULT 0,
			list_hidden INTEGER NOT NULL DEFAULT 0,
			delete_protected INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS document_tags (
			document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			PRIMARY KEY(document_id, tag_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_document_tags_tag_id ON document_tags(tag_id)`,
		`CREATE INDEX IF NOT EXISTS idx_document_tags_tag_document ON document_tags(tag_id, document_id)`,
		`CREATE TABLE IF NOT EXISTS tag_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
			label TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL,
			match_mode TEXT NOT NULL,
			keywords TEXT NOT NULL,
			exclude_keywords TEXT NOT NULL DEFAULT '',
			position INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tag_rules_tag_id ON tag_rules(tag_id)`,
		`CREATE TABLE IF NOT EXISTS custom_fields (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				label TEXT NOT NULL UNIQUE,
				position INTEGER NOT NULL DEFAULT 0,
				autocomplete_enabled INTEGER NOT NULL DEFAULT 0,
				value_folder_min_documents INTEGER NOT NULL DEFAULT 0
			)`,
		`CREATE TABLE IF NOT EXISTS search_favorites (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				query TEXT NOT NULL DEFAULT '',
				tags TEXT NOT NULL DEFAULT '[]',
				custom_fields TEXT NOT NULL DEFAULT '[]',
				date_mode TEXT NOT NULL DEFAULT '',
				date_year INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_search_favorites_name_lower ON search_favorites(lower(name))`,
		`CREATE TABLE IF NOT EXISTS document_custom_values (
				document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
				field_id INTEGER NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
			value TEXT NOT NULL DEFAULT '',
			PRIMARY KEY(document_id, field_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_document_custom_values_field_id ON document_custom_values(field_id)`,
		`CREATE INDEX IF NOT EXISTS idx_document_custom_values_field_value_document ON document_custom_values(field_id, value, document_id)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			occurred_at TEXT NOT NULL,
			actor TEXT NOT NULL DEFAULT '',
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			route TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			target TEXT NOT NULL DEFAULT '',
			status INTEGER NOT NULL DEFAULT 0,
			remote_addr TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_occurred_at ON audit_logs(occurred_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
			session_version INTEGER NOT NULL DEFAULT 1 CHECK(session_version > 0),
			row_version INTEGER NOT NULL DEFAULT 1 CHECK(row_version > 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS user_permissions (
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			permission TEXT NOT NULL,
			PRIMARY KEY(user_id, permission)
		)`,
		`CREATE TABLE IF NOT EXISTS account_preferences (
			account_source TEXT NOT NULL CHECK(account_source IN ('config', 'database')),
			account_subject TEXT NOT NULL,
			custom_pdf_preview_enabled INTEGER NOT NULL DEFAULT 0 CHECK(custom_pdf_preview_enabled IN (0, 1)),
			row_version INTEGER NOT NULL DEFAULT 1 CHECK(row_version > 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(account_source, account_subject),
			CHECK(length(account_subject) > 0)
		)`,
		`CREATE TABLE IF NOT EXISTS ocr_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			document_id INTEGER NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			language TEXT NOT NULL,
			language_label TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			current_page INTEGER NOT NULL DEFAULT 0,
			total_pages INTEGER NOT NULL DEFAULT 0,
			text_length INTEGER NOT NULL DEFAULT 0,
			message TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ocr_jobs_document_updated ON ocr_jobs(document_id, updated_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ocr_jobs_document_id_desc ON ocr_jobs(document_id, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ocr_jobs_status_created ON ocr_jobs(status, created_at ASC, id ASC)`,
	}
	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := r.runSchemaMigrations(ctx); err != nil {
		return err
	}
	return sqlutil.RecordSchemaVersion(ctx, r.db, repositorySchemaComponent, repositorySchemaVersion)
}

func (r *Repository) ensureUserTables(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
			session_version INTEGER NOT NULL DEFAULT 1 CHECK(session_version > 0),
			row_version INTEGER NOT NULL DEFAULT 1 CHECK(row_version > 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS user_permissions (
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			permission TEXT NOT NULL,
			PRIMARY KEY(user_id, permission)
		)`,
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ensureAccountPreferenceTable(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS account_preferences (
		account_source TEXT NOT NULL CHECK(account_source IN ('config', 'database')),
		account_subject TEXT NOT NULL,
		custom_pdf_preview_enabled INTEGER NOT NULL DEFAULT 0 CHECK(custom_pdf_preview_enabled IN (0, 1)),
		row_version INTEGER NOT NULL DEFAULT 1 CHECK(row_version > 0),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY(account_source, account_subject),
		CHECK(length(account_subject) > 0)
	)`)
	return err
}

func (r *Repository) runSchemaMigrations(ctx context.Context) error {
	current, found, err := sqlutil.CurrentSchemaVersion(ctx, r.db, repositorySchemaComponent)
	if err != nil {
		return err
	}
	if !found {
		current = 1
	}
	for _, migration := range repositorySchemaMigrations {
		if current >= migration.Version && !migration.Always {
			continue
		}
		if err := migration.Apply(ctx, r); err != nil {
			return fmt.Errorf("repository schema migration %d (%s): %w", migration.Version, migration.Name, err)
		}
	}
	return nil
}

func (r *Repository) ensureDocumentSearchTable(ctx context.Context) error {
	exists, usesTrigram, err := r.documentSearchTableStatus(ctx)
	if err != nil {
		return err
	}
	rebuild := !exists || !usesTrigram
	if exists && !usesTrigram {
		if _, err := r.db.ExecContext(ctx, `DROP TABLE document_search`); err != nil {
			return err
		}
	}
	if _, err := r.db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS document_search USING fts5(
		original_name,
		title,
		description,
		tags,
		search_text,
		tokenize='trigram'
	)`); err != nil {
		return err
	}
	if rebuild {
		return r.RebuildSearchIndex(ctx)
	}
	return nil
}

func (r *Repository) documentSearchUsesTrigram(ctx context.Context) (bool, error) {
	_, usesTrigram, err := r.documentSearchTableStatus(ctx)
	return usesTrigram, err
}

func (r *Repository) documentSearchTableStatus(ctx context.Context) (bool, bool, error) {
	var createSQL string
	err := r.db.QueryRowContext(ctx, `
		SELECT sql
		FROM sqlite_master
		WHERE type = 'table'
		  AND name = 'document_search'`).Scan(&createSQL)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	createSQL = strings.ToLower(createSQL)
	return true, strings.Contains(createSQL, "using fts5") && strings.Contains(createSQL, "trigram"), nil
}

func (r *Repository) ensureDocumentUploadWayColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "documents", "upload_way", `ALTER TABLE documents ADD COLUMN upload_way TEXT NOT NULL DEFAULT 'web'`)
}

func (r *Repository) documentColumnExists(ctx context.Context, column string) (bool, error) {
	return r.columnExists(ctx, "documents", column)
}

func (r *Repository) ensureDocumentContentTextSourceColumn(ctx context.Context) error {
	exists, err := r.documentColumnExists(ctx, "content_text_source")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := r.db.ExecContext(ctx, `ALTER TABLE documents ADD COLUMN content_text_source TEXT NOT NULL DEFAULT 'none'`); err != nil {
			return err
		}
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE documents
		SET content_text_source = CASE
			WHEN trim(content_text) = '' THEN ?
			WHEN trim(content_text_source) = '' OR content_text_source = ? THEN ?
			WHEN content_text_source IN (?, ?, ?, ?, ?) THEN content_text_source
			ELSE ?
		END`,
		document.ContentTextSourceNone,
		document.ContentTextSourceNone,
		document.ContentTextSourceUnknown,
		document.ContentTextSourcePDF,
		document.ContentTextSourceFile,
		document.ContentTextSourceRaw,
		document.ContentTextSourceOCR,
		document.ContentTextSourceUnknown,
		document.ContentTextSourceUnknown,
	)
	return err
}

func (r *Repository) ensureDocumentPostImportPendingColumn(ctx context.Context) error {
	if err := r.ensureColumn(ctx, "documents", "post_import_pending", `ALTER TABLE documents ADD COLUMN post_import_pending INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if err := r.ensureColumn(ctx, "documents", "post_import_attempted_at", `ALTER TABLE documents ADD COLUMN post_import_attempted_at TEXT`); err != nil {
		return err
	}
	if err := r.ensureColumn(ctx, "documents", "post_import_attempts", `ALTER TABLE documents ADD COLUMN post_import_attempts INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_documents_post_import_pending`); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_documents_post_import_pending ON documents(post_import_attempted_at, uploaded_at, id) WHERE deleted_at IS NULL AND post_import_pending = 1`); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE documents
		SET post_import_pending = 1
		WHERE deleted_at IS NULL
		  AND post_import_pending = 0
		  AND (
			(mime_type NOT LIKE 'image/%' AND content_text_source = ?)
			OR (mime_type IN ('application/pdf', 'image/jpeg', 'image/png', 'image/gif') AND thumbnail_path = '')
		  )`,
		document.ContentTextSourceNone,
	)
	return err
}

func (r *Repository) ensureTagGroupModeColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "tags", "group_mode", `ALTER TABLE tags ADD COLUMN group_mode INTEGER NOT NULL DEFAULT 0`)
}

func (r *Repository) ensureTagPrimaryTagColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "tags", "primary_tag", `ALTER TABLE tags ADD COLUMN primary_tag INTEGER NOT NULL DEFAULT 0`)
}

func (r *Repository) ensureTagListHiddenColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "tags", "list_hidden", `ALTER TABLE tags ADD COLUMN list_hidden INTEGER NOT NULL DEFAULT 0`)
}

func (r *Repository) ensureTagDeleteProtectedColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "tags", "delete_protected", `ALTER TABLE tags ADD COLUMN delete_protected INTEGER NOT NULL DEFAULT 0`)
}

func (r *Repository) ensureTagRuleExcludeKeywordsColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "tag_rules", "exclude_keywords", `ALTER TABLE tag_rules ADD COLUMN exclude_keywords TEXT NOT NULL DEFAULT ''`)
}

func (r *Repository) ensureCustomFieldAutocompleteColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "custom_fields", "autocomplete_enabled", `ALTER TABLE custom_fields ADD COLUMN autocomplete_enabled INTEGER NOT NULL DEFAULT 0`)
}

func (r *Repository) ensureCustomFieldValueFolderColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "custom_fields", "value_folder_min_documents", `ALTER TABLE custom_fields ADD COLUMN value_folder_min_documents INTEGER NOT NULL DEFAULT 0`)
}

func (r *Repository) ensureSearchFavoriteCustomFieldsColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "search_favorites", "custom_fields", `ALTER TABLE search_favorites ADD COLUMN custom_fields TEXT NOT NULL DEFAULT '[]'`)
}

func (r *Repository) ensureOCRJobMessageColumn(ctx context.Context) error {
	return r.ensureColumn(ctx, "ocr_jobs", "message", `ALTER TABLE ocr_jobs ADD COLUMN message TEXT NOT NULL DEFAULT ''`)
}

func (r *Repository) ensureColumn(ctx context.Context, table, column, alterSQL string) error {
	exists, err := r.columnExists(ctx, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = r.db.ExecContext(ctx, alterSQL)
	return err
}

func (r *Repository) columnExists(ctx context.Context, table, column string) (bool, error) {
	rows, err := r.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	return columnExists(rows, err, column)
}

func columnExists(rows *sql.Rows, err error, column string) (bool, error) {
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
