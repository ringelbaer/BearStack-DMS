package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/facerec"
)

func TestPrepareFacePhotoOnlyQueuesRequestedImage(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	job, err := l.PrepareFacePhoto(ctx, "b.jpg", facerec.Model)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	var enabled bool
	if err = l.index.db.QueryRow(`SELECT count(*) FROM photo_face_jobs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("jobs %d %v", count, err)
	}
	if err = l.index.db.QueryRow(`SELECT enabled FROM photo_face_state WHERE id=1`).Scan(&enabled); err != nil || enabled {
		t.Fatalf("global processing changed: %v %v", enabled, err)
	}
	if err = l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(0)}}); err != nil {
		t.Fatal(err)
	}
	faces, _ := l.AutomaticFaces(ctx, "b.jpg")
	if err = l.RenamePerson(ctx, faces[0].PersonID, "Preserved"); err != nil {
		t.Fatal(err)
	}
	job, err = l.PrepareFacePhoto(ctx, "b.jpg", facerec.Model)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(1)}}); err != nil {
		t.Fatal(err)
	}
	faces, _ = l.AutomaticFaces(ctx, "b.jpg")
	if len(faces) != 1 || faces[0].Name != "Preserved" {
		t.Fatalf("override lost: %+v", faces)
	}
	for _, path := range []string{"../a.jpg", "/a.jpg", ""} {
		if _, err = l.PrepareFacePhoto(ctx, path, facerec.Model); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("unsafe path: %q %v", path, err)
		}
		if _, err = l.PhotoFaces(ctx, path); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("unsafe read path: %q %v", path, err)
		}
	}
	if err = os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = l.PrepareFacePhoto(ctx, "a.jpg", facerec.Model); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("private source: %v", err)
	}
	if _, err = l.PhotoFaces(ctx, "b.jpg"); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("private detections: %v", err)
	}
}
