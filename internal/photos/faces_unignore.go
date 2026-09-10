package photos

import (
	"context"
	"crypto/sha256"
)

// UnignorePhotoFaces clears ignored flags only in the displayed photo. Names,
// person assignments and manual markers stay intact; stale snapshots cannot
// restore newly ignored faces that the user has not seen.
func (l *Library) UnignorePhotoFaces(ctx context.Context, path, revision string) (int, error) {
	if len(revision) != sha256.Size*2 {
		return 0, ErrLabelInvalid
	}
	if err := l.refreshGroupPhoto(ctx, path); err != nil {
		return 0, err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE photo_face_state SET id=id WHERE id=1`); err != nil {
		return 0, err
	}
	photo, err := readGroupPhoto(ctx, tx, path)
	if err != nil {
		return 0, err
	}
	if photo.Revision != revision {
		return 0, ErrGroupPhotoChanged
	}
	affected := map[int64]bool{}
	count := 0
	for _, face := range photo.Faces {
		if face.Ignored {
			affected[face.PersonID] = true
			count++
		}
	}
	if count == 0 {
		return 0, nil
	}
	var baseRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&baseRevision); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE photo_faces SET ignored=0 WHERE path=? AND ignored=1`, path); err != nil {
		return 0, err
	}
	committedRevision, err := refreshFaceMutationTx(ctx, tx, affected)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	l.syncFaceMutation(ctx, affected, baseRevision, committedRevision)
	return count, nil
}
