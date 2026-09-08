package photos

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestFaceImageCacheSharesAndBoundsImages(t *testing.T) {
	ctx := context.Background()
	c := faceImageCache{limitBytes: 32}
	var calls atomic.Int32
	decode := func() (*image.NRGBA, error) { calls.Add(1); return image.NewNRGBA(image.Rect(0, 0, 2, 2)), nil }
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if _, err := c.get(ctx, faceImageKey{path: "a"}, decode); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("decoded %d times", calls.Load())
	}
	for _, key := range []string{"b", "a", "c"} {
		if _, err := c.get(ctx, faceImageKey{path: key}, decode); err != nil {
			t.Fatal(err)
		}
	}
	if c.entries[faceImageKey{path: "b"}] != nil || c.bytes != 32 || c.lru.Len() != 2 {
		t.Fatal("LRU/byte limit failed")
	}
	large := func() (*image.NRGBA, error) { return image.NewNRGBA(image.Rect(0, 0, 4, 4)), nil }
	if _, err := c.get(ctx, faceImageKey{path: "large"}, large); err != nil {
		t.Fatal(err)
	}
	if c.bytes != 32 || c.entries[faceImageKey{path: "large"}] != nil {
		t.Fatal("oversize image cached")
	}
	c.close()
	if c.bytes != 0 || c.lru.Len() != 0 {
		t.Fatal("cache retained after close")
	}
	if _, err := c.get(ctx, faceImageKey{}, decode); !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	var tiny faceImageCache
	for i := range 20 {
		if _, err := tiny.get(ctx, faceImageKey{mtime: int64(i)}, decode); err != nil {
			t.Fatal(err)
		}
	}
	if tiny.lru.Len() != faceImageCacheEntries {
		t.Fatal("entry limit failed")
	}
}

func TestFaceImageCacheCancellationAndRetry(t *testing.T) {
	var c faceImageCache
	key := faceImageKey{path: "photo"}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := c.get(ctx, key, func() (*image.NRGBA, error) { close(started); <-ctx.Done(); return nil, ctx.Err() })
		finished <- err
	}()
	<-started
	waiter, stop := context.WithCancel(context.Background())
	stop()
	if _, err := c.get(waiter, key, func() (*image.NRGBA, error) { t.Fatal("cancelled waiter decoded"); return nil, nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	retry := make(chan error, 1)
	go func() {
		_, err := c.get(context.Background(), key, func() (*image.NRGBA, error) { return image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil })
		retry <- err
	}()
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := <-retry; err != nil {
		t.Fatalf("leader cancellation leaked to another request: %v", err)
	}
}

func TestSharedFaceImageRefreshesSourceAndVisibility(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	_, photo := finishGroupPhoto(t, l, 6, 0)
	for _, face := range photo.Faces {
		if _, err := l.FaceThumbnail(ctx, face.ID); err != nil {
			t.Fatal(err)
		}
	}
	if l.faceImages.lru.Len() != 1 {
		t.Fatal("one photo must have one prepared raster")
	}
	first, err := l.faceImage(ctx, "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	second, err := l.faceImage(ctx, "a.jpg")
	if err != nil || first != second {
		t.Fatal("prepared raster not reused", err)
	}
	writeXMPFace(t, filepath.Join(l.Root(), "a.jpg"), "Sidecar changed", .3, .3, .2, .2)
	sidecar, err := l.faceImage(ctx, "a.jpg")
	if err != nil || sidecar == first {
		t.Fatal("sidecar fingerprint did not invalidate prepared raster", err)
	}
	writeJPEG(t, filepath.Join(l.Root(), "a.jpg"), color.Black)
	third, err := l.faceImage(ctx, "a.jpg")
	if err != nil || third == first {
		t.Fatal("replacement used old raster", err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), AdminOnlyMarkerName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.faceImage(ctx, "a.jpg"); !errors.Is(err, ErrAdminOnly()) {
		t.Fatal("private cached raster served", err)
	}
	if err := os.Remove(filepath.Join(l.Root(), AdminOnlyMarkerName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(l.Root(), "a.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.faceImage(ctx, "a.jpg"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("deleted cached raster served", err)
	}
}

func BenchmarkFaceImageReuse(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "photo.jpg")
	f, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	err = jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 2000, 2000)), nil)
	f.Close()
	if err != nil {
		b.Fatal(err)
	}
	l, err := New(root, b.TempDir(), filepath.Join(b.TempDir(), "photos.db"), 60)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { l.Close() })
	ctx := context.Background()
	b.Run("decode_each_crop", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := l.decodeFaceImage(ctx, path); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("shared_raster", func(b *testing.B) {
		if _, err := l.faceImage(ctx, "photo.jpg"); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := l.faceImage(ctx, "photo.jpg"); err != nil {
				b.Fatal(err)
			}
		}
	})
}
