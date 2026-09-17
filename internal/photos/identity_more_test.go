package photos

import (
	"bearstack/internal/facerec"
	"bytes"
	"context"
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func identityAt(t *testing.T, l *Library, path string) photoEntity {
	t.Helper()
	e, err := scanEntity(l.index.db.QueryRow(`SELECT `+entityColumns+` FROM photo_entities WHERE path=?`, path))
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func rebuildIdentity(t *testing.T, l *Library) {
	t.Helper()
	if _, err := l.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func copyPhotoTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0750)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, b, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPhotoCopyThenDeleteAndConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "copy", true: "manual conflict"}[conflict], func(t *testing.T) {
			l := faceLibrary(t, "old/a.jpg")
			finishFace(t, l, 0)
			fs, _ := l.AutomaticFaces(context.Background(), "old/a.jpg")
			before := identityAt(t, l, "old/a.jpg")
			preview, err := l.FaceThumbnail(context.Background(), fs[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			copyPhotoTree(t, filepath.Join(l.root, "old"), filepath.Join(l.root, "new"))
			rebuildIdentity(t, l)
			dest := identityAt(t, l, "new/a.jpg")
			if before.ID == dest.ID {
				t.Fatal("copies collapsed")
			}
			if conflict {
				if err = l.index.setMediaTags(context.Background(), "new/a.jpg", []string{"independent"}); err != nil {
					t.Fatal(err)
				}
			}
			if err = os.RemoveAll(filepath.Join(l.root, "old")); err != nil {
				t.Fatal(err)
			}
			rebuildIdentity(t, l)
			after := identityAt(t, l, "new/a.jpg")
			if conflict {
				if after.ID != dest.ID {
					t.Fatal("manual target overwritten")
				}
				if identityAt(t, l, "old/a.jpg").MissingSince == 0 {
					t.Fatal("missing source not retained")
				}
				return
			}
			if after.ID != before.ID {
				t.Fatal("source identity lost")
			}
			got, err := l.FaceThumbnail(context.Background(), fs[0].ID)
			if err != nil || !bytes.Equal(got, preview) {
				t.Fatal("copy preview lost", err)
			}
		})
	}
}

