package photos

import (
	"container/list"
	"context"
	"errors"
	"image"
	"os"
	"sync"
)

const faceImageCacheBytes = 32 << 20
const faceImageCacheEntries = 8

var errFaceSourceChanged = errors.New("Foto wurde während der Bildaufbereitung geändert; bitte erneut laden")

type faceImageKey struct {
	path        string
	size, mtime int64
	xmp         string
}

func imageSourceKey(path string, info os.FileInfo) faceImageKey {
	return faceImageKey{path: path, size: info.Size(), mtime: info.ModTime().UnixNano(), xmp: xmpSidecarFingerprint(path)}
}

type faceImageEntry struct {
	key   faceImageKey
	image *image.NRGBA
}

type faceImageFlight struct {
	done  chan struct{}
	image *image.NRGBA
	err   error
}

// Only the immutable, oriented 1600px raster is retained, never the full-size
// source. Both bytes and entry count are bounded. Visibility is checked by each
// caller, including cache hits; sharing a decode grants no access to the image.
type faceImageCache struct {
	mu         sync.Mutex
	entries    map[faceImageKey]*list.Element
	lru        list.List
	bytes      int
	limitBytes int
	pending    map[faceImageKey]*faceImageFlight
	closed     bool
}

func (c *faceImageCache) get(ctx context.Context, key faceImageKey, decode func() (*image.NRGBA, error)) (*image.NRGBA, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, os.ErrClosed
		}
		if entry := c.entries[key]; entry != nil {
			c.lru.MoveToFront(entry)
			img := entry.Value.(faceImageEntry).image
			c.mu.Unlock()
			return img, nil
		}
		if flight := c.pending[key]; flight != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-flight.done:
				if errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded) {
					continue
				}
				return flight.image, flight.err
			}
		}
		if c.pending == nil {
			c.pending = make(map[faceImageKey]*faceImageFlight)
		}
		flight := &faceImageFlight{done: make(chan struct{})}
		c.pending[key] = flight
		c.mu.Unlock()
		img, err := decode()
		c.mu.Lock()
		limit := c.limitBytes
		if limit <= 0 {
			limit = faceImageCacheBytes
		}
		if err == nil && img != nil && !c.closed && len(img.Pix) <= limit {
			for c.lru.Len() > 0 && (c.bytes+len(img.Pix) > limit || c.lru.Len() >= faceImageCacheEntries) {
				oldest := c.lru.Back()
				entry := oldest.Value.(faceImageEntry)
				c.bytes -= len(entry.image.Pix)
				delete(c.entries, entry.key)
				c.lru.Remove(oldest)
			}
			if c.entries == nil {
				c.entries = make(map[faceImageKey]*list.Element)
			}
			c.entries[key] = c.lru.PushFront(faceImageEntry{key, img})
			c.bytes += len(img.Pix)
		}
		flight.image, flight.err = img, err
		delete(c.pending, key)
		close(flight.done)
		c.mu.Unlock()
		return img, err
	}
}

func (c *faceImageCache) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.entries = nil
	c.lru.Init()
	c.bytes = 0
}

func (l *Library) checkFaceImageSource(key faceImageKey) error {
	info, err := os.Stat(key.path)
	if err != nil {
		return err
	}
	if imageSourceKey(key.path, info) != key {
		return errFaceSourceChanged
	}
	return nil
}
