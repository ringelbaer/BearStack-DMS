// Datei enthaelt Indexabfragen und Hilfslogik fuer Admin-only-Sichtbarkeit im Fotoindex.
package photos

import (
	"context"
	"database/sql"
)

func (l *Library) refreshAdminOnlyIndexFlags(ctx context.Context) error {
	if l == nil || !l.index.available() {
		return nil
	}
	dirs, err := l.indexedDirectories(ctx)
	if err != nil {
		return err
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	mediaStmt, err := tx.PrepareContext(ctx, `UPDATE media_index SET admin_only = ? WHERE directory = ? AND admin_only <> ?`)
	if err != nil {
		return err
	}
	defer mediaStmt.Close()
	blogStmt, err := tx.PrepareContext(ctx, `UPDATE blog_index SET admin_only = ? WHERE directory = ? AND admin_only <> ?`)
	if err != nil {
		return err
	}
	defer blogStmt.Close()
	folderStmt, err := tx.PrepareContext(ctx, `
		UPDATE folder_index
		SET admin_only = ?,
			public_media_count = CASE WHEN ? = 1 THEN 0 ELSE media_count END
		WHERE path = ?
			AND (admin_only <> ? OR public_media_count <> CASE WHEN ? = 1 THEN 0 ELSE media_count END)`)
	if err != nil {
		return err
	}
	defer folderStmt.Close()

	adminOnlyCache := map[string]bool{}
	affectedFolders := make(map[string]struct{})
	changedAny := false
	for _, rel := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		adminOnly := 0
		if abs, err := l.Resolve(rel); err == nil {
			adminOnly = boolInt(directoryAdminOnlyFromAbsCached(rel, abs, adminOnlyCache))
		}
		mediaResult, err := mediaStmt.ExecContext(ctx, adminOnly, rel, adminOnly)
		if err != nil {
			return err
		}
		blogResult, err := blogStmt.ExecContext(ctx, adminOnly, rel, adminOnly)
		if err != nil {
			return err
		}
		folderResult, err := folderStmt.ExecContext(ctx,
			adminOnly,
			adminOnly,
			rel,
			adminOnly,
			adminOnly,
		)
		if err != nil {
			return err
		}
		if rowsAffectedChanged(mediaResult) || rowsAffectedChanged(blogResult) || rowsAffectedChanged(folderResult) {
			changedAny = true
			markAffectedFolders(affectedFolders, rel)
			if rel != "" {
				affectedFolders[rel] = struct{}{}
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

func rowsAffectedChanged(result sql.Result) bool {
	if result == nil {
		return false
	}
	rows, err := result.RowsAffected()
	return err == nil && rows > 0
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
