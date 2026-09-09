package photos

import (
	"context"
	"database/sql"
	"testing"

	"bearstack/internal/facerec"
)

func TestFaceQualityPersistenceAndReferenceSelection(t *testing.T) {
	for _, kind := range []string{"legacy", "eligible", "weak"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a.jpg")
			if err := l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
				t.Fatal(err)
			}
			job, err := l.NextFaceJob(ctx)
			if err != nil {
				t.Fatal(err)
			}
			d := faceDetection(0)
			if kind != "legacy" {
				d.Quality = &facerec.Quality{FacePixels: 64, Sharpness: 40, ReferenceEligible: kind == "eligible"}
			}
			if err := l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{d}}); err != nil {
				t.Fatal(err)
			}
			var eligible sql.NullBool
			var pixels, sharpness sql.NullFloat64
			if err := l.index.db.QueryRow(`SELECT reference_eligible,face_pixels,sharpness FROM photo_faces`).Scan(&eligible, &pixels, &sharpness); err != nil {
				t.Fatal(err)
			}
			if eligible.Valid != (kind != "legacy") || pixels.Valid != eligible.Valid || sharpness.Valid != eligible.Valid {
				t.Fatalf("quality fields: %v %v %v", eligible, pixels, sharpness)
			}
			if pixels.Valid && (pixels.Float64 != 64 || sharpness.Float64 != 40 || eligible.Bool != (kind == "eligible")) {
				t.Fatal("quality not persisted")
			}
			want := 1
			if kind == "weak" {
				want = 0
			}
			if got := faceReferenceCount(t, l); got != want {
				t.Fatalf("references %d want %d", got, want)
			}
			if kind == "weak" {
				fs, err := l.AutomaticFaces(ctx, "a.jpg")
				if err != nil || len(fs) != 1 {
					t.Fatalf("weak detection lost: %v %v", fs, err)
				}
				if _, err = l.SetFaceFavorite(ctx, fs[0].ID, fs[0].PersonID, true); err != nil {
					t.Fatal(err)
				}
				if faceReferenceCount(t, l) != 1 {
					t.Fatal("explicit favorite must override quality")
				}
				if _, err = l.SetFaceFavorite(ctx, fs[0].ID, fs[0].PersonID, false); err != nil {
					t.Fatal(err)
				}
				if faceReferenceCount(t, l) != 0 {
					t.Fatal("removed quality override still referenced")
				}
			}
		})
	}
}

func TestFaceQualityMigrationPreservesLegacyGroups(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	fs, _ := l.AutomaticFaces(ctx, "a.jpg")
	if err := l.RenamePerson(ctx, fs[0].PersonID, "Ada"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`ALTER TABLE photo_faces DROP COLUMN reference_eligible; ALTER TABLE photo_faces DROP COLUMN face_pixels; ALTER TABLE photo_faces DROP COLUMN sharpness; UPDATE schema_migrations SET version=24 WHERE component='photos'`); err != nil {
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
	faces, err := reopened.AutomaticFaces(ctx, "a.jpg")
	if err != nil || len(faces) != 1 || faces[0].PersonID != fs[0].PersonID || faces[0].Name != "Ada" {
		t.Fatalf("migration changed groups: %+v %v", faces, err)
	}
	prepareReferenceGraph(t, reopened)
	if faceReferenceCount(t, reopened) != 1 {
		t.Fatal("legacy reference lost")
	}
}
