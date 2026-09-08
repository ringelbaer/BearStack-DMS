package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type faceThumbnailFlight struct {
	done chan struct{}
	data []byte
	err  error
}

// Active crops persist without size or count limits. SQLite tracks expirations
// for ignored crops, so idle files can be removed without scanning the cache.
// Only in-flight renders are tracked in memory; cache hits require no directory
// scan, so lookup cost does not grow with the number of cached faces.
type faceThumbnailCache struct {
	mu      sync.Mutex
	dir     string
	root    string
	db      *sql.DB
	closed  bool
	pending map[string]*faceThumbnailFlight
	cancel  context.CancelFunc
	done    chan struct{}
}

func faceThumbnailKey(f RecognizedFace, size int, abs string, sourceSize, sourceModTime int64) string {
	return fmt.Sprintf("%d:%d:%s:%d:%d:%g:%g:%g:%g", f.ID, size, abs, sourceSize, sourceModTime, f.X, f.Y, f.Width, f.Height)
}

func hashFaceThumbnailKey(key string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
}

func (c *faceThumbnailCache) path(key string) string {
	return filepath.Join(c.dir, key[:2], key[2:4], key+".jpg")
}

func (c *faceThumbnailCache) read(ctx context.Context, key string) []byte {
	if c.dir == "" {
		return nil
	}
	var expires int64
	if c.db != nil {
		if err := c.db.QueryRowContext(ctx, `SELECT expires_at FROM photo_face_thumbnail_cache WHERE cache_key=?`, key).Scan(&expires); err != nil {
			return nil
		}
		if expires > 0 && expires <= time.Now().Unix() {
			return nil
		}
	}
	b, _ := os.ReadFile(c.path(key))
	if expires > 0 && expires <= time.Now().Unix() {
		return nil
	}
	return b
}

// Register before publishing the file. The SQL statement reads the current
// ignored state atomically with registration, including edits during rendering.
func (c *faceThumbnailCache) register(ctx context.Context, key string, faceID int64) bool {
	if c.db == nil {
		return true
	}
	now := time.Now().Unix()
	res, err := c.db.ExecContext(ctx, `INSERT INTO photo_face_thumbnail_cache(cache_key,face_id,expires_at)
 SELECT ?,id,CASE WHEN ignored=1 THEN ? ELSE 0 END FROM photo_faces WHERE id=?
 ON CONFLICT(cache_key) DO UPDATE SET expires_at=CASE
 WHEN excluded.expires_at>0 AND photo_face_thumbnail_cache.expires_at>? THEN photo_face_thumbnail_cache.expires_at
 ELSE excluded.expires_at END`, key, now+int64(ignoredFaceThumbnailTTL/time.Second), faceID, now)
	if err != nil {
		return false
	}
	n, err := res.RowsAffected()
	return err == nil && n == 1
}

func (c *faceThumbnailCache) get(ctx context.Context, faceID int64, key string, render func() ([]byte, error)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key = hashFaceThumbnailKey(key)
	// Shard the cache to avoid very large individual directories.
	path := c.path(key)
	// Cache hits can read concurrently; writers publish with an atomic rename.
	if b := c.read(ctx, key); len(b) > 0 {
		return b, nil
	}
	cacheable := true
	c.mu.Lock()
	// Recheck after locking in case a render finished during the first read.
	if c.dir != "" && !c.closed {
		// Old files have no inventory entry yet. Adopt them before rendering;
		// their bytes, path and mtime remain unchanged, even during backfill.
		if c.db != nil {
			cacheable = c.adoptLegacy(ctx, key, faceID) == nil
		}
		if b := c.read(ctx, key); len(b) > 0 {
			c.mu.Unlock()
			return b, nil
		}
	}
	if f := c.pending[key]; f != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.done:
			return bytes.Clone(f.data), f.err
		}
	}
	if c.pending == nil {
		c.pending = make(map[string]*faceThumbnailFlight)
	}
	f := &faceThumbnailFlight{done: make(chan struct{})}
	c.pending[key] = f
	c.mu.Unlock()
	b, err := render()
	c.mu.Lock()
	if err == nil && cacheable && !c.closed && c.dir != "" {
		// A cache failure must not prevent displaying a successfully rendered crop.
		if os.MkdirAll(filepath.Dir(path), 0700) == nil {
			// Never renew the expiry of old bytes if publishing their replacement
			// fails. Missing files with a registered deadline are safe to retry.
			if e := os.Remove(path); (e == nil || errors.Is(e, os.ErrNotExist)) && c.register(ctx, key, faceID) {
				_ = writeFaceCache(path, b)
			}
		}
	}
	f.data, f.err = b, err
	delete(c.pending, key)
	close(f.done)
	c.mu.Unlock()
	return bytes.Clone(b), err
}

func writeFaceCache(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".face-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Atomic publication protects cached files across interrupted writes.
	return os.Rename(f.Name(), path)
}

func (c *faceThumbnailCache) close() {
	c.mu.Lock()
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	done := c.done
	c.mu.Unlock()
	if done != nil {
		<-done
	}
}
