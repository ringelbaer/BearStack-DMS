package photos

import (
	"context"
	"database/sql"
)

type photoRouteRevision struct {
	Instance string `json:"instance"`
	Number   int64  `json:"number"`
}

// The triggers participate in the metadata writer's transaction, including
// rebuilds, GPX/XMP updates and visibility refreshes. A global revision also
// invalidates recursive ancestors. Unrelated tags/ratings do not invalidate it.
func setupPhotoRouteRevision(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS photo_route_revision (id INTEGER PRIMARY KEY CHECK(id=1), instance TEXT NOT NULL, revision INTEGER NOT NULL)`,
		`INSERT OR IGNORE INTO photo_route_revision(id,instance,revision) VALUES(1,lower(hex(randomblob(16))),0)`,
		`CREATE TRIGGER IF NOT EXISTS photo_route_insert AFTER INSERT ON media_index
		WHEN NEW.latitude IS NOT NULL AND NEW.longitude IS NOT NULL
		BEGIN UPDATE photo_route_revision SET revision=revision+1 WHERE id=1; END`,
		`CREATE TRIGGER IF NOT EXISTS photo_route_delete AFTER DELETE ON media_index
		WHEN OLD.latitude IS NOT NULL AND OLD.longitude IS NOT NULL
		BEGIN UPDATE photo_route_revision SET revision=revision+1 WHERE id=1; END`,
		`CREATE TRIGGER IF NOT EXISTS photo_route_update
		AFTER UPDATE OF latitude,longitude,captured_at,mod_time_unix_nano,directory,type,admin_only ON media_index
		WHEN ((OLD.latitude IS NOT NULL AND OLD.longitude IS NOT NULL) OR (NEW.latitude IS NOT NULL AND NEW.longitude IS NOT NULL))
		AND (OLD.latitude IS NOT NEW.latitude OR OLD.longitude IS NOT NEW.longitude
		OR OLD.captured_at IS NOT NEW.captured_at OR OLD.mod_time_unix_nano IS NOT NEW.mod_time_unix_nano
		OR OLD.directory IS NOT NEW.directory OR OLD.type IS NOT NEW.type OR OLD.admin_only IS NOT NEW.admin_only)
		BEGIN UPDATE photo_route_revision SET revision=revision+1 WHERE id=1; END`,
	} {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (l *Library) photoRouteRevision(ctx context.Context) (photoRouteRevision, error) {
	var revision photoRouteRevision
	err := l.index.db.QueryRowContext(ctx, `SELECT instance,revision FROM photo_route_revision WHERE id=1`).Scan(&revision.Instance, &revision.Number)
	return revision, err
}
