package photos

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"
)

func (c *faceThumbnailCache) start() {
	if c.db == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel, c.done = cancel, make(chan struct{})
	go func() {
		defer close(c.done)
		for ctx.Err() == nil {
			delay := time.Minute
			if err := c.purgeExpired(ctx, time.Now()); err != nil {
				if ctx.Err() == nil {
					slog.Warn("Gesichtsvorschau-Cache bereinigen", "error", err)
				}
			} else {
				var next int64
				if err := c.db.QueryRowContext(ctx, `SELECT coalesce(min(expires_at),0) FROM photo_face_thumbnail_cache WHERE expires_at>0`).Scan(&next); err == nil && next > 0 {
					delay = max(time.Millisecond, min(delay, time.Until(time.Unix(next, 0))))
				}
			}
			// Interleave bounded migration work with expiration cleanup. A large
			// library never delays startup or collects every cache path in memory.
			if more, err := c.backfillLegacyBatch(ctx); err != nil {
				if ctx.Err() == nil {
					slog.Warn("Gesichtsvorschau-Cache zuordnen", "error", err)
				}
			} else if more {
				delay = min(delay, 250*time.Millisecond)
			}

			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

func (c *faceThumbnailCache) purgeExpired(ctx context.Context, now time.Time) error {
	var cursor faceCacheExpiry
	var failures error
	for {
		n, next, err := c.purgeExpiredBatch(ctx, now.Unix(), cursor)
		if failures == nil {
			failures = err
		}
		if n < 100 || ctx.Err() != nil {
			return failures
		}
		cursor = next
	}
}

type faceCacheExpiry struct {
	key     string
	expires int64
}

func (c *faceThumbnailCache) purgeExpiredBatch(ctx context.Context, now int64, cursor faceCacheExpiry) (int, faceCacheExpiry, error) {
	// Serialize file removal with cache publication, keeping each batch bounded.
	c.mu.Lock()
	defer c.mu.Unlock()
	rows, err := c.db.QueryContext(ctx, `SELECT cache_key,expires_at FROM photo_face_thumbnail_cache WHERE expires_at>0 AND expires_at<=? AND (expires_at,cache_key)>(?,?) ORDER BY expires_at,cache_key LIMIT 100`, now, cursor.expires, cursor.key)
	if err != nil {
		return 0, cursor, err
	}
	var entries []faceCacheExpiry
	for rows.Next() {
		var entry faceCacheExpiry
		if err = rows.Scan(&entry.key, &entry.expires); err != nil {
			rows.Close()
			return 0, cursor, err
		}
		entries = append(entries, entry)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, cursor, err
	}
	var failures error
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return 0, cursor, errors.Join(failures, err)
		}
		cursor = entry
		tx, e := c.db.BeginTx(ctx, nil)
		if e != nil {
			return 0, cursor, e
		}
		// Take a write reservation and recheck pinning before unlinking. A
		// concurrent retention/restore/review cannot commit between this check
		// and the unlink; failures leave the persistent journal intact.
		res, e := tx.ExecContext(ctx, `UPDATE photo_face_thumbnail_cache SET expires_at=expires_at WHERE cache_key=? AND expires_at>0 AND expires_at<=? AND NOT EXISTS(SELECT 1 FROM photo_retained_photo_faces r WHERE r.id=face_id) AND NOT EXISTS(SELECT 1 FROM photo_faces f WHERE f.id=face_id AND f.needs_review=1)`, entry.key, now)
		if e != nil {
			tx.Rollback()
			return 0, cursor, e
		}
		n, e := res.RowsAffected()
		if e != nil {
			tx.Rollback()
			return 0, cursor, e
		}
		if n == 0 {
			tx.Rollback()
			continue
		}
		if e = os.Remove(c.path(entry.key)); e != nil && !errors.Is(e, os.ErrNotExist) {
			tx.Rollback()
			failures = errors.Join(failures, e)
			continue
		}
		if _, e = tx.ExecContext(ctx, `DELETE FROM photo_face_thumbnail_cache WHERE cache_key=?`, entry.key); e == nil {
			e = tx.Commit()
		} else {
			tx.Rollback()
		}
		if e != nil {
			failures = errors.Join(failures, e)
		}

	}
	return len(entries), cursor, failures
}
