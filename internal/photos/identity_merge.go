package photos

import (
	"context"
	"database/sql"
)

// Retention and privacy both move faces out of the serving table. They still
// belong to the same person and must follow every identity merge atomically.
func mergeRetainedPersonTx(ctx context.Context, tx *sql.Tx, source, target int64, manual bool) error {
	if _, err := tx.ExecContext(ctx, `UPDATE photo_retained_photo_faces SET person_id=?,manual=CASE WHEN ? THEN 1 ELSE manual END WHERE person_id=?`, target, manual, source); err != nil {
		return err
	}
	// Preserve a hidden imported name only if the surviving person has no name
	// or competing provenance. Never publish the hidden name during the merge.
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO photo_hidden_person_names(person_id,name,name_fold,name_source)
 SELECT ?,h.name,h.name_fold,h.name_source FROM photo_hidden_person_names h JOIN photo_people p ON p.id=?
 WHERE h.person_id=? AND p.manual_name=0 AND p.name='' AND (p.name_source='' OR p.name_source=h.name_source)`, target, target, source); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_people SET name_source=(SELECT name_source FROM photo_hidden_person_names WHERE person_id=?) WHERE id=? AND name='' AND manual_name=0 AND name_source='' AND EXISTS(SELECT 1 FROM photo_hidden_person_names WHERE person_id=?)`, target, target, target); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM photo_hidden_person_names WHERE person_id=?`, source)
	return err
}
