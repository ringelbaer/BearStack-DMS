package photos

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestPeopleMatchesKnownOverviewWithoutPagination(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<61)
 INSERT INTO photo_people(name,name_fold) SELECT printf('Person %02d',x),printf('person %02d',x) FROM n;
 INSERT INTO photo_faces(path,person_id,x,y,width,height,confidence,embedding,model)
 SELECT f.path,p.id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_people p
 CROSS JOIN photo_faces f WHERE f.id=1 AND p.id<>f.person_id;
 UPDATE photo_people SET name='Jürgen_100%',name_fold='juergen_100%' WHERE id=2;
 INSERT INTO photo_people(name,name_fold) VALUES('Empty','empty');`); err != nil {
		t.Fatal(err)
	}
	// Two detections of one person in the same photo count as one photo.
	if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,person_id,x,y,width,height,confidence,embedding,model)
 SELECT path,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE person_id=2 LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"", "Person", "Person 61", "Juergen", "Jürgen", "%", "_", "absent", "Empty"} {
		page, err := l.People(ctx, 0, 1, q, true)
		if err != nil {
			t.Fatal(err)
		}
		got, err := l.SuggestPeople(ctx, q)
		if err != nil || got.HasNext != page.HasNext || len(got.People) != len(page.People) {
			t.Fatalf("%q: %+v vs %+v, %v", q, got, page, err)
		}
		for i, person := range got.People {
			want := page.People[i]
			if person.ID != want.ID || person.Name != want.Name || person.Count != want.Count || person.Count != 1 || person.FaceID != want.FaceID || person.FaceID <= 0 {
				t.Fatalf("%q suggestion %+v vs %+v", q, person, want)
			}
		}
	}
	before, err := l.SuggestPeople(ctx, "Juergen")
	if err != nil || len(before.People) != 1 {
		t.Fatalf("portrait before ignoring: %+v %v", before, err)
	}
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET ignored=1 WHERE id=?`, before.People[0].FaceID); err != nil {
		t.Fatal(err)
	}
	after, err := l.SuggestPeople(ctx, "Juergen")
	if err != nil || len(after.People) != 1 || after.People[0].FaceID <= before.People[0].FaceID || after.People[0].Count != 1 {
		t.Fatalf("ignored portrait was not replaced: %+v %v", after, err)
	}
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET ignored=1 WHERE person_id=2`); err != nil {
		t.Fatal(err)
	}
	if got, err := l.SuggestPeople(ctx, "Juergen"); err != nil || len(got.People) != 0 {
		t.Fatalf("ignored person suggested: %+v %v", got, err)
	}
}

func TestSuggestPeopleChecksImportedNameSourceAndCancellation(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "one/a.jpg", "source/z.jpg")
	finishFace(t, l, 0)
	if _, err := l.index.db.Exec(`UPDATE photo_people SET name='Imported',name_fold='imported',manual_name=0,name_source='source/z.jpg'`); err != nil {
		t.Fatal(err)
	}
	if got, err := l.SuggestPeople(ctx, "Imported"); err != nil || len(got.People) != 1 {
		t.Fatalf("public imported name: %+v %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "source/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := l.SuggestPeople(ctx, "Imported"); err != nil || len(got.People) != 0 {
		t.Fatalf("private imported name disclosed: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.SuggestPeople(ctx, ""); err == nil {
		t.Fatal("cancellation ignored")
	}
}
