package photos

import (
	"context"
	"strings"
)

// CanResetIgnoredDirectory excludes the root, first-level and virtual folders.
// Require a canonical path so alternate spellings cannot bypass the depth rule.
func CanResetIgnoredDirectory(directory string) bool {
	if len(directory) > 4096 {
		return false
	}
	clean, err := CleanPath(directory)
	return err == nil && clean == directory && strings.Count(directory, "/") >= 1 && !IsPeopleFolder(directory)
}

// ResetIgnoredDirectoryFaces restores unnamed, ignored faces in this subtree.
// Named groups and active faces are excluded. Existing visibility cleanup runs
// first; the reset and reference updates commit together without inference.
func (l *Library) ResetIgnoredDirectoryFaces(ctx context.Context, directory string) (int64, error) {
	if !CanResetIgnoredDirectory(directory) {
		return 0, ErrLabelInvalid
	}
	private, err := l.FolderAdminOnly(directory)
	if err != nil {
		return 0, err
	}
	if private {
		return 0, ErrAdminOnly()
	}
	scope, args := directoryPeopleRange("f.path", directory)
	// Refresh visibility before selecting the write set, including shared groups'
	// other directories so reference refreshes cannot reintroduce private faces.
	if err := l.refreshPeopleVisibility(ctx, `p.name='' AND p.id IN (SELECT f.person_id FROM photo_faces f INDEXED BY idx_photo_faces_path WHERE f.ignored=1 AND `+scope+`)`, args...); err != nil {
		return 0, err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET id=id WHERE id=1`); err != nil {
		return 0, err
	}
	// The exact prefix range uses the existing path index, including for names
	// containing '%' or '_'. Retain only distinct group IDs, never a photo list.
	selection := ` FROM photo_faces f INDEXED BY idx_photo_faces_path
 CROSS JOIN media_index m ON m.path=f.path
 CROSS JOIN photo_people p ON p.id=f.person_id
 WHERE ` + scope + ` AND f.ignored=1 AND p.name='' AND m.admin_only=0`
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT f.person_id`+selection, args...)
	if err != nil {
		return 0, err
	}
	affected := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		affected[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(affected) == 0 {
		return 0, err
	}
	var baseRevision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&baseRevision); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE photo_faces SET ignored=0 WHERE id IN (SELECT f.id`+selection+`)`, args...)
	if err != nil {
		return 0, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	revision, err := refreshFaceMutationTx(ctx, tx, affected)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	l.syncFaceMutation(ctx, affected, baseRevision, revision)
	return count, nil
}
