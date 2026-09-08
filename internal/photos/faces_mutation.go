package photos

import (
	"context"
	"database/sql"
)

// refreshFaceMutationTx prepares references and the new revision in the caller's
// transaction. It never commits or publishes cache changes.
func refreshFaceMutationTx(ctx context.Context, tx *sql.Tx, affected map[int64]bool) (int64, error) {
	for person := range affected {
		if err := refreshFaceReferencesTx(ctx, tx, person); err != nil {
			return 0, err
		}
	}
	var revision int64
	err := tx.QueryRowContext(ctx, `UPDATE photo_face_state SET revision=revision+1 WHERE id=1 RETURNING revision`).Scan(&revision)
	return revision, err
}

// syncFaceMutation runs only after a successful commit, with faceRuntime.mu held.
// Cache errors cannot undo the write; invalidate for the next analysis instead.
func (l *Library) syncFaceMutation(ctx context.Context, affected map[int64]bool, baseRevision, committedRevision int64) {
	if l.faceRuntime.graph != nil && l.faceRuntime.revision == baseRevision {
		if err := l.syncFaceGraphPeople(ctx, affected, committedRevision); err == nil {
			return
		}
	}
	// Never acknowledge unrelated changes missing from the cached graph.
	l.faceRuntime.graph = nil
}
