package photos

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPeoplePortraitFollowsVisibleActiveThumbnail(t *testing.T) {
	l := faceLibrary(t, "first/a.jpg", "second/b.jpg", "third/c.jpg")
	for range 3 {
		finishFace(t, l, 0)
	}
	ctx := context.Background()
	check := func(directory string, count int) Person {
		t.Helper()
		page, err := l.People(ctx, 0, 1, "", false, false)
		if err != nil || len(page.People) != 1 {
			t.Fatalf("people: %+v %v", page, err)
		}
		person := page.People[0]
		if person.Directory != directory || person.Count != count || person.Portrait == nil {
			t.Fatalf("portrait metadata: %+v", person)
		}
		face, err := l.Face(ctx, person.FaceID)
		if err != nil || *person.Portrait != (FaceRegion{face.X, face.Y, face.Width, face.Height}) {
			t.Fatalf("portrait mismatch: %+v %+v %v", person.Portrait, face, err)
		}
		return person
	}
	first := check("first", 3)
	if err := l.EditFaces(ctx, []int64{first.FaceID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	check("second", 2)
	if err := os.WriteFile(filepath.Join(l.Root(), "second/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	check("third", 1)
}
