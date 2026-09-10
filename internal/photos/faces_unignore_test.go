package photos

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestUnignorePhotoFacesPreservesAssignmentsAndOtherPhotos(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	_, a := finishGroupPhoto(t, l, 3, 0)
	_, b := finishGroupPhoto(t, l, 3, 0)
	if err := l.RenamePerson(ctx, a.Faces[0].PersonID, "Anna"); err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{a.Faces[0].ID, a.Faces[1].ID, b.Faces[0].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	before, err := l.PhotoFaces(ctx, a.Path)
	if err != nil {
		t.Fatal(err)
	}
	count, err := l.UnignorePhotoFaces(ctx, a.Path, before.Revision)
	if err != nil || count != 2 {
		t.Fatalf("restore: %d %v", count, err)
	}
	after, _ := l.PhotoFaces(ctx, a.Path)
	for i, face := range before.Faces {
		face.Ignored = false
		if !reflect.DeepEqual(face, after.Faces[i]) {
			t.Fatalf("metadata changed: before=%+v after=%+v", face, after.Faces[i])
		}
	}
	sibling, _ := l.Face(ctx, b.Faces[0].ID)
	if !sibling.Ignored || sibling.Name != "Anna" || sibling.PersonID != a.Faces[0].PersonID {
		t.Fatalf("sibling changed: %+v", sibling)
	}
	if _, err := l.UnignorePhotoFaces(ctx, a.Path, before.Revision); !errors.Is(err, ErrGroupPhotoChanged) {
		t.Fatalf("stale snapshot: %v", err)
	}
	if n, err := l.UnignorePhotoFaces(ctx, a.Path, after.Revision); err != nil || n != 0 {
		t.Fatalf("no-op: %d %v", n, err)
	}
	// A later ignore must never be included in an older decision.
	if err := l.EditFaces(ctx, []int64{a.Faces[2].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.UnignorePhotoFaces(ctx, a.Path, after.Revision); !errors.Is(err, ErrGroupPhotoChanged) {
		t.Fatalf("new ignore accepted: %v", err)
	}
	face, _ := l.Face(ctx, a.Faces[2].ID)
	if !face.Ignored {
		t.Fatal("stale request restored a new ignore")
	}
}
