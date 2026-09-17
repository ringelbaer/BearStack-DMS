package photos

import (
	"context"
	"database/sql"
)

// Recreated in the schema transaction so column additions also reach privacy snapshots.
func setupIdentityPrivacyTx(ctx context.Context, tx *sql.Tx) error {
	cols, err := loadRetainedColumns(ctx, tx)
	if err != nil {
		return err
	}
	for _, stmt := range []string{
		`DROP TRIGGER IF EXISTS photo_identity_private`,
		`DROP TRIGGER IF EXISTS photo_identity_public`,
		`CREATE TRIGGER photo_identity_private AFTER UPDATE OF admin_only ON media_index WHEN new.admin_only=1 AND old.admin_only=0 BEGIN
 INSERT OR IGNORE INTO photo_retained_photo_faces(retention_id,` + cols["photo_faces"] + `) SELECT entity_id,` + cols["photo_faces"] + ` FROM photo_faces WHERE path=new.path;
 DELETE FROM photo_faces WHERE path=new.path; DELETE FROM photo_face_jobs WHERE path=new.path;
 INSERT OR REPLACE INTO photo_hidden_person_names SELECT id,name,name_fold,name_source FROM photo_people WHERE name_source=new.path AND manual_name=0 AND name<>'';
 UPDATE photo_people SET name='',name_fold='' WHERE name_source=new.path AND manual_name=0;
 END`,
		`CREATE TRIGGER photo_identity_public AFTER UPDATE OF admin_only ON media_index WHEN new.admin_only=0 AND old.admin_only=1 BEGIN
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
	return nil
}
