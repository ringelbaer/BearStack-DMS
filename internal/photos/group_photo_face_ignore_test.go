package photos

import (
	"context"
	"errors"
	"testing"
)

func TestGroupPhotoFaceIgnoreIsLimitedToOneDetection(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	_, a := finishGroupPhoto(t, l, 6, 0)
	_, b := finishGroupPhoto(t, l, 6, 0)
	// The same person occurs twice in this photo and once in another photo.
	if err := l.EditFaces(ctx, []int64{a.Faces[1].ID}, a.Faces[0].PersonID, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetFaceFavorite(ctx, a.Faces[0].ID, a.Faces[0].PersonID, true); err != nil {
		t.Fatal(err)
	}
	a, err := l.GroupPhoto(ctx, a.Path)
	if err != nil {
		t.Fatal(err)
	}
	count, err := l.IgnoreGroupPhotoFace(ctx, a.Path, a.Revision, a.Faces[0].ID)
	if err != nil || count != 1 {
		t.Fatalf("single ignore: %d %v", count, err)
	}
	current, err := l.GroupPhoto(ctx, a.Path)
	if err != nil || current.Remaining != 5 || len(current.Faces) != 6 || current.Revision == a.Revision {
		t.Fatalf("current photo: %+v %v", current, err)
	}
	for i, face := range current.Faces {
		if face.Ignored != (i == 0) {
			t.Fatalf("wrong ignored detection: %+v", face)
		}
	}
	if !current.Faces[0].Favorite || current.Faces[1].PersonID != current.Faces[0].PersonID {
		t.Fatal("favorite or group assignment changed")
	}
	other, err := l.AutomaticFaces(ctx, b.Path)
	if err != nil || len(other) != 6 || other[0].PersonID != current.Faces[0].PersonID {
		t.Fatalf("other photo changed: %+v %v", other, err)
	}
	for _, id := range selectedReferences(t, l) {
		if id == a.Faces[0].ID {
			t.Fatal("ignored favorite remains a recognition reference")
		}
	}
	for _, revision := range []string{a.Revision, current.Revision} {
		if _, err := l.IgnoreGroupPhotoFace(ctx, a.Path, revision, a.Faces[0].ID); !errors.Is(err, ErrGroupPhotoChanged) {
			t.Fatalf("repeat should conflict: %v", err)
		}
	}
	for _, id := range []int64{0, -1, b.Faces[0].ID} {
		if _, err := l.IgnoreGroupPhotoFace(ctx, a.Path, current.Revision, id); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("invalid or unrelated ID %d: %v", id, err)
		}
	}
	if err := l.RenamePerson(ctx, a.Faces[2].PersonID, "Named"); err != nil {
		t.Fatal(err)
	}
	current, err = l.GroupPhoto(ctx, a.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.IgnoreGroupPhotoFace(ctx, a.Path, current.Revision, a.Faces[2].ID); !errors.Is(err, ErrGroupPhotoChanged) {
		t.Fatalf("named detection should be protected: %v", err)
	}
}
