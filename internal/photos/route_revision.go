package photos

import (
	"context"
	"database/sql"
	"fmt"
)

type photoRouteRevision struct {
	Instance string `json:"instance"`
	Number   int64  `json:"number"`
}

// The triggers participate in the metadata writer's transaction, including
// rebuilds, GPX/XMP updates and visibility refreshes. Per-directory high-water
// marks retain deletions and moves, so recursive ancestors invalidate without
// touching unrelated routes. Tags/ratings do not affect route revisions.
func setupPhotoRouteRevision(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS photo_route_revision (id INTEGER PRIMARY KEY CHECK(id=1), instance TEXT NOT NULL, revision INTEGER NOT NULL)`,
		`INSERT OR IGNORE INTO photo_route_revision(id,instance,revision) VALUES(1,lower(hex(randomblob(16))),0)`,
		`CREATE TABLE IF NOT EXISTS photo_route_directory_revision (
            directory TEXT NOT NULL, type TEXT NOT NULL, admin_only INTEGER NOT NULL,
            revision INTEGER NOT NULL, PRIMARY KEY(directory,type,admin_only)) WITHOUT ROWID`,
		// Reinstall inside the same transaction to upgrade the old global triggers.
		`DROP TRIGGER IF EXISTS photo_route_insert`,
		`DROP TRIGGER IF EXISTS photo_route_delete`,
		`DROP TRIGGER IF EXISTS photo_route_update`,
		`CREATE TRIGGER photo_route_insert AFTER INSERT ON media_index
        WHEN NEW.latitude IS NOT NULL AND NEW.longitude IS NOT NULL
        BEGIN UPDATE photo_route_revision SET revision=revision+1 WHERE id=1; ` + routeDirectoryRevisionSQL("NEW") + ` END`,
		`CREATE TRIGGER photo_route_delete AFTER DELETE ON media_index
        WHEN OLD.latitude IS NOT NULL AND OLD.longitude IS NOT NULL
        BEGIN UPDATE photo_route_revision SET revision=revision+1 WHERE id=1; ` + routeDirectoryRevisionSQL("OLD") + ` END`,
		`CREATE TRIGGER photo_route_update
        AFTER UPDATE OF latitude,longitude,captured_at,mod_time_unix_nano,directory,type,admin_only ON media_index
        WHEN ((OLD.latitude IS NOT NULL AND OLD.longitude IS NOT NULL) OR (NEW.latitude IS NOT NULL AND NEW.longitude IS NOT NULL))
        AND (OLD.latitude IS NOT NEW.latitude OR OLD.longitude IS NOT NEW.longitude
        OR OLD.captured_at IS NOT NEW.captured_at OR OLD.mod_time_unix_nano IS NOT NEW.mod_time_unix_nano
        OR OLD.directory IS NOT NEW.directory OR OLD.type IS NOT NEW.type OR OLD.admin_only IS NOT NEW.admin_only)
        BEGIN UPDATE photo_route_revision SET revision=revision+1 WHERE id=1; ` +
			routeDirectoryRevisionSQL("OLD") + routeDirectoryRevisionSQL("NEW") + ` END`,
	} {
		if _, err = tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// At most two small revision rows per changed photo, independent of tree depth.
// Keep tombstones: removing the final photo must still invalidate a cached route.
func routeDirectoryRevisionSQL(row string) string {
	return fmt.Sprintf(`INSERT INTO photo_route_directory_revision(directory,type,admin_only,revision)
        SELECT %[1]s.directory,%[1]s.type,%[1]s.admin_only,revision FROM photo_route_revision
        WHERE id=1 AND %[1]s.latitude IS NOT NULL AND %[1]s.longitude IS NOT NULL
        ON CONFLICT(directory,type,admin_only) DO UPDATE SET revision=excluded.revision; `, row)
}

func (l *Library) scopedPhotoRouteRevision(ctx context.Context, query indexMediaOptions) (photoRouteRevision, error) {
	where := "1=1"
	var args []any
	if query.Directory != "" {
		start, end := prefixRange(query.Directory + "/")
		where += " AND (directory=? OR (directory>=? AND directory<?))"
		args = append(args, query.Directory, start, end)
	}
	if mediaType := normalizeMediaType(query.MediaType); mediaType != "" {
		where += " AND type=?"
		args = append(args, mediaType)
	}
	if !query.IncludeAdminOnly {
		where += " AND admin_only=0"
	}
	var revision photoRouteRevision
	err := l.index.db.QueryRowContext(ctx, `SELECT instance || ':directory-v1',
        (SELECT COALESCE(MAX(revision),0) FROM photo_route_directory_revision WHERE `+where+`)
        FROM photo_route_revision WHERE id=1`, args...).Scan(&revision.Instance, &revision.Number)
	return revision, err
}
