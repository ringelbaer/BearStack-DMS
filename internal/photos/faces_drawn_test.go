package photos

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bearstack/internal/facerec"
)

func TestDrawnFacePersistsAndAssignsWithoutInference(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	_, revision, err := l.FaceDrawingImage(ctx, "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	box := FaceRegion{X: .2, Y: .2, Width: .3, Height: .3}
	id, err := l.AddDrawnFace(ctx, "a.jpg", revision, box, 0, "Manuell")
	if err != nil {
		t.Fatal(err)
	}
	f, err := l.Face(ctx, id)
	if err != nil || !f.Drawn || !f.Manual || f.Name != "Manuell" {
		t.Fatalf("face: %+v %v", f, err)
	}
	if _, err = l.FaceThumbnail(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = l.SetFaceFavorite(ctx, id, f.PersonID, true); err != nil {
		t.Fatal(err)
	}
	assertRecognitionCounts(t, l, 0, 0, 0)
	if _, err = l.AddDrawnFace(ctx, "a.jpg", revision, box, 0, "Duplikat"); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	_, otherRevision, err := l.FaceDrawingImage(ctx, "b.jpg")
	if err != nil {
		t.Fatal(err)
	}
	other, err := l.AddDrawnFace(ctx, "b.jpg", otherRevision, box, f.PersonID, "")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := l.Face(ctx, other)
	if err != nil || f2.PersonID != f.PersonID {
		t.Fatalf("assignment: %+v %v", f2, err)
	}
	var initialReferences int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references`).Scan(&initialReferences); err != nil || initialReferences != 0 {
		t.Fatalf("manual favorite is not a vector reference: %d %v", initialReferences, err)
	}
	for _, detections := range [][]facerec.Detection{nil, {faceDetection(0)}} {
		job, err := l.PrepareFacePhoto(ctx, "a.jpg", facerec.Model)
		if err != nil {
			t.Fatal(err)
		}
		if err = l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: detections}); err != nil {
			t.Fatal(err)
		}
		photo, err := l.PhotoFaces(ctx, "a.jpg")
		if err != nil || len(photo.Faces) != 1 || photo.Faces[0].ID != id || photo.Faces[0].Name != "Manuell" {
			t.Fatalf("reanalysis: %+v %v", photo, err)
		}
	}
	if suggestions, err := l.SuggestPeopleForFace(ctx, id); err != nil || len(suggestions.People) != 0 {
		t.Fatalf("manual vector: %+v %v", suggestions, err)
	}
	var references int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references WHERE face_id IN (?,?)`, id, other).Scan(&references); err != nil || references != 0 {
		t.Fatalf("fabricated references: %d %v", references, err)
	}
	// Other regions are still recognized and included in the automatic statistics.
	detection := faceDetection(1)
	detection.X = .65
	job, err := l.PrepareFacePhoto(ctx, "a.jpg", facerec.Model)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{detection}}); err != nil {
		t.Fatal(err)
	}
	assertRecognitionCounts(t, l, 0, 1, 0)
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	resumed, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if f, err = resumed.Face(ctx, id); err != nil || !f.Drawn || f.Name != "Manuell" {
		t.Fatalf("restart: %+v %v", f, err)
	}
}

func TestDrawnFaceRejectsInvalidAndChangedSource(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	_, revision, err := l.FaceDrawingImage(ctx, "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	for _, box := range []FaceRegion{{X: math.NaN(), Width: .2, Height: .2}, {X: -.1, Width: .2, Height: .2}, {X: .9, Width: .2, Height: .2}, {Width: 0, Height: .2}} {
		if _, err = l.AddDrawnFace(ctx, "a.jpg", revision, box, 0, "Test"); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("invalid %+v: %v", box, err)
		}
	}
	box := FaceRegion{X: .2, Y: .2, Width: .3, Height: .3}
	if _, err = l.AddDrawnFace(ctx, "a.jpg", revision, box, 123, ""); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("missing target: %v", err)
	}
	if _, err = l.AddDrawnFace(ctx, "a.jpg", revision, box, 0, ""); !errors.Is(err, ErrLabelInvalid) {
		t.Fatalf("empty name: %v", err)
	}
	if _, err = l.AddDrawnFace(ctx, "../a.jpg", revision, box, 0, "Test"); !errors.Is(err, ErrLabelInvalid) {
		t.Fatalf("traversal: %v", err)
	}
	future := time.Now().Add(time.Hour)
	if err = os.Chtimes(filepath.Join(l.Root(), "a.jpg"), future, future); err != nil {
		t.Fatal(err)
	}
	if _, err = l.AddDrawnFace(ctx, "a.jpg", revision, box, 0, "Test"); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("stale source: %v", err)
	}
	if err = os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = l.FaceDrawingImage(ctx, "a.jpg"); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("private preview: %v", err)
	}
	if _, err = l.AddDrawnFace(ctx, "a.jpg", revision, box, 0, "Test"); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("private write: %v", err)
	}
}

func TestDrawnFaceMigration(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	if _, err := l.index.db.Exec(`DROP INDEX idx_face_drawn_active; ALTER TABLE photo_faces DROP COLUMN drawn; UPDATE schema_migrations SET version=28 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	resumed, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	photo, err := resumed.PhotoFaces(context.Background(), "a.jpg")
	if err != nil || len(photo.Faces) != 1 || photo.Faces[0].Drawn {
		t.Fatalf("migration: %+v %v", photo, err)
	}
}
