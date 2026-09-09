package photos

import (
	"context"
	"database/sql"
)

// setupFaceReconciliation adds a resumable, embedding-only maintenance queue.
// User corrections schedule another pass without resetting a running cursor.
func setupFaceReconciliation(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS photo_face_reconciliation (
 id INTEGER PRIMARY KEY CHECK(id=1), pending INTEGER NOT NULL DEFAULT 1,
 running INTEGER NOT NULL DEFAULT 0, generation INTEGER NOT NULL DEFAULT 1,
 run_generation INTEGER NOT NULL DEFAULT 0, cursor INTEGER NOT NULL DEFAULT 0,
 upper_id INTEGER NOT NULL DEFAULT 0, processed INTEGER NOT NULL DEFAULT 0,
 reassigned INTEGER NOT NULL DEFAULT 0, suggestions INTEGER NOT NULL DEFAULT 0
 )`,
		`INSERT OR IGNORE INTO photo_face_reconciliation(id) VALUES(1)`,
		`CREATE INDEX IF NOT EXISTS idx_face_reconcile_candidates ON photo_faces(id) WHERE manual=0 AND ignored=0 AND favorite=0`,
		`CREATE TABLE IF NOT EXISTS photo_face_merge_suggestions (
 id INTEGER PRIMARY KEY AUTOINCREMENT, source_id INTEGER NOT NULL, target_id INTEGER NOT NULL,
 source_revision INTEGER NOT NULL, target_revision INTEGER NOT NULL,
 source_face_id INTEGER NOT NULL, target_face_id INTEGER NOT NULL,
 score REAL NOT NULL, model TEXT NOT NULL, rejected INTEGER NOT NULL DEFAULT 0,
 UNIQUE(source_id,target_id), CHECK(source_id<>target_id)
 )`,
		`CREATE INDEX IF NOT EXISTS idx_face_merge_suggestions_visible ON photo_face_merge_suggestions(rejected,score DESC,id)`,
		`CREATE INDEX IF NOT EXISTS idx_face_merge_suggestions_target ON photo_face_merge_suggestions(target_id)`,
		`CREATE TRIGGER IF NOT EXISTS face_reconcile_person_change AFTER UPDATE OF name,manual_name,name_source ON photo_people
 WHEN new.name<>old.name OR new.manual_name<>old.manual_name OR new.name_source<>old.name_source BEGIN
 UPDATE photo_face_reconciliation SET pending=1,generation=generation+1 WHERE id=1;
 END`,
		`CREATE TRIGGER IF NOT EXISTS face_reconcile_person_trust AFTER UPDATE OF manual_name,name_source ON photo_people
 WHEN new.manual_name<>old.manual_name OR new.name_source<>old.name_source BEGIN
 UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id=new.id;
 END`,
		`CREATE TRIGGER IF NOT EXISTS face_reconcile_face_change AFTER UPDATE OF person_id,manual,favorite,ignored ON photo_faces
 WHEN new.manual<>old.manual OR new.favorite<>old.favorite OR new.ignored<>old.ignored
 OR (new.person_id<>old.person_id AND (new.manual=1 OR old.manual=1)) BEGIN
 UPDATE photo_face_reconciliation SET pending=1,generation=generation+1 WHERE id=1;
 END`,
		`CREATE TRIGGER IF NOT EXISTS face_reconcile_face_trust AFTER UPDATE OF manual,favorite ON photo_faces
 WHEN new.manual<>old.manual OR new.favorite<>old.favorite BEGIN
 UPDATE photo_person_revisions SET revision=revision+1 WHERE person_id IN(old.person_id,new.person_id);
 END`,
		`CREATE TRIGGER IF NOT EXISTS face_reconcile_reference_change AFTER UPDATE OF reference_limit ON photo_face_reference_settings
 WHEN new.reference_limit<>old.reference_limit BEGIN
 UPDATE photo_face_reconciliation SET pending=1,generation=generation+1 WHERE id=1;
 END`,
		`CREATE TRIGGER IF NOT EXISTS face_reconcile_model_change AFTER UPDATE OF model ON photo_face_state
 WHEN new.model<>old.model BEGIN
 UPDATE photo_face_reconciliation SET pending=1,generation=generation+1 WHERE id=1;
 END`,
		`CREATE TRIGGER IF NOT EXISTS face_reconcile_person_delete AFTER DELETE ON photo_people BEGIN
 DELETE FROM photo_face_merge_suggestions WHERE source_id=old.id OR target_id=old.id;
 END`,
		`CREATE TRIGGER IF NOT EXISTS face_reconcile_suggestion_revision AFTER UPDATE OF revision ON photo_person_revisions
 WHEN new.revision<>old.revision BEGIN
 DELETE FROM photo_face_merge_suggestions WHERE rejected=0 AND (source_id=new.person_id OR target_id=new.person_id);
 END`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}
