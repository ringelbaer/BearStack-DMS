package photos

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// The live tables remain compact serving indexes. Retained rows are stored in
// separate, typed tables under their immutable entity ID, not in the filesystem.
// This also keeps missing entries out of every existing search/count/worker.
type retainedTable struct{ table, key, kind string }

var retainedTables = []retainedTable{
	{"media_index", "path", "image"}, {"media_tag_index", "media_path", "media"},
	{"photo_thumbnail_index", "media_path", "media"}, {"photo_faces", "path", "media"},
	{"photo_face_jobs", "path", "media"},
	{"folder_index", "path", "folder"}, {"folder_tag_index", "folder_path", "folder"},
	{"blog_index", "path", "blog"}, {"blog_tag_index", "blog_path", "blog"},
	{"gpx_index", "path", "gpx"},
}

func setupPhotoIdentity(ctx context.Context, db *sql.DB) error {
	if _, err := ensurePhotoColumn(ctx, db, "photo_thumbnail_index", "content_verified", "ALTER TABLE photo_thumbnail_index ADD COLUMN content_verified INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	for _, col := range []struct{ name, ddl string }{
		{"entity_id", "INTEGER NOT NULL DEFAULT 0"},
		{"needs_review", "INTEGER NOT NULL DEFAULT 0"},
		{"embedding_current", "INTEGER NOT NULL DEFAULT 1"},
		{"source_revision", "INTEGER NOT NULL DEFAULT 1"},
		{"source_hash", "TEXT NOT NULL DEFAULT ''"},
		{"source_path", "TEXT NOT NULL DEFAULT ''"},
		{"source_size", "INTEGER NOT NULL DEFAULT 0"},
		{"source_mtime", "INTEGER NOT NULL DEFAULT 0"},
	} {
		if _, err := ensurePhotoColumn(ctx, db, "photo_faces", col.name, "ALTER TABLE photo_faces ADD COLUMN "+col.name+" "+col.ddl); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS photo_entities (
 id INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT NOT NULL, path TEXT NOT NULL,
 cache_path TEXT NOT NULL, fingerprint TEXT NOT NULL DEFAULT '', size_bytes INTEGER NOT NULL DEFAULT 0,
 mtime INTEGER NOT NULL DEFAULT 0, revision INTEGER NOT NULL DEFAULT 1, missing_since INTEGER NOT NULL DEFAULT 0,
 manual_revision INTEGER NOT NULL DEFAULT 0, UNIQUE(kind,path))`,
		`CREATE INDEX IF NOT EXISTS idx_photo_entity_hash ON photo_entities(fingerprint,kind) WHERE fingerprint<>''`,
		`CREATE INDEX IF NOT EXISTS idx_photo_entity_path ON photo_entities(path)`,
		`CREATE INDEX IF NOT EXISTS idx_photo_entity_missing ON photo_entities(missing_since,id) WHERE missing_since>0`,
		`CREATE TABLE IF NOT EXISTS photo_identity_scan (path TEXT PRIMARY KEY, directory TEXT NOT NULL, kind TEXT NOT NULL, fingerprint TEXT NOT NULL, size_bytes INTEGER NOT NULL, mtime INTEGER NOT NULL, signature TEXT NOT NULL) WITHOUT ROWID`,
		`CREATE INDEX IF NOT EXISTS idx_photo_scan_directory ON photo_identity_scan(directory)`,
		`CREATE TABLE IF NOT EXISTS photo_identity_directories(path TEXT PRIMARY KEY, signature TEXT NOT NULL) WITHOUT ROWID`,
		`CREATE INDEX IF NOT EXISTS idx_photo_scan_hash ON photo_identity_scan(fingerprint,kind) WHERE fingerprint<>''`,
		`CREATE TABLE IF NOT EXISTS photo_relocations (id INTEGER PRIMARY KEY AUTOINCREMENT, entity_id INTEGER NOT NULL, old_path TEXT NOT NULL, new_path TEXT NOT NULL, created_at INTEGER NOT NULL, automatic INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS photo_cache_gc (path TEXT PRIMARY KEY, created_at INTEGER NOT NULL) WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS photo_hidden_person_names(person_id INTEGER PRIMARY KEY,name TEXT NOT NULL,name_fold TEXT NOT NULL,name_source TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS photo_identity_state (id INTEGER PRIMARY KEY CHECK(id=1), revision INTEGER NOT NULL DEFAULT 1, last_complete INTEGER NOT NULL DEFAULT 0); INSERT OR IGNORE INTO photo_identity_state(id) VALUES(1)`,
		`CREATE TABLE IF NOT EXISTS photo_relocation_plan(id INTEGER PRIMARY KEY,path TEXT NOT NULL,admin_only INTEGER NOT NULL)`,
	}
	for _, stmt := range statements {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	for _, spec := range []struct{ table, kind, size, mtime string }{
		{"media_index", "type", "size_bytes", "mod_time_unix_nano"},
		{"folder_index", "'folder'", "0", "mod_time_unix_nano"},
		{"blog_index", "'blog'", "0", "mod_time_unix_nano"},
		{"gpx_index", "'gpx'", "size_bytes", "mod_time_unix_nano"},
	} {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(`INSERT OR IGNORE INTO photo_entities(kind,path,cache_path,size_bytes,mtime) SELECT %s,path,path,%s,%s FROM %s`, spec.kind, spec.size, spec.mtime, spec.table)); err != nil {
			return err
		}
		kind := spec.kind
		if kind == "type" {
			kind = "new.type"
		}
		if _, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS identity_%s_insert AFTER INSERT ON %s WHEN NOT EXISTS(SELECT 1 FROM photo_entities WHERE kind=%s AND path=new.path) BEGIN
 INSERT INTO photo_entities(kind,path,cache_path) VALUES(%s,new.path,new.path);
 UPDATE photo_entities SET cache_path='#entity/'||id WHERE kind=%s AND path=new.path; END`, spec.table, spec.table, kind, kind, kind)); err != nil {
			return err
		}
		if spec.table != "gpx_index" {
			if _, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS identity_%s_tags AFTER UPDATE OF tags ON %s WHEN new.tags<>old.tags BEGIN UPDATE photo_entities SET manual_revision=manual_revision+1,revision=revision+1 WHERE path=new.path AND kind=%s; END`, spec.table, spec.table, kind)); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_faces SET entity_id=coalesce((SELECT id FROM photo_entities WHERE path=photo_faces.path AND kind='image'),0),source_path=path,
 source_size=coalesce((SELECT size_bytes FROM media_index WHERE path=photo_faces.path),0),source_mtime=coalesce((SELECT mod_time_unix_nano FROM media_index WHERE path=photo_faces.path),0) WHERE entity_id=0`); err != nil {
		return err
	}
	for _, spec := range retainedTables {
		if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS photo_retained_`+spec.table+` AS SELECT CAST(0 AS INTEGER) AS retention_id,t.* FROM `+spec.table+` t WHERE 0`); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_retained_`+spec.table+` ON photo_retained_`+spec.table+`(retention_id)`); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_retained_face_id ON photo_retained_photo_faces(id)`,
		`CREATE INDEX IF NOT EXISTS idx_face_entity ON photo_faces(entity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_face_review ON photo_faces(needs_review,id) WHERE needs_review=1`,
		`CREATE TRIGGER IF NOT EXISTS identity_face_insert AFTER INSERT ON photo_faces WHEN new.entity_id=0 BEGIN
 UPDATE photo_faces SET entity_id=coalesce((SELECT id FROM photo_entities WHERE path=new.path AND kind='image'),0),source_path=new.path,
 source_size=coalesce((SELECT size_bytes FROM media_index WHERE path=new.path),0),source_mtime=coalesce((SELECT mod_time_unix_nano FROM media_index WHERE path=new.path),0),source_hash=coalesce((SELECT fingerprint FROM photo_entities WHERE path=new.path AND kind='image'),'') WHERE id=new.id; END`,
		`DROP TRIGGER IF EXISTS photo_media_faces_invalidate`,
		`CREATE TRIGGER photo_media_faces_invalidate AFTER UPDATE OF size_bytes,mod_time_unix_nano ON media_index
 WHEN new.size_bytes<>old.size_bytes OR new.mod_time_unix_nano<>old.mod_time_unix_nano BEGIN
 UPDATE photo_faces SET needs_review=1,source_revision=source_revision+1 WHERE path=new.path AND needs_review=0 AND NOT EXISTS(SELECT 1 FROM photo_entities e WHERE e.id=photo_faces.entity_id AND e.fingerprint<>'' AND e.fingerprint=photo_faces.source_hash AND e.size_bytes=new.size_bytes AND e.mtime=new.mod_time_unix_nano);
 UPDATE photo_face_jobs SET source_size=new.size_bytes,source_mtime=new.mod_time_unix_nano WHERE path=new.path AND EXISTS(SELECT 1 FROM photo_faces f JOIN photo_entities e ON e.id=f.entity_id WHERE f.path=new.path AND f.needs_review=0 AND f.source_hash<>'' AND f.source_hash=e.fingerprint);
 DELETE FROM photo_face_jobs WHERE path=new.path AND EXISTS(SELECT 1 FROM photo_faces WHERE path=new.path AND needs_review=1); END`,
		`CREATE TRIGGER IF NOT EXISTS identity_face_review AFTER UPDATE OF needs_review ON photo_faces WHEN new.needs_review<>old.needs_review BEGIN
 DELETE FROM photo_face_references WHERE face_id=new.id;
 UPDATE photo_face_thumbnail_cache SET expires_at=0 WHERE face_id=new.id AND new.needs_review=1;
 UPDATE photo_face_state SET revision=revision+1 WHERE id=1;
 UPDATE photo_face_reference_settings SET pending=1,cursor=0 WHERE new.needs_review=0;
 UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id=new.person_id; END`,
		`DROP TRIGGER IF EXISTS photo_media_faces_delete`,
		`CREATE TRIGGER photo_media_faces_delete AFTER DELETE ON media_index BEGIN
 DELETE FROM photo_faces WHERE path=old.path; DELETE FROM photo_face_jobs WHERE path=old.path; DELETE FROM photo_xmp_people WHERE path=old.path;
 UPDATE photo_people SET name='',name_fold='',name_source='' WHERE name_source=old.path AND manual_name=0 AND NOT EXISTS(SELECT 1 FROM photo_entities WHERE path=old.path AND missing_since>0); END`,
		`DROP TRIGGER IF EXISTS face_thumbnail_delete`,
		`CREATE TRIGGER face_thumbnail_delete AFTER DELETE ON photo_faces BEGIN
 UPDATE photo_face_thumbnail_cache SET expires_at=CASE WHEN EXISTS(SELECT 1 FROM photo_retained_photo_faces WHERE id=old.id) THEN 0 ELSE unixepoch() END WHERE face_id=old.id; END`,
		`DROP TRIGGER IF EXISTS face_thumbnail_ignore`,
		`CREATE TRIGGER face_thumbnail_ignore AFTER UPDATE OF ignored ON photo_faces WHEN new.ignored<>old.ignored BEGIN
 UPDATE photo_face_thumbnail_cache SET expires_at=CASE WHEN new.ignored=0 OR new.needs_review=1 THEN 0 ELSE unixepoch()+172800 END WHERE face_id=new.id; END`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	cols, err := loadRetainedColumns(ctx, db)
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE TRIGGER IF NOT EXISTS photo_identity_private AFTER UPDATE OF admin_only ON media_index WHEN new.admin_only=1 AND old.admin_only=0 BEGIN
 INSERT OR IGNORE INTO photo_retained_photo_faces SELECT f.entity_id,f.* FROM photo_faces f WHERE f.path=new.path;
 DELETE FROM photo_faces WHERE path=new.path; DELETE FROM photo_face_jobs WHERE path=new.path;
 INSERT OR REPLACE INTO photo_hidden_person_names SELECT id,name,name_fold,name_source FROM photo_people WHERE name_source=new.path AND manual_name=0 AND name<>'';
 UPDATE photo_people SET name='',name_fold='' WHERE name_source=new.path AND manual_name=0;
 END`,
		`CREATE TRIGGER IF NOT EXISTS photo_identity_public AFTER UPDATE OF admin_only ON media_index WHEN new.admin_only=0 AND old.admin_only=1 BEGIN
 INSERT INTO photo_faces(` + cols["photo_faces"] + `) SELECT ` + cols["photo_faces"] + ` FROM photo_retained_photo_faces WHERE path=new.path;
 DELETE FROM photo_retained_photo_faces WHERE path=new.path;
 UPDATE photo_faces SET needs_review=1,source_revision=source_revision+1 WHERE path=new.path AND (source_size<>new.size_bytes OR source_mtime<>new.mod_time_unix_nano) AND NOT EXISTS(SELECT 1 FROM photo_entities e WHERE e.id=entity_id AND e.fingerprint<>'' AND e.fingerprint=source_hash AND e.size_bytes=new.size_bytes AND e.mtime=new.mod_time_unix_nano);
 UPDATE photo_people SET name=(SELECT name FROM photo_hidden_person_names WHERE person_id=photo_people.id),name_fold=(SELECT name_fold FROM photo_hidden_person_names WHERE person_id=photo_people.id) WHERE manual_name=0 AND name='' AND id IN(SELECT person_id FROM photo_hidden_person_names WHERE name_source=new.path);
 DELETE FROM photo_hidden_person_names WHERE name_source=new.path;
 UPDATE photo_face_reference_settings SET pending=1,cursor=0;
 END`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadRetainedColumns(ctx context.Context, db *sql.DB) (map[string]string, error) {
	out := map[string]string{}
	for _, spec := range retainedTables {
		rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+spec.table+`)`)
		if err != nil {
			return nil, err
		}
		var cols []string
		for rows.Next() {
			var cid, notnull, pk int
			var name, kind string
			var def any
			if err = rows.Scan(&cid, &name, &kind, &notnull, &def, &pk); err != nil {
				rows.Close()
				return nil, err
			}
			cols = append(cols, `"`+name+`"`)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		out[spec.table] = strings.Join(cols, ",")
	}
	return out, nil
}
