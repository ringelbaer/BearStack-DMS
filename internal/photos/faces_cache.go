package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type faceThumbnailFlight struct {
	done chan struct{}
	data []byte
	err  error
}

// Encoded crops persist in the photo cache without size or count limits.
// Only in-flight renders are tracked in memory; cache hits require no directory
// scan, so lookup cost does not grow with the number of cached faces.
type faceThumbnailCache struct {
	mu      sync.Mutex
	dir     string
	closed  bool
	pending map[string]*faceThumbnailFlight
}

func (c *faceThumbnailCache) get(ctx context.Context, key string, render func() ([]byte, error)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%x.jpg", sha256.Sum256([]byte(key)))
	// Shard the cache to avoid very large individual directories.
	path := filepath.Join(c.dir, name[:2], name[2:4], name)
	// Cache hits can read concurrently; writers publish with an atomic rename.
	if c.dir != "" {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return b, nil
		}
	}
	c.mu.Lock()
	// Recheck after locking in case a render finished during the first read.
	if c.dir != "" && !c.closed {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
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
	if err == nil && !c.closed && c.dir != "" {
		// A cache failure must not prevent displaying a successfully rendered crop.
		if os.MkdirAll(filepath.Dir(path), 0700) == nil {
			_ = writeFaceCache(path, b)
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
	defer c.mu.Unlock()
	c.closed = true
}
