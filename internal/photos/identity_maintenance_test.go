package photos

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestRetainedSchemaMigrationPreservesRowsAndColumnMapping(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg", "keep.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "album/a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatal(faces, err)
	}
	face := faces[0]
	preview, err := l.FaceThumbnail(ctx, face.ID)
	if err != nil {
		t.Fatal(err)
	}
	offline := filepath.Join(t.TempDir(), "offline")
	if err = os.Rename(filepath.Join(l.root, "album"), offline); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	// Simulate additive live migrations and a differently ordered retained table.
	// These extra columns stand in for a future feature, not production fields.
	if _, err = l.index.db.Exec(`ALTER TABLE photo_faces ADD COLUMN future_a TEXT NOT NULL DEFAULT 'alpha';
 ALTER TABLE photo_faces ADD COLUMN future_b TEXT NOT NULL DEFAULT 'beta';
 ALTER TABLE photo_retained_photo_faces ADD COLUMN future_b TEXT NOT NULL DEFAULT 'beta';
 UPDATE photo_retained_photo_faces SET future_b='saved';
 CREATE TABLE photo_identity_state(id INTEGER PRIMARY KEY,revision INTEGER,last_complete INTEGER);
 INSERT INTO photo_identity_state VALUES(1,42,123);
 UPDATE schema_migrations SET version=36 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.root, l.cacheDir, l.DBPath()
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err = New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var n int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='photo_identity_state'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("obsolete state survived", n, err)
	}
	if err = l.index.db.QueryRow(`SELECT version FROM schema_migrations WHERE component='photos'`).Scan(&n); err != nil || n != photoSchemaVersion {
		t.Fatal(n, err)
	}
	if err = os.Rename(offline, filepath.Join(root, "album")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	var a, b string
	if err = l.index.db.QueryRow(`SELECT future_a,future_b FROM photo_faces WHERE id=?`, face.ID).Scan(&a, &b); err != nil || a != "alpha" || b != "saved" {
		t.Fatal(a, b, err)
	}
	if got, err := l.FaceThumbnail(ctx, face.ID); err != nil || !bytes.Equal(got, preview) {
		t.Fatal("migration changed preview", err)
	}
	// Recreated privacy triggers and explicit retention inserts use column names.
	marker := filepath.Join(root, "album/.adminonly")
	if err = os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if err = os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if err = os.Rename(filepath.Join(root, "album"), offline); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if err = os.Rename(offline, filepath.Join(root, "album")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if err = l.index.db.QueryRow(`SELECT future_a,future_b FROM photo_faces WHERE id=?`, face.ID).Scan(&a, &b); err != nil || a != "alpha" || b != "saved" {
		t.Fatal("column order corrupted data", a, b, err)
	}
}

func TestRetainedSchemaMigrationRollsBackOnIncompatibleShape(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	ctx := context.Background()
	if _, err := l.index.db.Exec(`ALTER TABLE media_index ADD COLUMN future TEXT DEFAULT 'new'; ALTER TABLE photo_retained_photo_faces ADD COLUMN removed_feature TEXT`); err != nil {
		t.Fatal(err)
	}
	if err := setupIdentityMaintenance(ctx, l.index.db); err == nil {
		t.Fatal("accepted implicit column removal")
	}
	cols, err := photoTableColumns(ctx, l.index.db, "photo_retained_media_index")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		if c.name == "future" {
			t.Fatal("partial migration committed")
		}
	}
	// Rollback leaves the connection and retained rows usable for an explicit migration.
	if err = l.index.db.QueryRow(`SELECT id FROM photo_entities WHERE path='a.jpg'`).Scan(new(int64)); err != nil {
		t.Fatal(err)
	}
}

func TestRetainedFacesFollowPersonMerge(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		for _, private := range []bool{false, true} {
			t.Run(fmt.Sprintf("automatic=%v/private=%v", automatic, private), func(t *testing.T) {
				ctx := context.Background()
				l := faceLibrary(t, "a-active.jpg", "b-hold/b.jpg", "z-target.jpg")
				finishFace(t, l, 0)
				finishFace(t, l, 0)
				finishFace(t, l, 1)
				faces, err := l.AutomaticFaces(ctx, "b-hold/b.jpg")
				if err != nil || len(faces) != 1 {
					t.Fatal(faces, err)
				}
				face := faces[0]
				targets, err := l.AutomaticFaces(ctx, "z-target.jpg")
				if err != nil || len(targets) != 1 {
					t.Fatal(targets, err)
				}
				target := targets[0].PersonID
				if _, err = l.SetFaceFavorite(ctx, face.ID, face.PersonID, true); err != nil {
					t.Fatal(err)
				}
				before, err := l.FaceThumbnail(ctx, face.ID)
				if err != nil {
					t.Fatal(err)
				}
				var embedding []byte
				if err = l.index.db.QueryRow(`SELECT embedding FROM photo_faces WHERE id=?`, face.ID).Scan(&embedding); err != nil {
					t.Fatal(err)
				}
				offline := filepath.Join(t.TempDir(), "offline")
				marker := filepath.Join(l.root, "b-hold", ".adminonly")
				if private {
					if _, err = l.index.db.Exec(`UPDATE photo_people SET name='Imported',name_fold='imported',name_source='b-hold/b.jpg',manual_name=0 WHERE id=?`, face.PersonID); err != nil {
						t.Fatal(err)
					}
					err = os.WriteFile(marker, nil, 0600)
				} else {
					err = os.Rename(filepath.Join(l.root, "b-hold"), offline)
				}
				if err != nil {
					t.Fatal(err)
				}
				rebuildIdentity(t, l)
				var retainedID int64
				if err = l.index.db.QueryRow(`SELECT person_id FROM photo_retained_photo_faces WHERE id=?`, face.ID).Scan(&retainedID); err != nil || retainedID != face.PersonID {
					t.Fatal(retainedID, err)
				}
				merge := func() error {
					if !automatic {
						return l.MergePeople(ctx, face.PersonID, target)
					}
					tx, err := l.index.db.BeginTx(ctx, nil)
					if err != nil {
						return err
					}
					defer tx.Rollback()
					if _, err = mergeAutomaticFaceGroupTx(ctx, tx, face.PersonID, target); err != nil {
						return err
					}
					return tx.Commit()
				}
				// Fail after both live and retained updates: the complete merge must roll back.
				if _, err = l.index.db.Exec(`CREATE TRIGGER reject_retained_merge BEFORE DELETE ON photo_people BEGIN SELECT RAISE(ABORT,'injected merge failure'); END`); err != nil {
					t.Fatal(err)
				}
				if err = merge(); err == nil {
					t.Fatal("expected rollback")
				}
				if err = l.index.db.QueryRow(`SELECT person_id FROM photo_retained_photo_faces WHERE id=?`, face.ID).Scan(&retainedID); err != nil || retainedID != face.PersonID {
					t.Fatal("partial merge", retainedID, err)
				}
				if _, err = l.index.db.Exec(`DROP TRIGGER reject_retained_merge`); err != nil {
					t.Fatal(err)
				}
				if err = merge(); err != nil {
					t.Fatal(err)
				}
				if private {
					var name string
					if err = l.index.db.QueryRow(`SELECT name FROM photo_people WHERE id=?`, target).Scan(&name); err != nil || name != "" {
						t.Fatal("hidden name published", name, err)
					}
				}
				// Reopen before the source returns: the association must be durable.
				root, cache, db := l.root, l.cacheDir, l.DBPath()
				if err = l.Close(); err != nil {
					t.Fatal(err)
				}
				l, err = New(root, cache, db, 60)
				if err != nil {
					t.Fatal(err)
				}
				defer l.Close()
				if private {
					err = os.Remove(marker)
				} else {
					err = os.Rename(offline, filepath.Join(root, "b-hold"))
				}
				if err != nil {
					t.Fatal(err)
				}
				rebuildIdentity(t, l)
				got, err := l.Face(ctx, face.ID)
				if err != nil || got.PersonID != target || !got.Favorite || (!automatic && !got.Manual) {
					t.Fatal(got, err)
				}
				if private && got.Name != "Imported" {
					t.Fatal("hidden provenance lost", got)
				}
				var current []byte
				if err = l.index.db.QueryRow(`SELECT embedding FROM photo_faces WHERE id=?`, face.ID).Scan(&current); err != nil || !bytes.Equal(current, embedding) {
					t.Fatal("embedding changed", err)
				}
				if current, err = l.FaceThumbnail(ctx, face.ID); err != nil || !bytes.Equal(current, before) {
					t.Fatal("preview changed", err)
				}
			})
		}
	}
}

func TestPhotoRelocationLeavesUnrelatedIndexesUntouched(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "from/album/a.jpg", "stable/z.jpg")
	// Unique bytes make the singleton relocation unambiguous.
	writeJPEG(t, filepath.Join(l.root, "stable/z.jpg"), color.Black)
	rebuildIdentity(t, l)
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	if err := os.MkdirAll(filepath.Join(l.root, "to"), 0750); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	if err := l.rebuildFaceReferences(ctx); err != nil {
		t.Fatal(err)
	}
	// SQL guards catch even delete/reinsert operations with identical final values.
	for _, spec := range [][2]string{{"photo_folder_scan", "path"}, {"folder_preview_index", "folder_path"}, {"photo_face_directories", "directory"}, {"photo_face_job_directories", "directory"}} {
		for _, op := range []string{"DELETE", "UPDATE"} {
			stmt := fmt.Sprintf(`CREATE TRIGGER guard_%s_%s BEFORE %s ON %s WHEN old.%s='stable' BEGIN SELECT RAISE(ABORT,'unrelated index touched'); END`, spec[0], op, op, spec[0], spec[1])
			if _, err := l.index.db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := l.index.db.Exec(`CREATE TRIGGER guard_unrelated_references BEFORE DELETE ON photo_face_references WHEN old.face_id IN(SELECT id FROM photo_faces WHERE path='stable/z.jpg') BEGIN SELECT RAISE(ABORT,'unrelated references touched'); END`); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(l.root, "from/album"), filepath.Join(l.root, "to/album")); err != nil {
		t.Fatal(err)
	}
	rebuildIdentity(t, l)
	var pending int
	if err := l.index.db.QueryRow(`SELECT pending FROM photo_face_reference_settings`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("global reference rebuild scheduled", pending, err)
	}
	for _, spec := range [][3]string{{"photo_faces", "photo_face_directories", "face_count"}, {"photo_face_jobs", "photo_face_job_directories", "job_count"}} {
		var mismatch int
		query := fmt.Sprintf(`SELECT count(*) FROM (SELECT directory,count(*) n FROM %s GROUP BY directory) a LEFT JOIN %s c USING(directory) WHERE c.%s IS NULL OR a.n<>c.%s`, spec[0], spec[1], spec[2], spec[2])
		if err := l.index.db.QueryRow(query).Scan(&mismatch); err != nil || mismatch != 0 {
			t.Fatal(spec, mismatch, err)
		}
		if err := l.index.db.QueryRow(`SELECT count(*) FROM ` + spec[1] + ` WHERE directory='from/album'`).Scan(&mismatch); err != nil || mismatch != 0 {
			t.Fatal("old counter remains", spec, mismatch, err)
		}
	}
	var from, to int
	if err := l.index.db.QueryRow(`SELECT recursive_media_count FROM folder_index WHERE path='from'`).Scan(&from); err != nil {
		t.Fatal(err)
	}
	if err := l.index.db.QueryRow(`SELECT recursive_media_count FROM folder_index WHERE path='to'`).Scan(&to); err != nil || from != 0 || to != 1 {
		t.Fatal(from, to, err)
	}
}

func TestPhotoRelocationRollbackAndRestartCheckpoint(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "old/a.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "old/a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatal(faces, err)
	}
	face := faces[0]
	before := identityAt(t, l, "old/a.jpg")
	preview, err := l.FaceThumbnail(ctx, face.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(l.root, "old"), filepath.Join(l.root, "new")); err != nil {
		t.Fatal(err)
	}
	if complete, err := l.scanPhotoIdentities(ctx); err != nil || !complete {
		t.Fatal(complete, err)
	}
	if _, err = l.index.db.Exec(`CREATE TRIGGER reject_relocation_commit BEFORE INSERT ON photo_relocations BEGIN SELECT RAISE(ABORT,'injected late failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = l.recognizePhotoRelocations(ctx); err == nil {
		t.Fatal("expected relocation rollback")
	}
	if e := identityAt(t, l, "old/a.jpg"); e.ID != before.ID {
		t.Fatal(e)
	}
	var path string
	if err = l.index.db.QueryRow(`SELECT path FROM photo_faces WHERE id=?`, face.ID).Scan(&path); err != nil || path != "old/a.jpg" {
		t.Fatal("partial face move", path, err)
	}
	if _, err = l.index.db.Exec(`DROP TRIGGER reject_relocation_commit`); err != nil {
		t.Fatal(err)
	}
	if err = l.recognizePhotoRelocations(ctx); err != nil {
		t.Fatal(err)
	}
	// Stop after the committed identity transaction, before publishing metadata
	// and recursive counts. Persistent scoped invalidations must resume on open.
	root, cache, db := l.root, l.cacheDir, l.DBPath()
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	l, err = New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	rebuildIdentity(t, l)
	if e := identityAt(t, l, "new/a.jpg"); e.ID != before.ID {
		t.Fatal(e)
	}
	got, err := l.Face(ctx, face.ID)
	if err != nil || got.Path != "new/a.jpg" || got.PersonID != face.PersonID {
		t.Fatal(got, err)
	}
	if after, err := l.FaceThumbnail(ctx, face.ID); err != nil || !bytes.Equal(after, preview) {
		t.Fatal("checkpoint changed preview", err)
	}
}

// Measures verified relocation publication for 100 entries inside a 10k-file
// library. The filesystem inventory/hash pass is deliberately outside the timer.
func BenchmarkPhotoRelocation100Of10000(b *testing.B) {
	l := newBenchmarkFilesystemLibrary(b, 100, 10000)
	defer l.Close()
	ctx := context.Background()
	if _, err := l.RebuildIndex(ctx); err != nil {
		b.Fatal(err)
	}
	old, next := benchmarkGalleryName(0), "relocated"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		source, err := scanEntity(l.index.db.QueryRow(`SELECT `+entityColumns+` FROM photo_entities WHERE kind='folder' AND path=?`, old))
		if err != nil {
			b.Fatal(err)
		}
		if err = os.Rename(filepath.Join(l.root, old), filepath.Join(l.root, next)); err != nil {
			b.Fatal(err)
		}
		if complete, err := l.scanPhotoIdentities(ctx); err != nil || !complete {
			b.Fatal(complete, err)
		}
		b.StartTimer()
		if err = l.relocatePhotoFolder(ctx, source, next, false); err != nil {
			b.Fatal(err)
		}
		old, next = next, old
	}
}
