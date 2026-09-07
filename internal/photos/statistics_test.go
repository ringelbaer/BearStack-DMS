package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPhotoStatisticsUsesBackgroundSnapshot(t *testing.T) {
	ctx := context.Background()
	lib, err := New(t.TempDir(), t.TempDir(), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	dir := filepath.Join(lib.CacheDir(), "thumbnails")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "one.webp"), []byte("one"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".partial.webp"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "one.webp"), filepath.Join(dir, "symlink.webp")); err != nil {
		t.Fatal(err)
	}
	stats, err := lib.Statistics(ctx)
	if err != nil || !stats.ThumbnailCacheMeasuredAt.IsZero() {
		t.Fatalf("request scanned filesystem: %#v, %v", stats, err)
	}
	if err := lib.RefreshThumbnailCacheStatistics(ctx); err != nil {
		t.Fatal(err)
	}
	first, err := lib.Statistics(ctx)
	if err != nil || first.ThumbnailCacheFiles != 1 || first.ThumbnailCacheBytes != 3 || first.ThumbnailCacheMeasuredAt.IsZero() {
		t.Fatalf("snapshot: %#v, %v", first, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "two.webp"), []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	stale, err := lib.Statistics(ctx)
	if err != nil || stale.ThumbnailCacheFiles != 1 {
		t.Fatalf("request rescanned filesystem: %#v, %v", stale, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := lib.RefreshThumbnailCacheStatistics(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	retained, err := lib.Statistics(ctx)
	if err != nil || retained.ThumbnailCacheFiles != 1 || !retained.ThumbnailCacheMeasuredAt.Equal(first.ThumbnailCacheMeasuredAt) {
		t.Fatalf("cancel replaced snapshot: %#v, %v", retained, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lib.RefreshThumbnailCacheStatistics(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	refreshed, err := lib.Statistics(ctx)
	if err != nil || refreshed.ThumbnailCacheFiles != 2 || refreshed.ThumbnailCacheBytes != 9 {
		t.Fatalf("refresh: %#v, %v", refreshed, err)
	}
}

func TestPhotoCacheStatsWaiterCanCancel(t *testing.T) {
	lib := &Library{statsFlight: &thumbnailCacheStatsFlight{done: make(chan struct{})}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lib.RefreshThumbnailCacheStatistics(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait cancellation: %v", err)
	}
}
