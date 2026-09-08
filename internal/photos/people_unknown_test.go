package photos

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUnknownPeopleExcludeNamedAndIgnoredFaces(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg", "d.jpg")
	for _, axis := range []int{0, 1, 2, 0} {
		finishFace(t, l, axis)
	}
	var faces []RecognizedFace
	for _, path := range []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"} {
		found, err := l.AutomaticFaces(ctx, path)
		if err != nil || len(found) != 1 {
			t.Fatalf("%s: %+v %v", path, found, err)
		}
		faces = append(faces, found[0])
	}
	if err := l.RenamePerson(ctx, faces[1].PersonID, "Known"); err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{faces[0].ID, faces[2].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	for _, known := range []bool{false, true} {
		page, err := l.People(ctx, 0, 1, "", known, true)
		if err != nil || !page.UnknownOnly || page.KnownOnly || page.IgnoredOnly || len(page.People) != 1 {
			t.Fatalf("unknown filter: %+v %v", page, err)
		}
		person := page.People[0]
		if person.Name != "" || person.ID != faces[3].PersonID || person.FaceID != faces[3].ID || person.Count != 1 {
			t.Fatalf("named or ignored portrait/photo counted: %+v", person)
		}
	}
	if page, err := l.People(ctx, 0, 1, "Known", false, true); err != nil || len(page.People) != 0 {
		t.Fatalf("name query escaped unknown filter: %+v %v", page, err)
	}
	if err := l.RenamePerson(ctx, faces[3].PersonID, "Newly named"); err != nil {
		t.Fatal(err)
	}
	if page, err := l.People(ctx, 0, 1, "", false, true); err != nil || len(page.People) != 0 || page.TotalPages != 1 {
		t.Fatalf("named person remained unknown: %+v %v", page, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.People(cancelled, 0, 1, "", false, true); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestUnknownPeoplePaginationClampsAfterNaming(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<62)
 INSERT INTO photo_people(name,name_fold) SELECT '', '' FROM n;
 INSERT INTO photo_faces(path,person_id,x,y,width,height,confidence,embedding,model)
 SELECT f.path,p.id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model
 FROM photo_people p CROSS JOIN photo_faces f WHERE f.id=1 AND p.id<>f.person_id;
 UPDATE photo_people SET name='Known',name_fold='known',manual_name=1 WHERE id=63;
 UPDATE photo_faces SET ignored=1 WHERE person_id=62;`); err != nil {
		t.Fatal(err)
	}
	first, err := l.People(ctx, 0, 1, "", false, true)
	if err != nil || len(first.People) != 60 || first.TotalPages != 2 || !first.HasNext || first.HasPrev {
		t.Fatalf("first: %+v %v", first, err)
	}
	last, err := l.People(ctx, 0, 999, "", false, true)
	if err != nil || len(last.People) != 1 || last.Page != 2 || last.HasNext || !last.HasPrev || last.People[0].ID != 61 {
		t.Fatalf("last: %+v %v", last, err)
	}
	if err := l.RenamePerson(ctx, last.People[0].ID, "Done"); err != nil {
		t.Fatal(err)
	}
	page, err := l.People(ctx, 0, 2, "", false, true)
	if err != nil || page.Page != 1 || page.TotalPages != 1 || len(page.People) != 60 || page.HasPrev || page.HasNext {
		t.Fatalf("clamped: %+v %v", page, err)
	}
}

func TestUnknownPeopleRefreshImportedNamesAndPrivacy(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "one/a.jpg", "other/b.jpg", "source/z.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	if _, err := l.index.db.Exec(`UPDATE photo_people SET name='Imported',name_fold='imported',manual_name=0,name_source='source/z.jpg' WHERE id=1;
 UPDATE photo_people SET name='Manual',name_fold='manual',manual_name=1 WHERE id=2;`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "other/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if page, err := l.People(ctx, 0, 1, "", false, true); err != nil || len(page.People) != 0 {
		t.Fatalf("imported person shown before source protection: %+v %v", page, err)
	}
	var private bool
	if err := l.index.db.QueryRow(`SELECT admin_only FROM media_index WHERE path='other/b.jpg'`).Scan(&private); err != nil || private {
		t.Fatalf("checked unrelated manually named group: %v %v", private, err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "source/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if page, err := l.People(ctx, 0, 1, "", false, true); err != nil || len(page.People) != 1 || page.People[0].Name != "" || page.People[0].ID != 1 {
		t.Fatalf("hidden imported name did not become unknown: %+v %v", page, err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "one/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if page, err := l.People(ctx, 0, 1, "", false, true); err != nil || len(page.People) != 0 {
		t.Fatalf("private unknown person exposed: %+v %v", page, err)
	}
}
