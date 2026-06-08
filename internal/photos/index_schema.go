// Datei definiert und migriert das SQLite-Schema des Fotoindex.
package photos

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bearstack/internal/sqlitedsn"
	"bearstack/internal/sqlitefuncs"
	"bearstack/internal/sqlutil"

	"modernc.org/sqlite"
)

var (
	registerSQLiteFunctionsOnce sync.Once
	registerSQLiteFunctionsErr  error
)

const (
	indexSchemaSetupTimeout = 30 * time.Second
	photoSchemaComponent    = "photos"
	photoSchemaVersion      = 17
)

type photoSchemaMigration struct {
	Version         int
	Name            string
	Table           string
	Column          string
	SQL             string
	BackfillSQL     string
	InvalidateIndex bool
}

var photoSchemaMigrations = []photoSchemaMigration{
	{Version: 2, Name: "media_index.tags", Table: "media_index", Column: "tags", SQL: `ALTER TABLE media_index ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`},
	{Version: 3, Name: "media_index.admin_only", Table: "media_index", Column: "admin_only", SQL: `ALTER TABLE media_index ADD COLUMN admin_only INTEGER NOT NULL DEFAULT 0`},
	{Version: 4, Name: "media_index.rating", Table: "media_index", Column: "rating", SQL: `ALTER TABLE media_index ADD COLUMN rating REAL`, InvalidateIndex: true},
	{Version: 5, Name: "media_index.faces", Table: "media_index", Column: "faces", SQL: `ALTER TABLE media_index ADD COLUMN faces TEXT NOT NULL DEFAULT '[]'`, InvalidateIndex: true},
	{Version: 6, Name: "media_index.xmp_fingerprint", Table: "media_index", Column: "xmp_fingerprint", SQL: `ALTER TABLE media_index ADD COLUMN xmp_fingerprint TEXT NOT NULL DEFAULT ''`, InvalidateIndex: true},
	{Version: 7, Name: "folder_index.tags", Table: "folder_index", Column: "tags", SQL: `ALTER TABLE folder_index ADD COLUMN tags TEXT NOT NULL DEFAULT '[]'`},
	{Version: 8, Name: "folder_index.admin_only", Table: "folder_index", Column: "admin_only", SQL: `ALTER TABLE folder_index ADD COLUMN admin_only INTEGER NOT NULL DEFAULT 0`},
	{Version: 9, Name: "folder_index.public_recursive_media_count", Table: "folder_index", Column: "public_recursive_media_count", SQL: `ALTER TABLE folder_index ADD COLUMN public_recursive_media_count INTEGER NOT NULL DEFAULT -1`, InvalidateIndex: true},
	{Version: 10, Name: "blog_index.admin_only", Table: "blog_index", Column: "admin_only", SQL: `ALTER TABLE blog_index ADD COLUMN admin_only INTEGER NOT NULL DEFAULT 0`},
	{Version: 11, Name: "photo_folder_scan.quick_signature_unix_nano", Table: "photo_folder_scan", Column: "quick_signature_unix_nano", SQL: `ALTER TABLE photo_folder_scan ADD COLUMN quick_signature_unix_nano INTEGER NOT NULL DEFAULT 0`},
	{Version: 12, Name: "photo_tags.color", Table: "photo_tags", Column: "color", SQL: `ALTER TABLE photo_tags ADD COLUMN color TEXT NOT NULL DEFAULT '#176b87'`},
	{Version: 13, Name: "folder_index.public_media_count", Table: "folder_index", Column: "public_media_count", SQL: `ALTER TABLE folder_index ADD COLUMN public_media_count INTEGER NOT NULL DEFAULT -1`, InvalidateIndex: true},
	{Version: 14, Name: "folder_index.recursive_blog_count", Table: "folder_index", Column: "recursive_blog_count", SQL: `ALTER TABLE folder_index ADD COLUMN recursive_blog_count INTEGER NOT NULL DEFAULT 0`, InvalidateIndex: true},
	{Version: 15, Name: "folder_index.public_recursive_blog_count", Table: "folder_index", Column: "public_recursive_blog_count", SQL: `ALTER TABLE folder_index ADD COLUMN public_recursive_blog_count INTEGER NOT NULL DEFAULT -1`, InvalidateIndex: true},
	{Version: 16, Name: "photo_folder_scan.order_mode", Table: "photo_folder_scan", Column: "order_mode", SQL: `ALTER TABLE photo_folder_scan ADD COLUMN order_mode TEXT NOT NULL DEFAULT ''`, InvalidateIndex: true},
	{Version: 17, Name: "media_index.random_hash", Table: "media_index", Column: "random_hash", SQL: `ALTER TABLE media_index ADD COLUMN random_hash TEXT NOT NULL DEFAULT ''`, BackfillSQL: `UPDATE media_index SET random_hash = bearstack_stable_hash(path) WHERE random_hash = ''`},
}