func TestPhotoRelocationNestedMixedContent(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "old/sub/a.jpg", "old/sub/b.jpg", "old/gone.jpg")
	writeJPEG(t, filepath.Join(l.root, "old/sub/b.jpg"), color.Black)
	if err := os.WriteFile(filepath.Join(l.root, "old/entry.md"), []byte("# Before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	first := identityAt(t, l, "old/sub/a.jpg")
	second := identityAt(t, l, "old/sub/b.jpg")
	blog := identityAt(t, l, "old/entry.md")
	if err := l.index.setMediaTags(ctx, "old/sub/a.jpg", []string{"kept"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(l.root, "old"), filepath.Join(l.root, "new")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(l.root, "new/gone.jpg")); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(l.root, "new/added.jpg"), color.RGBA{R: 255, A: 255})
	if err := os.WriteFile(filepath.Join(l.root, "new/entry.md"), []byte("# After\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if identityAt(t, l, "new/sub/a.jpg").ID != first.ID || identityAt(t, l, "new/sub/b.jpg").ID != second.ID || identityAt(t, l, "new/entry.md").ID != blog.ID {
		t.Fatal("subtree identities not transferred")
	}
	if identityAt(t, l, "new/gone.jpg").MissingSince == 0 {
		t.Fatal("deleted descendant not retained")
	}
}

func TestPhotoRelocationPrivacyAndReviewDecision(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "old/a.jpg")
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "old/a.jpg")
	id := fs[0].ID
	if _, err := l.FaceThumbnail(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(l.root, "old"), filepath.Join(l.root, "new")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "new/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if _, err := l.FaceThumbnail(ctx, id); err == nil {
		t.Fatal("private preview visible")
	}
	if err := os.Remove(filepath.Join(l.root, "new/.adminonly")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if _, err := l.FaceThumbnail(ctx, id); err != nil {
		t.Fatal("private archival failed", err)
	}
	writeJPEG(t, filepath.Join(l.root, "new/a.jpg"), color.Black)
	rebuildIdentity(t, l)
	review, err := l.FaceSourceReview(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	box := FaceRegion{X: review.Face.X, Y: review.Face.Y, Width: review.Face.Width, Height: review.Face.Height}
	if err = l.ConfirmFaceSource(ctx, id, "stale", box, "", nil); !errors.Is(err, ErrLabelConflict) {
		t.Fatal("stale accepted", err)
	}
	if err = l.ConfirmFaceSource(ctx, id, review.Revision, box, "Ada", nil); err != nil {
		t.Fatal(err)
	}
	f, err := l.Face(ctx, id)
	if err != nil || f.NeedsReview || f.Name != "Ada" {
		t.Fatal(f, err)
	}
	if _, err = l.SetFaceFavorite(ctx, id, f.PersonID, true); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references WHERE face_id=?`, id).Scan(&n); err != nil || n != 0 {
		t.Fatal("historical vector reused", n, err)
	}
}

func TestPhotoMissingDeadlineAndOfflineRoot(t *testing.T) {
	l := faceLibrary(t, "old/a.jpg", "keep.jpg")
	if err := os.RemoveAll(filepath.Join(l.root, "old")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	before := identityAt(t, l, "old/a.jpg")
	rebuildIdentity(t, l)
	if identityAt(t, l, "old/a.jpg").MissingSince != before.MissingSince {
		t.Fatal("deadline extended")
	}
	if _, err := l.index.db.Exec(`UPDATE photo_entities SET missing_since=? WHERE missing_since>0`, time.Now().Add(-PhotoRetention-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(l.root, "keep.jpg")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if identityAt(t, l, "old/a.jpg").ID != before.ID {
		t.Fatal("offline root purged history")
	}
}

func TestPhotoIdentityReadonlyMount(t *testing.T) {
	root := os.Getenv("BEARSTACK_READONLY_TEST_ROOT")
	if root == "" {
		t.Skip("set BEARSTACK_READONLY_TEST_ROOT to a disposable read-only mount")
	}
	if err := os.WriteFile(filepath.Join(root, "bearstack-write-probe"), []byte("probe"), 0600); err == nil {
		os.Remove(filepath.Join(root, "bearstack-write-probe"))
		t.Fatal("fixture is writable")
	}
	l, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	rebuildIdentity(t, l)
	rebuildIdentity(t, l)
	state, err := l.PhotoIdentities(context.Background(), 0)
	if err != nil || state.Fingerprinted == 0 || state.Fingerprinted != state.Files {
		t.Fatal(state, err)
	}
}

func TestPhotoIdentityWarmScanAndReviewPins(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg")
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "album/a.jpg")
	id := fs[0].ID
	if _, err := l.FaceThumbnail(ctx, id); err != nil {
		t.Fatal(err)
	}
	// A normal unchanged pass retains the manifest without hashing/decode writes.
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_warm_hash BEFORE UPDATE ON photo_entities WHEN new.fingerprint<>old.fingerprint BEGIN SELECT RAISE(ABORT,'unexpected hash update');END; CREATE TRIGGER fail_warm_manifest BEFORE INSERT ON photo_identity_scan WHEN new.kind<>'folder' BEGIN SELECT RAISE(ABORT,'unexpected manifest rewrite');END`); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if _, err := l.index.db.Exec(`DROP TRIGGER fail_warm_hash;DROP TRIGGER fail_warm_manifest`); err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{id}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(l.root, "album/a.jpg"), color.Black)
	rebuildIdentity(t, l)
	if _, _, err := l.faceThumbnails.purgeExpiredBatch(ctx, time.Now().Add(14*24*time.Hour).Unix(), faceCacheExpiry{}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.FaceThumbnail(ctx, id); err != nil {
		t.Fatal("ignored review preview expired", err)
	}
}

func TestPhotoIdentityAmbiguityAndAtomicRollback(t *testing.T) {
	for _, mode := range []string{"duplicate", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			l := faceLibrary(t, "old/a.jpg")
			before := identityAt(t, l, "old/a.jpg")
			copyPhotoTree(t, filepath.Join(l.root, "old"), filepath.Join(l.root, "target"))
			if mode == "duplicate" {
				copyPhotoTree(t, filepath.Join(l.root, "old"), filepath.Join(l.root, "competitor"))
			}
			if err := os.RemoveAll(filepath.Join(l.root, "old")); err != nil {
				t.Fatal(err)
			}
			if mode == "rollback" {
				if _, err := l.index.db.Exec(`CREATE TRIGGER fail_relocation BEFORE INSERT ON photo_relocations BEGIN SELECT RAISE(ABORT,'injected rollback');END`); err != nil {
					t.Fatal(err)
				}
				if _, err := l.RebuildIndex(context.Background()); err == nil {
					t.Fatal("injection not hit")
				}
				if identityAt(t, l, "old/a.jpg").ID != before.ID {
					t.Fatal("partial mapping committed")
				}
				if _, err := l.index.db.Exec(`DROP TRIGGER fail_relocation`); err != nil {
					t.Fatal(err)
				}
				rebuildIdentity(t, l)
				if identityAt(t, l, "target/a.jpg").ID != before.ID {
					t.Fatal("retry lost identity")
				}
			} else {
				rebuildIdentity(t, l)
				if identityAt(t, l, "target/a.jpg").ID == before.ID || identityAt(t, l, "competitor/a.jpg").ID == before.ID {
					t.Fatal("ambiguous mapping accepted")
				}
			}
		})
	}
}

