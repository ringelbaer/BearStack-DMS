package photos

import (
	"context"
	"database/sql"
)

func setupFaceFavorites(ctx context.Context, db *sql.DB) error {
	if _, err := ensurePhotoColumn(ctx, db, "photo_faces", "favorite", `ALTER TABLE photo_faces ADD COLUMN favorite INTEGER NOT NULL DEFAULT 0 CHECK(favorite IN (0,1))`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_face_reference_folders ON photo_faces(person_id,ignored,model,directory,favorite DESC,manual DESC,confidence DESC,id,path);
 CREATE INDEX IF NOT EXISTS idx_face_favorites ON photo_faces(person_id,model,id,path) WHERE favorite=1 AND ignored=0;
 DROP INDEX IF EXISTS idx_face_reference_selection;
 UPDATE photo_face_reference_settings SET pending=1,cursor=0 WHERE id=1`)
	return err
}

type FaceFavorite struct {
	ID       int64 `json:"id"`
	PersonID int64 `json:"person_id"`
	Favorite bool  `json:"favorite"`
}

// SetFaceFavorite is an idempotent assignment, not a toggle. The expected person
// prevents stale clients from favoriting a face moved to another group.
func (l *Library) SetFaceFavorite(ctx context.Context, id, person int64, favorite bool) (FaceFavorite, error) {
	out := FaceFavorite{ID: id, PersonID: person, Favorite: favorite}
	if id <= 0 || person <= 0 {
		return out, ErrLabelInvalid
	}
	if err := l.refreshPersonIDsVisibility(ctx, person); err != nil {
		return out, err
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	if _, err := l.Face(ctx, id); err != nil {
		return out, err
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	// Reserve the writer before checking membership, also against index updates.
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET id=id WHERE id=1`); err != nil {
		return out, err
	}
	var currentPerson int64
	var current, ignored bool
	if err = tx.QueryRowContext(ctx, `SELECT f.person_id,f.favorite,f.ignored FROM photo_faces f JOIN media_index m ON m.path=f.path WHERE f.id=? AND m.admin_only=0`, id).Scan(&currentPerson, &current, &ignored); err != nil {
		return out, err
	}
	if currentPerson != person || ignored {
		return out, ErrLabelConflict
	}
	if current == favorite {
		return out, nil
	}
	var baseRevision int64
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM photo_face_state WHERE id=1`).Scan(&baseRevision); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_faces SET favorite=? WHERE id=?`, favorite, id); err != nil {
		return out, err
	}
	if err = refreshFaceReferencesTx(ctx, tx, person); err != nil {
		return out, err
	}
	var committedRevision int64
	if err = tx.QueryRowContext(ctx, `UPDATE photo_face_state SET revision=revision+1 WHERE id=1 RETURNING revision`).Scan(&committedRevision); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return out, err
	}
	if l.faceRuntime.graph != nil && l.faceRuntime.revision == baseRevision {
		// Avoid rebuilding the whole collection for one star. A failed cache
		// refresh invalidates the graph for the next analysis; the write succeeded.
		if err := l.syncFaceGraphPeople(ctx, map[int64]bool{person: true}, committedRevision); err != nil {
			l.faceRuntime.graph = nil
		}
	} else {
		// Never acknowledge unrelated changes missing from the cached graph.
		l.faceRuntime.graph = nil
	}
	return out, nil
}
