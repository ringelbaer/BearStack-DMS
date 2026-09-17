package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (l *Library) thumbnailIdentityPath(rel string) string {
	if l == nil || !l.index.available() {
		return rel
	}
	var key string
	if err := l.index.db.QueryRow(`SELECT cache_path FROM photo_entities WHERE path=? AND kind IN('image','video') AND missing_since=0`, rel).Scan(&key); err == nil && key != "" {
		return key
	}
	return rel
}

func (l *Library) retainedThumbnailReady(ctx context.Context, m Media, size int) bool {
	if !l.index.available() {
		return false
	}
	var valid bool
	err := l.index.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM photo_thumbnail_index t JOIN photo_entities e ON e.path=t.media_path WHERE e.path=? AND e.kind=? AND e.fingerprint<>'' AND t.content_verified=1 AND t.size=? AND t.status='generated' AND t.source_size_bytes=? AND t.source_mod_time_unix_nano=? AND e.size_bytes=? AND e.mtime=?)`, m.Path, m.Type, size, m.SizeBytes, m.ModTime.UnixNano(), m.SizeBytes, m.ModTime.UnixNano()).Scan(&valid)
	return err == nil && valid
}

func (l *Library) finishIdentityScan(ctx context.Context) error {
	if err := l.publishIdentityFingerprints(ctx); err != nil {
		return err
	}
	if _, err := l.index.db.ExecContext(ctx, `UPDATE photo_identity_state SET last_complete=unixepoch() WHERE id=1`); err != nil {
		return err
	}
	for {
		rows, err := l.index.db.QueryContext(ctx, `SELECT `+entityColumns+` FROM photo_entities WHERE missing_since>0 AND missing_since<=? ORDER BY missing_since,id LIMIT 100`, time.Now().Add(-PhotoRetention).Unix())
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
			if err := l.purgeRetainedEntity(ctx, e); err != nil {
				return err
			}
		}
		if len(entries) < 100 {
			break
		}
	}
	return l.processPhotoCacheGC(ctx)
}

func (l *Library) purgeRetainedEntity(ctx context.Context, e photoEntity) error {
	rows, err := l.index.db.QueryContext(ctx, `SELECT size FROM photo_retained_photo_thumbnail_index WHERE retention_id=?`, e.ID)
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
	var paths []string
	raw := &Library{cacheDir: l.cacheDir}
	for _, size := range sizes {
		paths = append(paths, raw.thumbnailCachePaths(e.CachePath, size)...)
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, path := range paths {
		rel, err := filepath.Rel(l.cacheDir, path)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO photo_cache_gc(path,created_at) VALUES(?,unixepoch())`, filepath.ToSlash(rel)); err != nil {
			return err
		}
	}
	if err = l.index.purgeEntityTx(ctx, tx, e); err != nil {
		return err
	}
	return tx.Commit()
}

func (l *Library) processPhotoCacheGC(ctx context.Context) error {
	root, err := os.OpenRoot(l.cacheDir)
	if err != nil {
		return err
	}
	defer root.Close()
	for {
		rows, err := l.index.db.QueryContext(ctx, `SELECT path FROM photo_cache_gc ORDER BY path LIMIT 100`)
		if err != nil {
			return err
		}
		var paths []string
		for rows.Next() {
			var p string
			if err = rows.Scan(&p); err != nil {
				rows.Close()
				return err
			}
			paths = append(paths, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, p := range paths {
			if err = ctx.Err(); err != nil {
				return err
			}
			clean := filepath.Clean(filepath.FromSlash(p))
			if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return ErrPathEscapesRoot()
			}
			// Reject symlink ancestors even inside the writable cache.
			parent := filepath.Dir(clean)
			for parent != "." {
				info, e := os.Lstat(filepath.Join(l.cacheDir, parent))
				if e != nil && !errors.Is(e, os.ErrNotExist) {
					return e
				}
				if e == nil && info.Mode()&os.ModeSymlink != 0 {
					return ErrPathEscapesRoot()
				}
				parent = filepath.Dir(parent)
			}
			if err = root.Remove(clean); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if _, err = l.index.db.ExecContext(ctx, `DELETE FROM photo_cache_gc WHERE path=?`, p); err != nil {
				return err
			}
		}
		if len(paths) < 100 {
			return nil
		}
	}
}
