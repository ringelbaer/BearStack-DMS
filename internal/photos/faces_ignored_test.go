package photos

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoredFacesFilterPaginationAndRestore(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "album/a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces: %v %v", faces, err)
	}
	face := faces[0]
	if err := l.RenamePerson(ctx, face.PersonID, "Jürgen"); err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{face.ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	// More than a page in one group: the ignored view paginates faces, not groups.
	_, err = l.index.db.ExecContext(ctx, `WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<62) INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,manual,ignored) SELECT f.path,f.directory,f.person_id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model,1,1 FROM photo_faces f CROSS JOIN n WHERE f.id=?`, face.ID)
	if err != nil {
		t.Fatal(err)
	}
	visible, err := l.People(ctx, 0, 1, "", false)
	if err != nil || len(visible.People) != 0 {
		t.Fatalf("ignored group visible: %+v %v", visible, err)
	}
	first, err := l.IgnoredFaces(ctx, 1, "Jürgen", true)
	if err != nil || len(first.Faces) != 60 || first.TotalPages != 2 || !first.HasNext || first.HasPrev || !first.IgnoredOnly {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := l.IgnoredFaces(ctx, 2, "Jürgen", true)
	if err != nil || len(second.Faces) != 3 || second.HasNext || !second.HasPrev || second.Faces[0].ID <= first.Faces[59].ID {
		t.Fatalf("second: %+v %v", second, err)
	}
	clamped, err := l.IgnoredFaces(ctx, 999, "Jürgen", true)
	if err != nil || clamped.Page != 2 || clamped.TotalPages != 2 || len(clamped.Faces) != 3 {
		t.Fatalf("clamped: %+v %v", clamped, err)
	}
	empty, err := l.IgnoredFaces(ctx, 1, "Other", false)
	if err != nil || len(empty.Faces) != 0 || empty.TotalPages != 1 || empty.Page != 1 {
		t.Fatalf("search: %+v %v", empty, err)
	}
	if err := l.EditFaces(ctx, []int64{face.ID}, 0, false, "Petra"); err != nil {
		t.Fatal(err)
	}
	restored, err := l.Face(ctx, face.ID)
	if err != nil || restored.Ignored || restored.Name != "Petra" || restored.PersonID == face.PersonID {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	second, err = l.IgnoredFaces(ctx, 2, "Jürgen", true)
	if err != nil || len(second.Faces) != 2 {
		t.Fatalf("other ignored faces changed: %+v %v", second, err)
	}
	var refs int
	if err := l.index.db.QueryRowContext(ctx, `SELECT count(*) FROM photo_face_references WHERE face_id=?`, face.ID).Scan(&refs); err != nil || refs != 1 {
		t.Fatalf("restored reference: %d %v", refs, err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "album/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	hidden, err := l.IgnoredFaces(ctx, 1, "", false)
	if err != nil || len(hidden.Faces) != 0 {
		t.Fatalf("private ignored faces leaked: %+v %v", hidden, err)
	}
}