func TestPhotoIdentityManualEmptyFolderAndRestart(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "old/a.jpg", "keep.jpg")
	finishFace(t, l, 0)
	before := identityAt(t, l, "old/a.jpg")
	outside := filepath.Join(t.TempDir(), "away")
	if err := os.Rename(filepath.Join(l.root, "old"), outside); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	missing := identityAt(t, l, "old")
	root, cache, db := l.root, l.cacheDir, l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal("missing folder prevented restart", err)
	}
	defer reopened.Close()
	if identityAt(t, reopened, "old").MissingSince != missing.MissingSince {
		t.Fatal("restart extended retention")
	}
	if err = os.Mkdir(filepath.Join(root, "empty"), 0750); err != nil {
		t.Fatal(err)
	}
	if err = reopened.ResolvePhotoRelocation(ctx, missing.ID, missing.Revision+1, "empty"); !errors.Is(err, ErrLabelConflict) {
		t.Fatal("stale manual mapping accepted", err)
	}
	if err = reopened.ResolvePhotoRelocation(ctx, missing.ID, missing.Revision, "empty"); err != nil {
		t.Fatal(err)
	}
	got := identityAt(t, reopened, "empty/a.jpg")
	if got.ID != before.ID || got.MissingSince == 0 {
		t.Fatal("missing descendant lost", got)
	}
}

func TestPhotoIdentityMigrationPreservesLegacyDataAndBytes(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "old/a.jpg")
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "old/a.jpg")
	id := fs[0].ID
	if err := l.RenamePerson(ctx, fs[0].PersonID, "Legacy Ada"); err != nil {
		t.Fatal(err)
	}
	preview, err := l.FaceThumbnail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	// Remove only the new migration's objects and columns to obtain a version-35
	// database. The real preexisting tables, IDs, cache inventory and files remain.
	rows, err := l.index.db.Query(`SELECT type,name,sql FROM sqlite_master WHERE type IN('trigger','index','table') AND sql IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	type object struct{ kind, name, sql string }
	var objects []object
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.kind, &o.name, &o.sql); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, o)
	}
	rows.Close()
	for _, o := range objects {
		if o.kind == "trigger" && (strings.Contains(o.sql, "photo_entities") || strings.Contains(o.sql, "photo_retained_") || strings.Contains(o.sql, "needs_review")) {
			if _, err = l.index.db.Exec(`DROP TRIGGER "` + o.name + `"`); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, o := range objects {
		if o.kind == "index" && (strings.Contains(o.sql, "needs_review") || strings.Contains(o.sql, "entity_id")) {
			if _, err = l.index.db.Exec(`DROP INDEX IF EXISTS "` + o.name + `"`); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, o := range objects {
		if o.kind == "table" && (strings.HasPrefix(o.name, "photo_identity_") || strings.HasPrefix(o.name, "photo_retained_") || o.name == "photo_entities" || o.name == "photo_relocations" || o.name == "photo_relocation_plan" || o.name == "photo_cache_gc" || o.name == "photo_hidden_person_names") {
			if _, err = l.index.db.Exec(`DROP TABLE "` + o.name + `"`); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, col := range []string{"entity_id", "needs_review", "embedding_current", "source_revision", "source_hash", "source_path", "source_size", "source_mtime"} {
		if _, err = l.index.db.Exec(`ALTER TABLE photo_faces DROP COLUMN ` + col); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = l.index.db.Exec(`ALTER TABLE photo_thumbnail_index DROP COLUMN content_verified; UPDATE schema_migrations SET version=35 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.root, l.cacheDir, l.DBPath()
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	got, err := migrated.Face(ctx, id)
	if err != nil || got.Name != "Legacy Ada" || got.PersonID != fs[0].PersonID {
		t.Fatal(got, err)
	}
	if b, err := migrated.FaceThumbnail(ctx, id); err != nil || !bytes.Equal(b, preview) {
		t.Fatal("migration rewrote preview", err)
	}
	if _, err = migrated.BackfillPhotoIdentities(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(root, "old"), filepath.Join(root, "new")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, migrated)
	got, err = migrated.Face(ctx, id)
	if err != nil || got.Path != "new/a.jpg" {
		t.Fatal(got, err)
	}
}

