package photos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func legacyFaceCacheFile(t *testing.T, l *Library, f RecognizedFace, size int) (string, []byte, time.Time) {
	t.Helper()
	abs := filepath.Join(l.Root(), filepath.FromSlash(f.Path))
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatal(err)
	}
	// Freeze the pre-migration filename format independently of the new helper.
	key := fmt.Sprintf("%d:%d:%s:%d:%d:%g:%g:%g:%g", f.ID, size, abs, info.Size(), info.ModTime().UnixNano(), f.X, f.Y, f.Width, f.Height)
	name := fmt.Sprintf("%x.jpg", sha256.Sum256([]byte(key)))
	path := filepath.Join(l.CacheDir(), "faces", "v1", name[:2], name[2:4], name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	// Distinct bytes prove that migration and API reads never re-rendered a crop.
	data := []byte(fmt.Sprintf("existing cached preview %d/%d", f.ID, size))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1700000000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	return path, data, stamp
}

func assertLegacyFaceCacheFile(t *testing.T, path string, data []byte, stamp time.Time) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(b, data) {
		t.Fatalf("legacy bytes changed: %s %q %v", path, b, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.ModTime().Equal(stamp) {
		t.Fatalf("legacy file rewritten: %s %v", path, err)
	}
}

func awaitLegacyFaceCacheBackfill(t *testing.T, l *Library) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var done bool
		if err := l.index.db.QueryRow(`SELECT cursor>=upper_id FROM photo_face_thumbnail_backfill WHERE id=1`).Scan(&done); err != nil {
			t.Fatal(err)
		}
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("legacy backfill did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLegacyFaceCacheMigrationPreservesPreviews(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	stopFaceCacheCleanup(t, l)
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	a, _ := l.AutomaticFaces(ctx, "a.jpg")
	b, _ := l.AutomaticFaces(ctx, "b.jpg")
	if err := l.EditFaces(ctx, []int64{b[0].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	type preview struct {
		path   string
		data   []byte
		stamp  time.Time
		faceID int64
		size   int
	}
	var previews []preview
	for _, face := range []RecognizedFace{a[0], b[0]} {
		for _, size := range []int{160, 640} {
			path, data, stamp := legacyFaceCacheFile(t, l, face, size)
			previews = append(previews, preview{path, data, stamp, face.ID, size})
		}
	}
	unknown := filepath.Join(l.faceThumbnails.dir, "unassigned.jpg")
	if err := os.WriteFile(unknown, []byte("keep"), 0600); err != nil {
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
	awaitLegacyFaceCacheBackfill(t, reopened)
	stopFaceCacheCleanup(t, reopened)
	var expires int64
	for _, p := range previews {
		assertLegacyFaceCacheFile(t, p.path, p.data, p.stamp)
		// Inventory migration preserves old bytes. The public thumbnail API
		// replaces their stretched rendering separately, only when requested.
		got := reopened.faceThumbnails.read(ctx, strings.TrimSuffix(filepath.Base(p.path), ".jpg"))
		if !bytes.Equal(got, p.data) {
			t.Fatalf("old preview not inventoried: %q", got)
		}
		assertLegacyFaceCacheFile(t, p.path, p.data, p.stamp)
	}
	for _, face := range []RecognizedFace{a[0], b[0]} {
		entries := faceCacheEntries(t, reopened, face.ID)
		if len(entries) != 2 {
			t.Fatalf("missing legacy inventory: %v", entries)
		}
		for _, expiry := range entries {
			if face.ID == a[0].ID && expiry != 0 {
				t.Fatal("active legacy cache expires")
			}
			if face.ID == b[0].ID {
				if expiry <= time.Now().Unix() || expiry > time.Now().Unix()+172800 {
					t.Fatalf("ignored legacy lifetime: %d", expiry)
				}
				expires = expiry
			}
		}
	}
	if err := reopened.faceThumbnails.purgeExpired(ctx, time.Unix(expires, 0)); err != nil {
		t.Fatal(err)
	}
	for _, p := range previews {
		if p.faceID == a[0].ID {
			assertLegacyFaceCacheFile(t, p.path, p.data, p.stamp)
		} else if _, err := os.Stat(p.path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("ignored legacy preview not expired", err)
		}
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "keep" {
		t.Fatal("unassigned legacy file removed", err)
	}
}

func TestLegacyFaceCacheOnDemandAdoption(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	stopFaceCacheCleanup(t, l)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	face := faces[0]
	if err := l.EditFaces(ctx, []int64{face.ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Unix() + 3600
	if _, err := l.index.db.Exec(`UPDATE photo_face_thumbnail_backfill SET cursor=0,upper_id=?,ignored_expires_at=?`, face.ID, deadline); err != nil {
		t.Fatal(err)
	}
	path, data, stamp := legacyFaceCacheFile(t, l, face, 160)
	abs := filepath.Join(l.Root(), face.Path)
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatal(err)
	}
	key := faceThumbnailKey(face, 160, abs, info.Size(), info.ModTime().UnixNano())
	readLegacy := func() ([]byte, error) {
		return l.faceThumbnails.get(ctx, face.ID, key, func() ([]byte, error) {
			return l.renderFaceThumbnail(ctx, face, 160)
		})
	}
	for range 2 {
		b, err := readLegacy()
		if err != nil || !bytes.Equal(b, data) {
			t.Fatalf("rendered before background migration: %q %v", b, err)
		}
		assertLegacyFaceCacheFile(t, path, data, stamp)
	}
	if _, err := l.faceThumbnails.backfillLegacyBatch(ctx); err != nil {
		t.Fatal(err)
	}
	for key, expiry := range faceCacheEntries(t, l, face.ID) {
		if expiry != deadline {
			t.Fatal("adoption or backfill extended deadline")
		}
		if _, err := l.index.db.Exec(`UPDATE photo_face_thumbnail_cache SET expires_at=1 WHERE cache_key=?`, key); err != nil {
			t.Fatal(err)
		}
	}
	b, err := readLegacy()
	if err != nil || bytes.Equal(b, data) {
		t.Fatalf("expired legacy file served: %q %v", b, err)
	}
}

func TestLegacyFaceCacheBackfillResumesWithoutSourceReads(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	stopFaceCacheCleanup(t, l)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	face := faces[0]
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<204)
 INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,manual,ignored)
 SELECT f.path,f.directory,f.person_id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model,1,1 FROM photo_faces f CROSS JOIN n WHERE f.id=?`, face.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Unix() + 3600
	if _, err := l.index.db.Exec(`UPDATE photo_face_thumbnail_backfill SET cursor=0,upper_id=205,ignored_expires_at=?`, deadline); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{101, 205} {
		face.ID = id
		legacyFaceCacheFile(t, l, face, 640)
	}
	// Migration must only use the indexed fingerprint, even if the source is
	// currently unavailable. It must never decode or regenerate a preview.
	if err := os.Remove(filepath.Join(l.Root(), "a.jpg")); err != nil {
		t.Fatal(err)
	}
	more, err := l.faceThumbnails.backfillLegacyBatch(ctx)
	if err != nil || !more {
		t.Fatalf("first batch: %v %v", more, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.faceThumbnails.backfillLegacyBatch(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("migration ignored cancellation", err)
	}
	var cursor int64
	if err := l.index.db.QueryRow(`SELECT cursor FROM photo_face_thumbnail_backfill`).Scan(&cursor); err != nil || cursor != 100 {
		t.Fatalf("checkpoint: %d %v", cursor, err)
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
	awaitLegacyFaceCacheBackfill(t, reopened)
	for _, id := range []int64{101, 205} {
		entries := faceCacheEntries(t, reopened, id)
		if len(entries) != 1 {
			t.Fatal("did not resume adoption", entries)
		}
		for _, expiry := range entries {
			if expiry != deadline {
				t.Fatal("restart extended legacy deadline")
			}
		}
	}
}

func TestLegacyFaceCacheMetadataFailurePreservesFile(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	stopFaceCacheCleanup(t, l)
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	path, data, stamp := legacyFaceCacheFile(t, l, faces[0], 160)
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_cache_adoption BEFORE INSERT ON photo_face_thumbnail_cache BEGIN SELECT RAISE(ABORT,'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	// Rendering can still satisfy this request, but a metadata failure must not
	// remove or rewrite a pre-existing file. A later request can replace it safely.
	if _, err := l.FaceThumbnail(ctx, faces[0].ID); err != nil {
		t.Fatal(err)
	}
	assertLegacyFaceCacheFile(t, path, data, stamp)
	if _, err := l.index.db.Exec(`DROP TRIGGER fail_cache_adoption`); err != nil {
		t.Fatal(err)
	}
	b, err := l.FaceThumbnail(ctx, faces[0].ID)
	if err != nil || bytes.Equal(b, data) {
		t.Fatalf("preview replacement did not recover: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("superseded preview remains: %v", err)
	}
}

func TestLegacyFaceCacheBackfillUsesBoundedIndexRange(t *testing.T) {
	l := faceLibrary(t)
	rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN SELECT f.id,f.path,f.x,f.y,f.width,f.height,coalesce(m.size_bytes,0),coalesce(m.mod_time_unix_nano,0)
 FROM photo_faces f LEFT JOIN media_index m ON m.path=f.path
 WHERE f.id>? AND f.id<=? ORDER BY f.id LIMIT 100`, 100, 100000)
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
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "SEARCH f USING INTEGER PRIMARY KEY") || strings.Contains(plan, "SCAN") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatal("unbounded legacy migration query:", plan)
	}
}
