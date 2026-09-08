package photos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func stopFaceCacheCleanup(t *testing.T, l *Library) {
	t.Helper()
	l.faceThumbnails.cancel()
	<-l.faceThumbnails.done
}

func faceCacheEntries(t *testing.T, l *Library, id int64) map[string]int64 {
	t.Helper()
	rows, err := l.index.db.Query(`SELECT cache_key,expires_at FROM photo_face_thumbnail_cache WHERE face_id=?`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	entries := map[string]int64{}
	for rows.Next() {
		var key string
		var expiry int64
		if err := rows.Scan(&key, &expiry); err != nil {
			t.Fatal(err)
		}
		entries[key] = expiry
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestIgnoredThumbnailExpiryAndPermanentRestoration(t *testing.T) {
	for _, action := range []string{"web", "labeling"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			l, person, session := labelFixture(t)
			stopFaceCacheCleanup(t, l)
			id := person.Faces[0].ID
			for _, size := range []int{160, 640} {
				if _, err := l.FaceThumbnailSize(ctx, id, size); err != nil {
					t.Fatal(err)
				}
			}
			before := time.Now().Unix()
			if action == "web" {
				if err := l.EditFaces(ctx, []int64{id}, 0, true, ""); err != nil {
					t.Fatal(err)
				}
			} else if _, err := l.ApplyLabelAction(ctx, "manager", person.ID, labelAction(person, session, "ignore")); err != nil {
				t.Fatal(err)
			}
			entries := faceCacheEntries(t, l, id)
			if len(entries) != 2 {
				t.Fatalf("cache sizes: %v", entries)
			}
			var deadline int64
			for key, expiry := range entries {
				if expiry < before+172800 || expiry > time.Now().Unix()+172800 {
					t.Fatalf("expected a two-day deadline, got %d", expiry)
				}
				deadline = expiry
				// Move the deadline closer to prove neither repeated ignore nor reads
				// extend it, without waiting in the test.
				if _, err := l.index.db.Exec(`UPDATE photo_face_thumbnail_cache SET expires_at=expires_at-60 WHERE cache_key=?`, key); err != nil {
					t.Fatal(err)
				}
			}
			deadline -= 60
			if err := l.EditFaces(ctx, []int64{id}, 0, true, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := l.FaceThumbnail(ctx, id); err != nil {
				t.Fatal(err)
			}
			for _, expiry := range faceCacheEntries(t, l, id) {
				if expiry != deadline {
					t.Fatalf("deadline extended: %d != %d", expiry, deadline)
				}
			}
			if err := l.faceThumbnails.purgeExpired(ctx, time.Unix(deadline-1, 0)); err != nil {
				t.Fatal(err)
			}
			for key := range entries {
				if _, err := os.Stat(l.faceThumbnails.path(key)); err != nil {
					t.Fatal("preview removed before its deadline", err)
				}
			}
			if err := l.faceThumbnails.purgeExpired(ctx, time.Unix(deadline, 0)); err != nil {
				t.Fatal(err)
			}
			for key := range entries {
				if _, err := os.Stat(l.faceThumbnails.path(key)); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("expired preview remains on disk", err)
				}
			}
			f, err := l.Face(ctx, id)
			if err != nil || !f.Ignored {
				t.Fatalf("ignored detection lost: %+v %v", f, err)
			}
			var embedding []byte
			if err := l.index.db.QueryRow(`SELECT embedding FROM photo_faces WHERE id=?`, id).Scan(&embedding); err != nil || decodeVector(embedding) == nil {
				t.Fatalf("embedding lost: %v", err)
			}
			// Ignored previews can be regenerated, and restoration works indefinitely.
			if _, err := l.FaceThumbnail(ctx, id); err != nil {
				t.Fatal(err)
			}
			if err := l.EditFaces(ctx, []int64{id}, 0, false, "Wiederhergestellt"); err != nil {
				t.Fatal(err)
			}
			for _, expiry := range faceCacheEntries(t, l, id) {
				if expiry != 0 {
					t.Fatal("restored preview still expires")
				}
			}
			if err := l.faceThumbnails.purgeExpired(ctx, time.Now().Add(365*24*time.Hour)); err != nil {
				t.Fatal(err)
			}
			if len(faceCacheEntries(t, l, id)) != 1 {
				t.Fatal("active preview removed")
			}
			f, err = l.Face(ctx, id)
			if err != nil || f.Ignored || f.Name != "Wiederhergestellt" {
				t.Fatalf("restore: %+v %v", f, err)
			}
		})
	}
}

func TestExpiredThumbnailIsNotServedAndCleanupRetries(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	stopFaceCacheCleanup(t, l)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].ID
	original, err := l.FaceThumbnail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{id}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	for key := range faceCacheEntries(t, l, id) {
		path := l.faceThumbnails.path(key)
		if err := os.WriteFile(path, []byte("expired jpeg"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := l.index.db.Exec(`UPDATE photo_face_thumbnail_cache SET expires_at=1 WHERE cache_key=?`, key); err != nil {
			t.Fatal(err)
		}
		b, err := l.FaceThumbnail(ctx, id)
		if err != nil || !bytes.Equal(b, original) {
			t.Fatalf("expired cache hit: %q %v", b, err)
		}
		// A failed unlink must leave its durable queue entry for a later retry.
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		blocker := filepath.Join(path, "blocker")
		if err := os.WriteFile(blocker, nil, 0600); err != nil {
			t.Fatal(err)
		}
		future := time.Now().Add(ignoredFaceThumbnailTTL + time.Hour)
		if err := l.faceThumbnails.purgeExpired(ctx, future); err == nil {
			t.Fatal("expected unlink failure")
		}
		if len(faceCacheEntries(t, l, id)) != 1 {
			t.Fatal("lost cleanup retry")
		}
		if err := os.Remove(blocker); err != nil {
			t.Fatal(err)
		}
		if err := l.faceThumbnails.purgeExpired(ctx, future); err != nil {
			t.Fatal(err)
		}
		if len(faceCacheEntries(t, l, id)) != 0 {
			t.Fatal("successful retry left queue entry")
		}
	}
}

func TestIgnoreDuringThumbnailRenderAndRestartCleanup(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	stopFaceCacheCleanup(t, l)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].ID
	started, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := l.faceThumbnails.get(ctx, id, "slow-render", func() ([]byte, error) {
			close(started)
			<-release
			return []byte("jpeg"), nil
		})
		finished <- err
	}()
	<-started
	err := l.EditFaces(ctx, []int64{id}, 0, true, "")
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	entries := faceCacheEntries(t, l, id)
	if len(entries) != 1 {
		t.Fatal(entries)
	}
	for _, expiry := range entries {
		if expiry <= time.Now().Unix() || expiry > time.Now().Unix()+172800 {
			t.Fatalf("concurrent ignore lost: %d", expiry)
		}
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	stopFaceCacheCleanup(t, reopened)
	for key, expiry := range faceCacheEntries(t, reopened, id) {
		if expiry != entries[key] {
			t.Fatal("restart extended expiry")
		}
	}
	// Start with a deadline near enough to exercise the actual timer worker.
	if _, err := reopened.index.db.Exec(`UPDATE photo_face_thumbnail_cache SET expires_at=?`, time.Now().Unix()+1); err != nil {
		t.Fatal(err)
	}
	reopened.faceThumbnails.start()
	deadline := time.Now().Add(5 * time.Second)
	for len(faceCacheEntries(t, reopened, id)) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("idle thumbnail not cleaned automatically")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for key := range entries {
		if _, err := os.Stat(reopened.faceThumbnails.path(key)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("worker left cached file", err)
		}
	}
}

func TestFaceThumbnailExpiryQueryUsesIndex(t *testing.T) {
	l := faceLibrary(t)
	rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN SELECT cache_key,expires_at FROM photo_face_thumbnail_cache WHERE expires_at>0 AND expires_at<=? AND (expires_at,cache_key)>(?,?) ORDER BY expires_at,cache_key LIMIT 100`, time.Now().Unix(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "idx_face_thumbnail_expiry") || strings.Contains(plan, "SCAN") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatal("unbounded cleanup query:", plan)
	}
}

func TestFaceThumbnailExpiryDrainsBatchesAndDeletedFaces(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	stopFaceCacheCleanup(t, l)
	finishFace(t, l, 0)
	ctx := context.Background()
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].ID
	for i := range 205 {
		key := fmt.Sprintf("%064x", i)
		if _, err := l.index.db.Exec(`INSERT INTO photo_face_thumbnail_cache VALUES(?,?,0)`, key, id); err != nil {
			t.Fatal(err)
		}
		path := l.faceThumbnails.path(key)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("jpeg"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.index.db.Exec(`DELETE FROM photo_faces WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	// One failed unlink in the first batch must not starve later expirations.
	blocked := l.faceThumbnails.path(fmt.Sprintf("%064x", 0))
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(blocked, "blocker")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := l.faceThumbnails.purgeExpired(ctx, time.Now()); err == nil {
		t.Fatal("expected failed unlink")
	}
	if len(faceCacheEntries(t, l, id)) != 1 {
		t.Fatal("one failed unlink blocked later batches")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err := l.faceThumbnails.purgeExpired(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(l.faceThumbnails.dir, "*", "*", "*.jpg"))
	if err != nil || len(files) != 0 || len(faceCacheEntries(t, l, id)) != 0 {
		t.Fatalf("incomplete batches: %v %v", files, err)
	}
}

func TestFaceThumbnailMigrationPreservesIgnoredData(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	stopFaceCacheCleanup(t, l)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].ID
	if err := l.EditFaces(ctx, []int64{id}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`DROP TRIGGER face_thumbnail_ignore; DROP TRIGGER face_thumbnail_delete; DROP TABLE photo_face_thumbnail_cache; DROP TABLE photo_face_thumbnail_backfill; UPDATE schema_migrations SET version=20 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	face, err := reopened.Face(ctx, id)
	if err != nil || !face.Ignored {
		t.Fatalf("migration lost ignored face: %+v %v", face, err)
	}
	if _, err := reopened.FaceThumbnail(ctx, id); err != nil {
		t.Fatal(err)
	}
	for _, expiry := range faceCacheEntries(t, reopened, id) {
		if expiry <= time.Now().Unix() || expiry > time.Now().Unix()+172800 {
			t.Fatalf("legacy ignored preview has no bounded lifetime: %d", expiry)
		}
	}
	if err := reopened.EditFaces(ctx, []int64{id}, 0, false, "Wiederhergestellt"); err != nil {
		t.Fatal(err)
	}
}