func TestPhotoIdentityMixedSidecarsTagsAndReanalysis(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "old/a.jpg", "old/b.jpg")
	writeJPEG(t, filepath.Join(l.root, "old/b.jpg"), color.Black)
	for name, body := range map[string]string{"entry.md": "# Original blog\n", "track.gpx": "<gpx/>", "clip.mp4": "video fixture"} {
		if err := os.WriteFile(filepath.Join(l.root, "old", name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rebuildIdentity(t, l)
	if err := l.index.setFolderTags(ctx, Folder{Path: "old", Name: "old", Tags: []string{"trip"}}); err != nil {
		t.Fatal(err)
	}
	setBlogTagsForTest(t, l, "old/entry.md", []string{"diary"})
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "old/a.jpg")
	id := fs[0].ID
	preview, err := l.FaceThumbnail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	blog, track, video := identityAt(t, l, "old/entry.md"), identityAt(t, l, "old/track.gpx"), identityAt(t, l, "old/clip.mp4")
	if err = os.Rename(filepath.Join(l.root, "old"), filepath.Join(l.root, "new")); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"entry.md": "# Updated blog\n", "track.gpx": "<gpx><metadata><name>Updated</name></metadata></gpx>", "added.mp4": "added video"} {
		if err = os.WriteFile(filepath.Join(l.root, "new", name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeXMPFace(t, filepath.Join(l.root, "new/a.jpg"), "XMP Ada", .35, .35, .3, .3)
	rebuildIdentity(t, l)
	if identityAt(t, l, "new/entry.md").ID != blog.ID || identityAt(t, l, "new/track.gpx").ID != track.ID || identityAt(t, l, "new/clip.mp4").ID != video.ID {
		t.Fatal("mixed identities changed")
	}
	var tags string
	if err = l.index.db.QueryRow(`SELECT tags FROM folder_index WHERE path='new'`).Scan(&tags); err != nil || tags != "[\"trip\"]" {
		t.Fatal(tags, err)
	}
	post, err := l.Blog(ctx, "new/entry.md", false)
	if err != nil || !strings.Contains(post.Text, "Updated") || len(post.Tags) != 1 || post.Tags[0] != "diary" {
		t.Fatal(post, err)
	}
	// Drain both queued photos; an XMP edit must retain the matched face ID/crop.
	for i := 0; i < 2; i++ {
		job, err := l.NextFaceJob(ctx)
		if err != nil {
			break
		}
		if err = l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(0)}}); err != nil {
			t.Fatal(err)
		}
	}
	f, err := l.Face(ctx, id)
	if err != nil || f.Path != "new/a.jpg" || f.NeedsReview {
		t.Fatal(f, err)
	}
	if b, err := l.FaceThumbnail(ctx, id); err != nil || !bytes.Equal(b, preview) {
		t.Fatal("XMP reanalysis replaced cache", err)
	}
}

