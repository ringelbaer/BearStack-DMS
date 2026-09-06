package photos

import (
	"context"
	"database/sql"
)

func setupFaceSchema(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS photo_face_state (id INTEGER PRIMARY KEY CHECK(id=1), enabled INTEGER NOT NULL DEFAULT 0, model TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 0)`,
		`INSERT OR IGNORE INTO photo_face_state(id) VALUES(1)`,
		`CREATE TABLE IF NOT EXISTS photo_face_jobs (path TEXT PRIMARY KEY, directory TEXT NOT NULL DEFAULT '', source_size INTEGER NOT NULL, source_mtime INTEGER NOT NULL, source_xmp TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'queued', attempts INTEGER NOT NULL DEFAULT 0, retry_at INTEGER NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS photo_face_job_directories (directory TEXT PRIMARY KEY, job_count INTEGER NOT NULL) WITHOUT ROWID`,
		`CREATE TRIGGER IF NOT EXISTS photo_face_job_insert AFTER INSERT ON photo_face_jobs BEGIN
 INSERT INTO photo_face_job_directories(directory,job_count) VALUES(new.directory,1) ON CONFLICT(directory) DO UPDATE SET job_count=job_count+1;
 END`,
		`CREATE TRIGGER IF NOT EXISTS photo_face_job_delete AFTER DELETE ON photo_face_jobs BEGIN
 UPDATE photo_face_job_directories SET job_count=job_count-1 WHERE directory=old.directory;
 DELETE FROM photo_face_job_directories WHERE directory=old.directory AND job_count<=0;
 END`,
		`CREATE INDEX IF NOT EXISTS idx_face_jobs_errors ON photo_face_jobs(path) WHERE error<>''`,
		`CREATE INDEX IF NOT EXISTS idx_face_jobs_ready ON photo_face_jobs(status,retry_at,path)`,
		`CREATE TABLE IF NOT EXISTS photo_people (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL DEFAULT '', name_fold TEXT NOT NULL DEFAULT '', name_source TEXT NOT NULL DEFAULT '', manual_name INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS idx_photo_people_name ON photo_people(name_fold,id)`,
		`CREATE TABLE IF NOT EXISTS photo_faces (id INTEGER PRIMARY KEY AUTOINCREMENT, path TEXT NOT NULL, directory TEXT NOT NULL DEFAULT '', person_id INTEGER NOT NULL, x REAL NOT NULL, y REAL NOT NULL, width REAL NOT NULL, height REAL NOT NULL, confidence REAL NOT NULL, embedding BLOB NOT NULL, model TEXT NOT NULL, manual INTEGER NOT NULL DEFAULT 0, ignored INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS idx_photo_faces_path ON photo_faces(path,id)`,
		`CREATE TABLE IF NOT EXISTS photo_face_directories (directory TEXT PRIMARY KEY, face_count INTEGER NOT NULL) WITHOUT ROWID`,
		`CREATE INDEX IF NOT EXISTS idx_face_reference_selection ON photo_faces(person_id,ignored,manual DESC,confidence DESC,id)`,
		`CREATE INDEX IF NOT EXISTS idx_photo_faces_person ON photo_faces(person_id,ignored,path,id)`,
		`CREATE TABLE IF NOT EXISTS photo_face_references (face_id INTEGER PRIMARY KEY, person_id INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_face_references_person ON photo_face_references(person_id,face_id)`,
		`CREATE TABLE IF NOT EXISTS photo_xmp_people (path TEXT NOT NULL, name TEXT NOT NULL, PRIMARY KEY(path,name)) WITHOUT ROWID`,
		`CREATE INDEX IF NOT EXISTS idx_xmp_people_name ON photo_xmp_people(name,path)`,
		`CREATE TRIGGER IF NOT EXISTS photo_faces_insert AFTER INSERT ON photo_faces BEGIN
 INSERT INTO photo_face_directories(directory,face_count) VALUES(new.directory,1) ON CONFLICT(directory) DO UPDATE SET face_count=face_count+1;
 END`,
		`CREATE TRIGGER IF NOT EXISTS photo_faces_delete AFTER DELETE ON photo_faces BEGIN
 DELETE FROM photo_face_references WHERE face_id=old.id;
 UPDATE photo_face_directories SET face_count=face_count-1 WHERE directory=old.directory;
 DELETE FROM photo_face_directories WHERE directory=old.directory AND face_count<=0;
 UPDATE photo_face_state SET revision=revision+1 WHERE id=1;
 END`,
		`CREATE TRIGGER IF NOT EXISTS photo_media_faces_delete AFTER DELETE ON media_index BEGIN
 DELETE FROM photo_faces WHERE path=old.path;
 DELETE FROM photo_face_jobs WHERE path=old.path;
 DELETE FROM photo_xmp_people WHERE path=old.path;
 UPDATE photo_people SET name='',name_fold='',name_source='' WHERE name_source=old.path AND manual_name=0;
 END`,
		`CREATE TRIGGER IF NOT EXISTS photo_media_faces_invalidate AFTER UPDATE OF size_bytes,mod_time_unix_nano,admin_only ON media_index
 WHEN new.size_bytes<>old.size_bytes OR new.mod_time_unix_nano<>old.mod_time_unix_nano OR new.admin_only<>old.admin_only BEGIN
 DELETE FROM photo_faces WHERE path=new.path;
 DELETE FROM photo_face_jobs WHERE path=new.path;
 UPDATE photo_people SET name='',name_fold='',name_source='' WHERE name_source=new.path AND manual_name=0;
 END`,
		`CREATE TRIGGER IF NOT EXISTS photo_xmp_people_insert AFTER INSERT ON media_index BEGIN
 INSERT OR IGNORE INTO photo_xmp_people(path,name) SELECT new.path,json_extract(value,'$.Name') FROM json_each(new.faces) WHERE coalesce(json_extract(value,'$.Name'),'')<>'';
 END`,
		`CREATE TRIGGER IF NOT EXISTS photo_xmp_people_update AFTER UPDATE OF faces ON media_index WHEN new.faces<>old.faces BEGIN
 DELETE FROM photo_xmp_people WHERE path=new.path;
 INSERT OR IGNORE INTO photo_xmp_people(path,name) SELECT new.path,json_extract(value,'$.Name') FROM json_each(new.faces) WHERE coalesce(json_extract(value,'$.Name'),'')<>'';
 UPDATE photo_people SET name='',name_fold='',name_source='' WHERE name_source=new.path AND manual_name=0 AND NOT EXISTS(SELECT 1 FROM photo_xmp_people xp WHERE xp.path=new.path AND bearstack_german_fold(xp.name)=photo_people.name_fold);
 END`,
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range statements {
		if _, err = tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	// One-time backfill: schema version is recorded by the caller after setup.
	var version int
	if err = tx.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE component='photos'`).Scan(&version); err != nil && err != sql.ErrNoRows {
		return err
	}
	// The table may be empty on both fresh installations and upgrades. INSERT OR IGNORE
	// is bounded to the migration, not repeated at every application startup.
	if version < 18 {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO photo_xmp_people(path,name) SELECT m.path,json_extract(j.value,'$.Name') FROM media_index m,json_each(m.faces) j WHERE coalesce(json_extract(j.value,'$.Name'),'')<>''`); err != nil {
			return err
		}
	}
	return tx.Commit()
}
