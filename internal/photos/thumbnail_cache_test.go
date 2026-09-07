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