func TestPhotoIdentityDetectsSameSizeAndMtimeChanges(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	path := filepath.Join(l.root, "a.jpg")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, 'a')
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := fs[0].ID
	before, err := l.FaceThumbnail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	old := identityAt(t, l, "a.jpg")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] = 'b'
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	f, err := l.Face(ctx, id)
	if err != nil || !f.NeedsReview {
		t.Fatal(f, err)
	}
	after := identityAt(t, l, "a.jpg")
	if old.CachePath == after.CachePath || old.Fingerprint == after.Fingerprint {
		t.Fatal("changed content accepted as old")
	}
	if b, err := l.FaceThumbnail(ctx, id); err != nil || !bytes.Equal(before, b) {
		t.Fatal("historical face snapshot lost", err)
	}
}

func BenchmarkPhotoIdentityWarm10k(b *testing.B) {
	l := newBenchmarkFilesystemLibrary(b, 100, 10000)
	defer l.Close()
	ctx := context.Background()
	if _, err := l.RebuildIndex(ctx); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := l.RebuildIndex(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPhotoIdentityConflictingNestedMovesStayOpen(t *testing.T) {
	l := faceLibrary(t, "old/a.jpg", "old/b.jpg", "old/sub/c.jpg", "old/sub/d.jpg")
	for name, c := range map[string]color.Color{"old/b.jpg": color.Black, "old/sub/c.jpg": color.RGBA{R: 255, A: 255}, "old/sub/d.jpg": color.RGBA{G: 255, A: 255}} {
		writeJPEG(t, filepath.Join(l.root, name), c)
	}
	rebuildIdentity(t, l)
	before := identityAt(t, l, "old")
	if err := os.Rename(filepath.Join(l.root, "old"), filepath.Join(l.root, "target")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(l.root, "target/sub"), filepath.Join(l.root, "split")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if identityAt(t, l, "target").ID == before.ID || identityAt(t, l, "old").MissingSince == 0 {
		t.Fatal("contradictory nested mapping published")
	}
}

func TestPhotoIdentityCachedThumbnailUsesCurrentPermissions(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg")
	m, err := l.Media("album/a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	cache := l.thumbnailCachePath(m.Path, 420)
	if err = os.MkdirAll(filepath.Dir(cache), 0750); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(cache, []byte("thumbnail"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = l.markThumbnailGenerated(ctx, m, 420); err != nil {
		t.Fatal(err)
	}
	if _, ready, err := l.CachedThumbnailContext(ctx, m.Path, 420, false); err != nil || !ready {
		t.Fatal(ready, err)
	}
	if err = os.WriteFile(filepath.Join(l.root, "album", AdminOnlyMarkerName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.CachedThumbnailContext(ctx, m.Path, 420, false); !errors.Is(err, ErrAdminOnly()) {
		t.Fatal("cached private photo disclosed", err)
	}
}

func TestPhotoIdentityCacheGCResumesAndRejectsEscapes(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	dir := filepath.Join(l.cacheDir, "blocked")
	if err := os.MkdirAll(dir, 0750); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(dir, "keep")
	if err := os.WriteFile(child, []byte("kept"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_cache_gc VALUES('blocked',unixepoch())`); err != nil {
		t.Fatal(err)
	}
	if err := l.processPhotoCacheGC(ctx); err == nil {
		t.Fatal("failed unlink was not reported")
	}
	var n int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_cache_gc`).Scan(&n); err != nil || n != 1 {
		t.Fatal("journal lost", n, err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := l.processPhotoCacheGC(ctx); err != nil {
		t.Fatal("retry", err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "keep"), []byte("kept"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(l.cacheDir, "outside")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_cache_gc VALUES('outside/keep',unixepoch())`); err != nil {
		t.Fatal(err)
	}
	if err := l.processPhotoCacheGC(ctx); err == nil {
		t.Fatal("symlink escape accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "keep")); err != nil {
		t.Fatal("outside cache file removed", err)
	}
}

func TestPhotoIdentityPreservesLegacyReviewPreview(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "a.jpg")
	f := fs[0]
	source := filepath.Join(l.root, "a.jpg")
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	key := hashFaceThumbnailKey(faceThumbnailKey(f, 160, source, info.Size(), info.ModTime().UnixNano()))
	cache := l.faceThumbnails.path(key)
	if err = os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, cache, color.White)
	before, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, source, color.Black)
	rebuildIdentity(t, l)
	if b, err := l.FaceThumbnail(ctx, f.ID); err != nil || !bytes.Equal(b, before) {
		t.Fatal("legacy review snapshot lost", err)
	}
}
