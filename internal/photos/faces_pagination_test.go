package photos

import (
	"context"
	"testing"
)

func TestPeoplePaginationCountsGroupsAndFaces(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces: %v %v", faces, err)
	}
	face := faces[0]
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<61) INSERT INTO photo_people(name,name_fold) SELECT printf('Person %d',x),printf('person %d',x) FROM n`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,person_id,x,y,width,height,confidence,embedding,model) SELECT f.path,p.id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_people p CROSS JOIN photo_faces f WHERE f.id=? AND p.id<>?`, face.ID, face.PersonID); err != nil {
		t.Fatal(err)
	}
	page, err := l.People(ctx, 0, 999, "Person", true)
	if err != nil || page.Page != 2 || page.TotalPages != 2 || len(page.People) != 1 || page.HasNext || !page.HasPrev {
		t.Fatalf("group last: %+v %v", page, err)
	}
	filtered, err := l.People(ctx, 0, 999, "Person 61", true)
	if err != nil || filtered.Page != 1 || filtered.TotalPages != 1 || len(filtered.People) != 1 || filtered.HasPrev {
		t.Fatalf("filtered: %+v %v", filtered, err)
	}
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET person_id=?`, face.PersonID); err != nil {
		t.Fatal(err)
	}
	detail, err := l.People(ctx, face.PersonID, 999, "", false)
	if err != nil || detail.Page != 2 || detail.TotalPages != 2 || len(detail.Faces) != 2 {
		t.Fatalf("detail last: %+v %v", detail, err)
	}
	overview, err := l.People(ctx, 0, 999, "", false)
	if err != nil || overview.Page != 1 || overview.TotalPages != 1 || len(overview.People) != 1 {
		t.Fatalf("empty groups counted: %+v %v", overview, err)
	}
}
