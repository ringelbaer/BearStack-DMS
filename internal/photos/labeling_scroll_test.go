package photos

import (
	"context"
	"testing"
)

func TestNamedPeopleSearchFoldingLiteralWildcardsAndCursor(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	a := labelAction(p, s, "detach")
	a.FaceID = p.FaceID
	r, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.RenamePerson(ctx, p.ID, "Müller_100%"); err != nil {
		t.Fatal(err)
	}
	if err = l.RenamePerson(ctx, r.NewID, "MüllerX100Y"); err != nil {
		t.Fatal(err)
	}
	s, _ = l.LabelSession(ctx)
	for _, test := range []struct {
		q     string
		after int64
		want  int
	}{
		{"MUELLER", 0, 2}, {"_100%", 0, 1}, {"mueller", p.ID, 1}, {"' OR 1=1 --", 0, 0}, {"fehlend", 0, 0},
	} {
		page, err := l.LabelNamedPeople(ctx, test.after, s.UpperID, test.q)
		if err != nil || len(page.People) != test.want {
			t.Fatalf("%q: %+v %v", test.q, page, err)
		}
	}
}

func TestLabelPersonCursorLargePagesAndMutationRevision(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	for range 85 {
		if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, p.FaceID); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.RenamePerson(ctx, p.ID, "Anna"); err != nil {
		t.Fatal(err)
	}
	var cursor int64
	seen := map[int64]bool{}
	for _, want := range []int{40, 40, 8} {
		page, err := l.LabelPersonAfter(ctx, p.ID, cursor, 40)
		if err != nil || len(page.Faces) != want || page.Count != 88 {
			t.Fatalf("page: %+v %v", page, err)
		}
		for _, face := range page.Faces {
			if face.ID <= cursor || seen[face.ID] {
				t.Fatal("duplicate/out-of-order face")
			}
			seen[face.ID] = true
		}
		cursor = page.Faces[len(page.Faces)-1].ID
		p = page
	}
	end, err := l.LabelPersonAfter(ctx, p.ID, cursor, 40)
	if err != nil || len(end.Faces) != 0 {
		t.Fatalf("end: %+v %v", end, err)
	}
	legacy, err := l.LabelPerson(ctx, p.ID, 0)
	if err != nil || len(legacy.Faces) != 4 {
		t.Fatalf("legacy: %+v %v", legacy, err)
	}
	a := labelAction(p, s, "unassign")
	a.FaceID = cursor
	r, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	after, err := l.LabelPerson(ctx, p.ID, 0)
	if err != nil || r.SourceRevision != after.Revision || r.SourceRevision <= p.Revision {
		t.Fatalf("receipt revision: %+v %+v %v", r, after, err)
	}
}
