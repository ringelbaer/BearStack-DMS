package photos

import (
	"context"
	"database/sql"
)

// Triggers cover mutations from every writer, including the web UI and indexer.
func setupLabelingSchema(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS photo_labeling_identity (id INTEGER PRIMARY KEY CHECK(id=1), instance TEXT NOT NULL, dataset TEXT NOT NULL)`,
		`INSERT OR IGNORE INTO photo_labeling_identity VALUES(1,lower(hex(randomblob(16))),lower(hex(randomblob(16))))`,
		`CREATE TABLE IF NOT EXISTS photo_person_revisions (person_id INTEGER PRIMARY KEY, revision INTEGER NOT NULL DEFAULT 1)`,
		`INSERT OR IGNORE INTO photo_person_revisions SELECT id,1 FROM photo_people`,
		`CREATE TABLE IF NOT EXISTS photo_labeling_actions (actor TEXT NOT NULL, operation_id TEXT NOT NULL, fingerprint TEXT NOT NULL, result TEXT NOT NULL, PRIMARY KEY(actor,operation_id)) WITHOUT ROWID`,
		`CREATE INDEX IF NOT EXISTS idx_label_face_page ON photo_faces(person_id,ignored,id)`,
		`CREATE INDEX IF NOT EXISTS idx_photo_people_unnamed ON photo_people(id) WHERE name=''`,
		`CREATE TRIGGER IF NOT EXISTS labeling_person_insert AFTER INSERT ON photo_people BEGIN INSERT OR REPLACE INTO photo_person_revisions VALUES(new.id,1); END`,
		`CREATE TRIGGER IF NOT EXISTS labeling_person_name AFTER UPDATE OF name ON photo_people WHEN new.name<>old.name BEGIN UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id=new.id; END`,
		`CREATE TRIGGER IF NOT EXISTS labeling_person_delete AFTER DELETE ON photo_people BEGIN DELETE FROM photo_person_revisions WHERE person_id=old.id; END`,
		`CREATE TRIGGER IF NOT EXISTS labeling_face_insert AFTER INSERT ON photo_faces BEGIN UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id=new.person_id; END`,
		`CREATE TRIGGER IF NOT EXISTS labeling_face_delete AFTER DELETE ON photo_faces BEGIN UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id=old.person_id; END`,
		`CREATE TRIGGER IF NOT EXISTS labeling_face_update AFTER UPDATE OF person_id,ignored,path,x,y,width,height ON photo_faces
 WHEN new.person_id<>old.person_id OR new.ignored<>old.ignored OR new.path<>old.path OR new.x<>old.x OR new.y<>old.y OR new.width<>old.width OR new.height<>old.height
 BEGIN UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id IN(old.person_id,new.person_id); END`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