func openIndexDB(path string) (*sql.DB, string, error) {
	if path == "" {
		return nil, "", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return nil, "", err
	}
	if err := registerSQLiteFunctions(); err != nil {
		return nil, "", err
	}
	dsn, err := indexSQLiteDSN(abs)
	if err != nil {
		return nil, "", err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, "", err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), indexSchemaSetupTimeout)
	defer cancel()
	if err := sqlutil.CheckSchemaVersion(ctx, db, photoSchemaComponent, photoSchemaVersion); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS media_index (
		path TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		directory TEXT NOT NULL,
		type TEXT NOT NULL,
		mime_type TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		mod_time_unix_nano INTEGER NOT NULL,
		captured_at TEXT NOT NULL DEFAULT '',
		width INTEGER NOT NULL DEFAULT 0,
		height INTEGER NOT NULL DEFAULT 0,
		orientation TEXT NOT NULL DEFAULT '',
		camera TEXT NOT NULL DEFAULT '',
		lens TEXT NOT NULL DEFAULT '',
		rating REAL,
		latitude REAL,
		longitude REAL,
		keywords TEXT NOT NULL DEFAULT '[]',
		tags TEXT NOT NULL DEFAULT '[]',
		faces TEXT NOT NULL DEFAULT '[]',
		xmp_fingerprint TEXT NOT NULL DEFAULT '',
		admin_only INTEGER NOT NULL DEFAULT 0,
		random_hash TEXT NOT NULL DEFAULT '',
		indexed_at TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS folder_index (
		path TEXT PRIMARY KEY,
		parent TEXT NOT NULL,
		name TEXT NOT NULL,
		media_count INTEGER NOT NULL DEFAULT 0,
		public_media_count INTEGER NOT NULL DEFAULT -1,
		recursive_media_count INTEGER NOT NULL DEFAULT 0,
		public_recursive_media_count INTEGER NOT NULL DEFAULT -1,
		recursive_blog_count INTEGER NOT NULL DEFAULT 0,
		public_recursive_blog_count INTEGER NOT NULL DEFAULT -1,
		dir_count INTEGER NOT NULL DEFAULT 0,
		mod_time_unix_nano INTEGER NOT NULL DEFAULT 0,
		order_mode TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		admin_only INTEGER NOT NULL DEFAULT 0,
		indexed_at TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS photo_folder_scan (
		path TEXT PRIMARY KEY,
		mod_time_unix_nano INTEGER NOT NULL,
		quick_signature_unix_nano INTEGER NOT NULL DEFAULT 0,
		order_mode TEXT NOT NULL DEFAULT '',
		scanned_at TEXT NOT NULL
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS blog_index (
		path TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		directory TEXT NOT NULL,
		date TEXT NOT NULL DEFAULT '',
		mod_time_unix_nano INTEGER NOT NULL,
		text TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '[]',
		admin_only INTEGER NOT NULL DEFAULT 0,
		indexed_at TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS photo_stats (
		key TEXT PRIMARY KEY,
		value INTEGER NOT NULL
	)`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS folder_preview_index (
		folder_path TEXT NOT NULL,
		rank INTEGER NOT NULL,
		media_path TEXT NOT NULL,
		PRIMARY KEY(folder_path, rank)
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS photo_thumbnail_index (
		media_path TEXT NOT NULL,
		size INTEGER NOT NULL,
		quality INTEGER NOT NULL DEFAULT 80,
		source_mod_time_unix_nano INTEGER NOT NULL DEFAULT 0,
		source_size_bytes INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'queued',
		attempts INTEGER NOT NULL DEFAULT 0,
		last_attempt_at TEXT NOT NULL DEFAULT '',
		generated_at TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		priority INTEGER NOT NULL DEFAULT 100,
		requested_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY(media_path, size)
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS photo_tags (
		name TEXT PRIMARY KEY,
		color TEXT NOT NULL DEFAULT '#176b87',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS media_tag_index (
		media_path TEXT NOT NULL,
		tag TEXT NOT NULL,
		PRIMARY KEY(media_path, tag)
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS folder_tag_index (
		folder_path TEXT NOT NULL,
		tag TEXT NOT NULL,
		PRIMARY KEY(folder_path, tag)
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS blog_tag_index (
		blog_path TEXT NOT NULL,
		tag TEXT NOT NULL,
		PRIMARY KEY(blog_path, tag)
	) WITHOUT ROWID`); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	if err := runPhotoSchemaMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_media_index_directory_date ON media_index(directory, captured_at DESC, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_directory_name ON media_index(directory, name COLLATE NOCASE, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_directory_type_date ON media_index(directory, type, captured_at DESC, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_date ON media_index(captured_at, mod_time_unix_nano, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_admin_date ON media_index(admin_only, captured_at, mod_time_unix_nano, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_type_date ON media_index(type, captured_at DESC, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_admin_directory_date ON media_index(admin_only, directory, captured_at DESC, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_admin_directory_name ON media_index(admin_only, directory, name COLLATE NOCASE, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_admin_gps_date ON media_index(admin_only, captured_at DESC, path) WHERE latitude IS NOT NULL AND longitude IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_gps ON media_index(latitude, longitude) WHERE latitude IS NOT NULL AND longitude IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_gps_date ON media_index(captured_at DESC, path) WHERE latitude IS NOT NULL AND longitude IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_random ON media_index(random_hash, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_admin_random ON media_index(admin_only, random_hash, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_directory_random ON media_index(directory, random_hash, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_index_admin_directory_random ON media_index(admin_only, directory, random_hash, path)`,
		`CREATE INDEX IF NOT EXISTS idx_folder_index_parent_name ON folder_index(parent, name COLLATE NOCASE)`,
		`CREATE INDEX IF NOT EXISTS idx_folder_index_parent_admin_name ON folder_index(parent, admin_only, name COLLATE NOCASE)`,
		`CREATE INDEX IF NOT EXISTS idx_folder_index_admin_path ON folder_index(admin_only, path)`,
		`CREATE INDEX IF NOT EXISTS idx_folder_preview_index_media ON folder_preview_index(media_path)`,
		`CREATE INDEX IF NOT EXISTS idx_photo_thumbnail_index_status ON photo_thumbnail_index(size, quality, status, priority, requested_at)`,
		`CREATE INDEX IF NOT EXISTS idx_photo_thumbnail_index_media ON photo_thumbnail_index(media_path)`,
		`CREATE INDEX IF NOT EXISTS idx_blog_index_directory_date ON blog_index(directory, date DESC, path)`,
		`CREATE INDEX IF NOT EXISTS idx_blog_index_admin_directory_date ON blog_index(admin_only, directory, date DESC, path)`,
		`CREATE INDEX IF NOT EXISTS idx_media_tag_index_tag ON media_tag_index(tag, media_path)`,
		`CREATE INDEX IF NOT EXISTS idx_folder_tag_index_tag ON folder_tag_index(tag, folder_path)`,
		`CREATE INDEX IF NOT EXISTS idx_blog_tag_index_tag ON blog_tag_index(tag, blog_path)`,
		`CREATE INDEX IF NOT EXISTS idx_photo_tags_name ON photo_tags(name)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS media_search USING fts5(path UNINDEXED, search_text, tokenize='trigram')`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS folder_search USING fts5(path UNINDEXED, search_text, tokenize='trigram')`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS blog_search USING fts5(path UNINDEXED, search_text, tokenize='trigram')`,
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			_ = db.Close()
			return nil, "", err
		}
	}
	if err := sqlutil.RecordSchemaVersion(ctx, db, photoSchemaComponent, photoSchemaVersion); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	return db, abs, nil
}

func runPhotoSchemaMigrations(ctx context.Context, db *sql.DB) error {
	current, found, err := sqlutil.CurrentSchemaVersion(ctx, db, photoSchemaComponent)
	if err != nil {
		return err
	}
	if !found {
		current = 1
	}
	for _, migration := range photoSchemaMigrations {
		if current >= migration.Version {
			continue
		}
		added, err := ensurePhotoColumn(ctx, db, migration.Table, migration.Column, migration.SQL)
		if err != nil {
			return fmt.Errorf("photo schema migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		if added && migration.InvalidateIndex {
			if err := invalidatePhotoIndexForMigration(ctx, db, migration); err != nil {
				return fmt.Errorf("photo schema migration %d (%s): %w", migration.Version, migration.Name, err)
			}
		}
		if added && migration.BackfillSQL != "" {
			if _, err := db.ExecContext(ctx, migration.BackfillSQL); err != nil {
				return fmt.Errorf("photo schema migration %d (%s) backfill: %w", migration.Version, migration.Name, err)
			}
		}
	}
	return nil
}

func invalidatePhotoIndexForMigration(ctx context.Context, db *sql.DB, migration photoSchemaMigration) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM photo_folder_scan`); err != nil {
		return err
	}
	if migration.Table == "media_index" {
		if _, err := db.ExecContext(ctx, `UPDATE media_index SET mod_time_unix_nano = 0`); err != nil {
			return err
		}
	}
	return nil
}

func indexSQLiteDSN(abs string) (string, error) {
	return sqlitedsn.FilePath(
		abs,
		"busy_timeout(5000)",
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"temp_store(MEMORY)",
	)
}

func registerSQLiteFunctions() error {
	registerSQLiteFunctionsOnce.Do(func() {
		if err := sqlitefuncs.RegisterGermanFold(); err != nil {
			registerSQLiteFunctionsErr = err
			return
		}
		registerSQLiteFunctionsErr = sqlite.RegisterDeterministicScalarFunction("bearstack_stable_hash", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if len(args) != 1 {
				return nil, nil
			}
			value, ok := args[0].(string)
			if !ok {
				return nil, nil
			}
			return stableHashKey(value), nil
		})
	})
	return registerSQLiteFunctionsErr
}

func ensurePhotoColumn(ctx context.Context, db *sql.DB, table, column, alterSQL string) (bool, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
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
			return false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if _, err := db.ExecContext(ctx, alterSQL); err != nil {
		return false, err
	}
	return true, nil
}
