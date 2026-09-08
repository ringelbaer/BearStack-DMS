package photos

import (
	"context"
	"testing"
)

func TestLabelOriginalKeySharesSourceAndChangesWithIndexedFile(t *testing.T) {
	ctx := context.Background()
	l, p, _ := labelFixture(t)
	if len(p.Faces) != 3 {
		t.Fatal(p)
	}
	if p.Faces[0].OriginalKey == p.Faces[1].OriginalKey || len(p.Faces[0].OriginalKey) != 64 {
		t.Fatal("different originals share a key")
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,person_id,x+0.01,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, p.Faces[0].ID); err != nil {
		t.Fatal(err)
	}
	page, err := l.LabelPerson(ctx, p.ID, 0)
	if err != nil || len(page.Faces) != 4 {
		t.Fatalf("%+v %v", page, err)
	}
	if page.Faces[0].OriginalKey != page.Faces[3].OriginalKey || page.Faces[0].Bounds == page.Faces[3].Bounds {
		t.Fatal("face-specific key or shared bounds")
	}
	// Index updates invalidate old detections. Simulate detection of the changed
	// source again, with the same face geometry and person but new file metadata.
	if _, err = l.index.db.Exec(`CREATE TABLE test_saved_detection AS SELECT * FROM photo_faces WHERE id=?`, p.Faces[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = l.index.db.Exec(`UPDATE media_index SET mod_time_unix_nano=mod_time_unix_nano+1 WHERE path=(SELECT path FROM photo_faces WHERE id=?)`, p.Faces[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = l.index.db.Exec(`INSERT INTO photo_faces SELECT * FROM test_saved_detection`); err != nil {
		t.Fatal(err)
	}
	after, err := l.LabelPerson(ctx, p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if after.Faces[0].OriginalKey == p.Faces[0].OriginalKey || after.Faces[1].OriginalKey != p.Faces[1].OriginalKey {
		t.Fatal("file changes must invalidate only that original")
	}
}
