package photos

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
)

// Queue cache deletion in the same transaction that retires its identity. No
// filesystem mutations occur until the new index state has committed.
func (l *Library) retireMediaCachesTx(ctx context.Context, tx *sql.Tx, e photoEntity) error {
	rows, err := tx.QueryContext(ctx, `SELECT size FROM photo_thumbnail_index WHERE media_path=? UNION SELECT size FROM photo_retained_photo_thumbnail_index WHERE retention_id=?`, e.Path, e.ID)
	if err != nil {
		return err
	}
	var sizes []int
	for rows.Next() {
		var size int
		if err = rows.Scan(&size); err != nil {
			rows.Close()
			return err
		}
		sizes = append(sizes, size)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	raw := &Library{cacheDir: l.cacheDir}
	for _, size := range sizes {
		for _, path := range raw.thumbnailCachePaths(e.CachePath, size) {
			rel, err := filepath.Rel(l.cacheDir, path)
			if err != nil {
				return err
			}
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO photo_cache_gc(path,created_at) VALUES(?,unixepoch())`, filepath.ToSlash(rel)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *Library) invalidateChangedContentTx(ctx context.Context, tx *sql.Tx) error {
	var after int64
	for {
		rows, err := tx.QueryContext(ctx, `SELECT `+prefixEntityColumns("e")+` FROM photo_entities e JOIN photo_identity_scan s ON s.path=e.path AND s.kind=e.kind WHERE e.id>? AND e.missing_since=0 AND e.fingerprint<>'' AND e.fingerprint<>s.fingerprint ORDER BY e.id LIMIT 100`, after)
		if err != nil {
			return err
		}
		var entries []photoEntity
		for rows.Next() {
			e, err := scanEntity(rows)
			if err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, e := range entries {
			if isMediaKind(e.Kind) {
				if err = l.retireMediaCachesTx(ctx, tx, e); err != nil {
					return err
				}
				if _, err = tx.ExecContext(ctx, `UPDATE photo_entities SET cache_path='#entity/'||id||'/'||(SELECT fingerprint FROM photo_identity_scan WHERE path=photo_entities.path) WHERE id=?; DELETE FROM photo_thumbnail_index WHERE media_path=?`, e.ID, e.Path); err != nil {
					return err
				}
				if _, err = tx.ExecContext(ctx, `UPDATE photo_faces SET needs_review=1,source_revision=source_revision+1 WHERE entity_id=? AND source_hash<>(SELECT fingerprint FROM photo_identity_scan WHERE path=photo_faces.path); UPDATE photo_retained_photo_faces SET needs_review=1,source_revision=source_revision+1 WHERE entity_id=? AND source_hash<>(SELECT fingerprint FROM photo_identity_scan WHERE path=photo_retained_photo_faces.path)`, e.ID, e.ID); err != nil {
					return err
				}
			}
			table := ""
			switch e.Kind {
			case "image", "video", "audio":
				table = "media_index"
			case "blog":
				table = "blog_index"
			case "gpx":
				table = "gpx_index"
			}
			if table == "media_index" {
				if _, err = tx.ExecContext(ctx, `UPDATE media_index SET xmp_fingerprint='#content-changed' WHERE path=?`, e.Path); err != nil {
					return err
				}
				table = ""
			}
			if table != "" {
				if _, err = tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET mod_time_unix_nano=-1 WHERE path=?`, table), e.Path); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM photo_folder_scan WHERE path=?`, parentPath(e.Path)); err != nil {
				return err
			}
			after = e.ID
		}
		if len(entries) < 100 {
			return nil
		}
	}
}
