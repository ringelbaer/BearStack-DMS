package photos

import (
	"context"
	"database/sql"
	"errors"
)

const DefaultFaceReferenceLimit = 30
const MaxFaceReferenceLimit = 100

func setupFaceReferenceSettings(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS photo_face_reference_settings (
 id INTEGER PRIMARY KEY CHECK(id=1),
 reference_limit INTEGER NOT NULL DEFAULT 30 CHECK(reference_limit BETWEEN 1 AND 100),
 pending INTEGER NOT NULL DEFAULT 1,
 cursor INTEGER NOT NULL DEFAULT 0
 ); INSERT OR IGNORE INTO photo_face_reference_settings(id) VALUES(1)`)
	return err
}

func (l *Library) FaceReferenceLimit(ctx context.Context) (int, error) {
	var limit int
	err := l.index.db.QueryRowContext(ctx, `SELECT reference_limit FROM photo_face_reference_settings WHERE id=1`).Scan(&limit)
	return limit, err
}

// SetFaceReferenceLimit schedules a resumable refresh; saving settings never scans
// the full collection or invokes inference. The next analysis refreshes references.
func (l *Library) SetFaceReferenceLimit(ctx context.Context, limit int) error {
	if limit < 1 || limit > MaxFaceReferenceLimit {
		return errors.New("Referenzen pro Person müssen zwischen 1 und 100 liegen")
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	result, err := l.index.db.ExecContext(ctx, `UPDATE photo_face_reference_settings SET reference_limit=?,pending=1,cursor=0 WHERE id=1 AND reference_limit<>?`, limit, limit)
	if err == nil {
		if changed, e := result.RowsAffected(); e == nil && changed > 0 {
			l.faceRuntime.graph = nil
		}
	}
	return err
}

// Caller holds faceRuntime.mu. Checkpoint every 100 people, keeping write
// transactions short and resuming after cancellation or process restart.
func (l *Library) rebuildFaceReferences(ctx context.Context) error {
	var pending bool
	if err := l.index.db.QueryRowContext(ctx, `SELECT pending FROM photo_face_reference_settings WHERE id=1`).Scan(&pending); err != nil {
		return err
	}
	if !pending {
		return nil
	}
	if err := l.RefreshFaceVisibility(ctx); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		tx, err := l.index.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		err = refreshFaceReferenceBatch(ctx, tx)
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.QueryRowContext(ctx, `SELECT pending FROM photo_face_reference_settings WHERE id=1`).Scan(&pending); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		if !pending {
			return nil
		}
	}
}

func refreshFaceReferenceBatch(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM photo_people WHERE id>(SELECT cursor FROM photo_face_reference_settings WHERE id=1) ORDER BY id LIMIT 100`)
	if err != nil {
		return err
	}
	var people []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		people = append(people, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range people {
		if err = refreshFaceReferencesTx(ctx, tx, id); err != nil {
			return err
		}
	}
	var cursor int64
	if len(people) > 0 {
		cursor = people[len(people)-1]
	}
	if _, err = tx.ExecContext(ctx, `UPDATE photo_face_reference_settings SET pending=?,cursor=? WHERE id=1`, len(people) == 100, cursor); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE photo_face_state SET revision=revision+1 WHERE id=1`)
	return err
}
