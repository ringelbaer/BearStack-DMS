package photos

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThumbnailCacheHitsDoNotWaitForSQLiteWriter(t *testing.T) {
	for _, initial := range []string{"missing", thumbnailStatusQueued, thumbnailStatusFailed, thumbnailStatusGenerated} {
		t.Run(initial, func(t *testing.T) {
			l := photoTagFixture(t)
			ctx := context.Background()
			media, err := l.MediaContext(ctx, "album/photo.jpg")
			if err != nil {
				t.Fatal(err)
			}
			path := l.thumbnailCachePath(media.Path, 120)
			if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("cached thumbnail"), 0600); err != nil {
				t.Fatal(err)
			}
			if initial != "missing" {
				if err := l.markThumbnailGenerated(ctx, media, 120); err != nil {
					t.Fatal(err)
				}
				if _, err := l.index.db.Exec(`UPDATE photo_thumbnail_index SET status=? WHERE media_path=? AND size=120`, initial, media.Path); err != nil {
					t.Fatal(err)
				}
			}
			if _, ready, err := l.CachedThumbnailContext(ctx, media.Path, 120, true); err != nil || !ready {
				t.Fatalf("initial cache hit: %v %v", ready, err)
			}
			var status string
			if err := l.index.db.QueryRow(`SELECT status FROM photo_thumbnail_index WHERE media_path=? AND size=120`, media.Path).Scan(&status); err != nil || status != thumbnailStatusGenerated {
				t.Fatalf("repair status=%q err=%v", status, err)
			}
			tx, err := l.index.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`UPDATE media_index SET name=name WHERE path=?`, media.Path); err != nil {
				t.Fatal(err)
			}
			requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, ready, err := l.CachedThumbnailContext(requestCtx, media.Path, 120, true)
				if err == nil && !ready {
					err = os.ErrNotExist
				}
				done <- err
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("cache hit waited for SQLite writer")
			}
		})
	}
}

func TestLargerThumbnailSourceDoesNotWaitForSQLiteWriter(t *testing.T) {
	for _, first := range []string{"ready", "missing", "empty", "stale", "changed-source", "legacy"} {
		t.Run(first, func(t *testing.T) {
			l := photoTagFixture(t)
			ctx := context.Background()
			media, err := l.MediaContext(ctx, "album/photo.jpg")
			if err != nil {
				t.Fatal(err)
			}
			want := l.thumbnailCachePath(media.Path, 480)
			if first == "legacy" {
				want = l.legacyThumbnailCachePath(media.Path, 480)
			}
			for _, size := range []int{480, 960} {
				cachePath := l.thumbnailCachePath(media.Path, size)
				if first == "legacy" && size == 480 {
					cachePath = want
				}
				if err := os.MkdirAll(filepath.Dir(cachePath), 0750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(cachePath, []byte("cached thumbnail"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := l.markThumbnailGenerated(ctx, media, size); err != nil {
					t.Fatal(err)
				}
			}
			var changeErr error
			switch first {
			case "missing":
				changeErr = os.Remove(want)
			case "empty":
				changeErr = os.Truncate(want, 0)
			case "stale":
				old := media.ModTime.Add(-time.Hour)
				changeErr = os.Chtimes(want, old, old)
			case "changed-source":
				_, changeErr = l.index.db.Exec(`UPDATE photo_thumbnail_index SET source_size_bytes=source_size_bytes+1 WHERE media_path=? AND size=480`, media.Path)
			}
			if changeErr != nil {
				t.Fatal(changeErr)
			}
			if first != "ready" && first != "legacy" {
				want = l.thumbnailCachePath(media.Path, 960)
			}
			// An ignore transaction can occupy one of the two pool connections.
			// The missing 160px strip preview must reuse a larger cached image
			// using the remaining connection, without waiting for that writer.
			tx, err := l.index.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`UPDATE media_index SET name=name WHERE path=?`, media.Path); err != nil {
				t.Fatal(err)
			}
			requestCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			got, ready := l.cachedLargerThumbnailSource(requestCtx, media, 160)
			if err := requestCtx.Err(); err != nil {
				t.Fatalf("larger thumbnail lookup waited for another database connection: %v", err)
			}
			if !ready || got != want {
				t.Fatalf("larger thumbnail = %q, %v; want %q", got, ready, want)
			}
		})
	}
}

func TestThumbnailGenerationWithOneDatabaseConnection(t *testing.T) {
	l := photoTagFixture(t)
	installFakeWebPThumbnailer(t, 160, 120)
	ctx := context.Background()
	if _, err := l.Thumbnail(ctx, "album/photo.jpg", 960); err != nil {
		t.Fatal(err)
	}
	l.index.db.SetMaxOpenConns(1)
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	got, err := l.Thumbnail(requestCtx, "album/photo.jpg", 160)
	if err != nil {
		t.Fatal(err)
	}
	if got != l.thumbnailCachePath("album/photo.jpg", 160) || !l.CachedThumbnailReady("album/photo.jpg", 160) {
		t.Fatalf("missing generated strip thumbnail: %q", got)
	}
}
