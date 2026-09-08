package photos

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const ignoredFaceThumbnailTTL = 48 * time.Hour

func setupFaceThumbnailSchema(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS photo_face_thumbnail_cache (cache_key TEXT PRIMARY KEY CHECK(length(cache_key)=64 AND cache_key NOT GLOB '*[^0-9a-f]*'), face_id INTEGER NOT NULL, expires_at INTEGER NOT NULL DEFAULT 0) WITHOUT ROWID`,
		`CREATE INDEX IF NOT EXISTS idx_face_thumbnail_face ON photo_face_thumbnail_cache(face_id)`,
		`CREATE INDEX IF NOT EXISTS idx_face_thumbnail_expiry ON photo_face_thumbnail_cache(expires_at,cache_key) WHERE expires_at>0`,
		`CREATE TABLE IF NOT EXISTS photo_face_thumbnail_backfill (id INTEGER PRIMARY KEY CHECK(id=1), cursor INTEGER NOT NULL DEFAULT 0, upper_id INTEGER NOT NULL, ignored_expires_at INTEGER NOT NULL)`,
		// Capture one deadline and an upper bound, without scanning the collection.
		// Neither cancellation nor reopening the database extends legacy lifetimes.
		fmt.Sprintf(`INSERT OR IGNORE INTO photo_face_thumbnail_backfill(id,upper_id,ignored_expires_at) SELECT 1,coalesce(max(id),0),unixepoch()+%d FROM photo_faces`, int64(ignoredFaceThumbnailTTL/time.Second)),
		// Cover both individual web edits and whole-group Android/labeling edits.
		fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS face_thumbnail_ignore AFTER UPDATE OF ignored ON photo_faces WHEN new.ignored<>old.ignored BEGIN
 UPDATE photo_face_thumbnail_cache SET expires_at=CASE WHEN new.ignored=0 THEN 0 ELSE unixepoch()+%d END WHERE face_id=new.id;
 END`, int64(ignoredFaceThumbnailTTL/time.Second)),
		`CREATE TRIGGER IF NOT EXISTS face_thumbnail_delete AFTER DELETE ON photo_faces BEGIN
 UPDATE photo_face_thumbnail_cache SET expires_at=unixepoch() WHERE face_id=old.id;
 END`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}
