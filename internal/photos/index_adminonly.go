// Datei enthaelt Indexabfragen und Hilfslogik fuer Admin-only-Sichtbarkeit im Fotoindex.
package photos

import (
	"context"
	"database/sql"

	"bearstack/internal/fsutil"
	"bearstack/internal/sqlutil"
)

func (l *Library) refreshAdminOnlyIndexFlags(ctx context.Context) error {
	if l == nil || !l.index.available() {
		return nil
	}
	dirs, err := l.indexedDirectories(ctx)
	if err != nil {
		return err
	}
	// Resolve current marker state before acquiring the database write lock.
	var byVisibility [2][]string
	adminOnlyCache := map[string]bool{}
	paths := fsutil.NewRootPathBatch(l.root)
	for _, rel := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		adminOnly := 0
		if _, abs, err := paths.Resolve(rel, true, ErrPathEscapesRoot()); err == nil {
			adminOnly = boolInt(directoryAdminOnlyFromAbsCached(rel, abs, adminOnlyCache))
		}
		byVisibility[adminOnly] = append(byVisibility[adminOnly], rel)
	}
	if len(dirs) == 0 {
		return nil
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	affectedFolders := make(map[string]struct{})
	changedAny := false
	for adminOnly, paths := range byVisibility {
		for start := 0; start < len(paths); start += adminOnlyUpdateBatchSize {
			batch := paths[start:min(start+adminOnlyUpdateBatchSize, len(paths))]
			changed, err := refreshAdminOnlyBatch(ctx, tx, batch, adminOnly)
			if err != nil {
				return err
			}
			for _, rel := range changed {
				changedAny = true
				markAffectedFolders(affectedFolders, rel)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := l.refreshFolderRecursiveCounts(ctx, affectedFolders); err != nil {
		return err
	}
	if changedAny {
		return l.refreshPhotoStats(ctx)
	}
	return nil
}

// Three IN clauses plus four flags stay below SQLite's conservative bind limit.
const adminOnlyUpdateBatchSize = 200

func refreshAdminOnlyBatch(ctx context.Context, tx *sql.Tx, paths []string, adminOnly int) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	placeholders := sqlutil.Placeholders(len(paths))
	args := make([]any, 0, len(paths)*3+4)
	for i := 0; i < 3; i++ {
		for _, path := range paths {
			args = append(args, path)
		}
		args = append(args, adminOnly)
	}
	args = append(args, adminOnly)
	// Return directories, not one RETURNING row per medium in a large album.
	rows, err := tx.QueryContext(ctx, `
		SELECT directory FROM media_index WHERE directory IN (`+placeholders+`) AND admin_only <> ?
		UNION
		SELECT directory FROM blog_index WHERE directory IN (`+placeholders+`) AND admin_only <> ?
		UNION
		SELECT path FROM folder_index WHERE path IN (`+placeholders+`)
			AND (admin_only <> ? OR public_media_count <> CASE WHEN ? = 1 THEN 0 ELSE media_count END)`, args...)
	if err != nil {
		return nil, err
	}
	var changed []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return nil, err
		}
		changed = append(changed, path)
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil || len(changed) == 0 {
		return nil, err
	}
	placeholders = sqlutil.Placeholders(len(changed))
	args = []any{adminOnly}
	for _, path := range changed {
		args = append(args, path)
	}
	args = append(args, adminOnly)
	if _, err := tx.ExecContext(ctx, `UPDATE media_index SET admin_only = ? WHERE directory IN (`+placeholders+`) AND admin_only <> ?`, args...); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE blog_index SET admin_only = ? WHERE directory IN (`+placeholders+`) AND admin_only <> ?`, args...); err != nil {
		return nil, err
	}
	args = []any{adminOnly, adminOnly}
	for _, path := range changed {
		args = append(args, path)
	}
	args = append(args, adminOnly, adminOnly)
	if _, err := tx.ExecContext(ctx, `UPDATE folder_index
		SET admin_only = ?, public_media_count = CASE WHEN ? = 1 THEN 0 ELSE media_count END
		WHERE path IN (`+placeholders+`)
			AND (admin_only <> ? OR public_media_count <> CASE WHEN ? = 1 THEN 0 ELSE media_count END)`, args...); err != nil {
		return nil, err
	}
	return changed, nil
}

func (l *Library) indexedDirectories(ctx context.Context) ([]string, error) {
	rows, err := l.index.db.QueryContext(ctx, `
		SELECT directory FROM media_index
		UNION
		SELECT directory FROM blog_index
		UNION
		SELECT path FROM folder_index`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dirs []string
	for rows.Next() {
		var dir string
		if err := rows.Scan(&dir); err != nil {
			return nil, err
		}
		dirs = append(dirs, dir)
	}
	return dirs, rows.Err()
}
