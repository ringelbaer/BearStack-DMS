package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"bearstack/internal/photos/photopath"
)

// Caller holds c.mu, excluding concurrent publication and cleanup. Adoption
// only adds missing metadata: existing bytes and expiry records are untouched.
func (c *faceThumbnailCache) adoptLegacy(ctx context.Context, key string, faceID int64) error {
	info, err := os.Lstat(c.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil
	}
	_, err = c.db.ExecContext(ctx, `INSERT INTO photo_face_thumbnail_cache(cache_key,face_id,expires_at)
 SELECT ?,f.id,CASE WHEN f.ignored=0 THEN 0
 WHEN f.id<=b.upper_id THEN min(b.ignored_expires_at,unixepoch()+?)
 ELSE unixepoch()+? END
 FROM photo_faces f CROSS JOIN photo_face_thumbnail_backfill b WHERE f.id=? AND b.id=1
 ON CONFLICT(cache_key) DO NOTHING`, key, int64(ignoredFaceThumbnailTTL/time.Second), int64(ignoredFaceThumbnailTTL/time.Second), faceID)
	return err
}

// Reconstruct the original cache keys from indexed source fingerprints and face
// bounds. No original image reads, decoding, directory walks or file rewrites.
// Checkpoints make the single pass resumable without extending ignored TTLs.
func (c *faceThumbnailCache) backfillLegacyBatch(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var cursor, upper int64
	if err := c.db.QueryRowContext(ctx, `SELECT cursor,upper_id FROM photo_face_thumbnail_backfill WHERE id=1`).Scan(&cursor, &upper); err != nil {
		return false, err
	}
	if cursor >= upper {
		return false, nil
	}
	rows, err := c.db.QueryContext(ctx, `SELECT f.id,f.path,f.x,f.y,f.width,f.height,coalesce(m.size_bytes,0),coalesce(m.mod_time_unix_nano,0)
 FROM photo_faces f LEFT JOIN media_index m ON m.path=f.path
 WHERE f.id>? AND f.id<=? ORDER BY f.id LIMIT 100`, cursor, upper)
	if err != nil {
		return false, err
	}
	type legacyFace struct {
		face  RecognizedFace
		size  int64
		mtime int64
	}
	var faces []legacyFace
	for rows.Next() {
		var f legacyFace
		if err = rows.Scan(&f.face.ID, &f.face.Path, &f.face.X, &f.face.Y, &f.face.Width, &f.face.Height, &f.size, &f.mtime); err != nil {
			rows.Close()
			return false, err
		}
		faces = append(faces, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, f := range faces {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		rel, err := photopath.Clean(f.face.Path)
		if err != nil {
			return false, err
		}
		abs := filepath.Join(c.root, filepath.FromSlash(rel))
		for _, size := range []int{160, 640} {
			key := hashFaceThumbnailKey(faceThumbnailKey(f.face, size, abs, f.size, f.mtime))
			if err := c.adoptLegacy(ctx, key, f.face.ID); err != nil {
				return false, err
			}
		}
		cursor = f.face.ID
	}
	if len(faces) < 100 {
		cursor = upper
	}
	_, err = c.db.ExecContext(ctx, `UPDATE photo_face_thumbnail_backfill SET cursor=? WHERE id=1`, cursor)
	return cursor < upper, err
}
