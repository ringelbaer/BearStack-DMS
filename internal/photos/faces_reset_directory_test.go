package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bearstack/internal/facerec"
)

func resetDirectoryFixture(t *testing.T) *Library {
	t.Helper()
	paths := []string{"2011/Party/one.jpg", "2011/Party/sub/two.jpg", "2011/Party-other/three.jpg", "2011/Party2/four.jpg", "2011/other/five.jpg", "2011/Party/private/six.jpg", "2011/p%_arty/seven.jpg", "2011/pXXarty/eight.jpg"}
	l := faceLibrary(t, paths...)
	if _, err := l.index.db.Exec(`INSERT INTO photo_people(id,name,name_fold,manual_name) VALUES(1,'','',1),(2,'Anna','anna',1),(3,'','',1); UPDATE photo_face_state SET model=? WHERE id=1`, facerec.Model); err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		_, err := l.index.db.Exec(`INSERT INTO photo_faces(id,path,directory,person_id,x,y,width,height,confidence,embedding,model,manual,ignored) VALUES(?,?,?,1,.1,.1,.2,.2,.99,?,?,1,1)`, i+1, path, filepath.ToSlash(filepath.Dir(path)), encodeVector(faceDetection(0).Embedding), facerec.Model)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []struct{ id, person, ignored int }{{20, 2, 0}, {21, 2, 1}, {22, 3, 0}} {
		_, err := l.index.db.Exec(`INSERT INTO photo_faces(id,path,directory,person_id,x,y,width,height,confidence,embedding,model,manual,ignored) VALUES(?,'2011/Party/one.jpg','2011/Party',?,.3,.3,.2,.2,.99,?,?,1,?)`, value.id, value.person, encodeVector(faceDetection(1).Embedding), facerec.Model, value.ignored)
		if err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func TestResetIgnoredDirectoryFacesScopeNamesAndReferences(t *testing.T) {
	l := resetDirectoryFixture(t)
	ctx := context.Background()
	before := map[int64]RecognizedFace{}
	for _, id := range []int64{1, 2, 3, 4, 5, 6, 7, 8, 20, 21, 22} {
		face, err := l.Face(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		before[id] = face
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "2011/Party/private/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	n, err := l.ResetIgnoredDirectoryFaces(ctx, "2011/Party")
	if err != nil || n != 2 {
		t.Fatalf("reset: %d %v", n, err)
	}
	for id, original := range before {
		if id == 6 {
			continue
		}
		after, err := l.Face(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if id == 1 || id == 2 {
			original.Ignored = false
		}
		if !reflect.DeepEqual(original, after) {
			t.Fatalf("face %d changed unexpectedly: %+v -> %+v", id, original, after)
		}
	}
	var ignored, refs int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE id=6 AND ignored=0`).Scan(&ignored); err != nil || ignored != 0 {
		t.Fatalf("private: %d %v", ignored, err)
	}
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references WHERE person_id=1`).Scan(&refs); err != nil || refs != 2 {
		t.Fatalf("references: %d %v", refs, err)
	}
	var beforeRevision, afterRevision int64
	l.index.db.QueryRow(`SELECT revision FROM photo_face_state WHERE id=1`).Scan(&beforeRevision)
	if n, err = l.ResetIgnoredDirectoryFaces(ctx, "2011/Party"); err != nil || n != 0 {
		t.Fatalf("repeat: %d %v", n, err)
	}
	l.index.db.QueryRow(`SELECT revision FROM photo_face_state WHERE id=1`).Scan(&afterRevision)
	if beforeRevision != afterRevision {
		t.Fatal("no-op invalidated caches")
	}
	if n, err = l.ResetIgnoredDirectoryFaces(ctx, "2011/p%_arty"); err != nil || n != 1 {
		t.Fatalf("wildcards: %d %v", n, err)
	}
	f, _ := l.Face(ctx, 8)
	if !f.Ignored {
		t.Fatal("SQL wildcard expanded")
	}
}

func TestResetIgnoredDirectoryFacesValidationAndRollback(t *testing.T) {
	l := resetDirectoryFixture(t)
	ctx := context.Background()
	for _, path := range []string{"", "2011", "/2011/Party", "2011/Party/..", "2011//Party", "../2011/Party", ".people/all", ".people/f-test/1", "2011/" + strings.Repeat("x", 4096)} {
		if n, err := l.ResetIgnoredDirectoryFaces(ctx, path); !errors.Is(err, ErrLabelInvalid) || n != 0 {
			t.Fatalf("%q: %d %v", path, n, err)
		}
	}
	for _, path := range []string{"2011/missing", "2011/Party/one.jpg"} {
		if _, err := l.ResetIgnoredDirectoryFaces(ctx, path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%q: %v", path, err)
		}
	}
	if err := os.Symlink(filepath.Join(l.Root(), "2011/Party"), filepath.Join(l.Root(), "2011/link")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ResetIgnoredDirectoryFaces(ctx, "2011/link"); err == nil {
		t.Fatal("symlink accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.ResetIgnoredDirectoryFaces(canceled, "2011/Party"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_reset_reference BEFORE INSERT ON photo_face_references BEGIN SELECT RAISE(ABORT,'test reference failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ResetIgnoredDirectoryFaces(ctx, "2011/Party"); err == nil {
		t.Fatal("expected reference failure")
	}
	var changed int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE id<20 AND ignored=0`).Scan(&changed); err != nil || changed != 0 {
		t.Fatalf("partial write: %d %v", changed, err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "2011/Party/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ResetIgnoredDirectoryFaces(ctx, "2011/Party"); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("private directory: %v", err)
	}
}

func TestResetIgnoredDirectoryFacesBulkAndIndexedScope(t *testing.T) {
	l := resetDirectoryFixture(t)
	// More than the 500-face limit of the individual edit endpoint.
	// Only bounded reference selections are rebuilt for each affected group.
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 100 UNION ALL SELECT x+1 FROM n WHERE x<750)
 INSERT INTO photo_faces(id,path,directory,person_id,x,y,width,height,confidence,embedding,model,manual,ignored)
 SELECT n.x,f.path,f.directory,f.person_id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model,f.manual,1 FROM n CROSS JOIN photo_faces f WHERE f.id=1`); err != nil {
		t.Fatal(err)
	}
	n, err := l.ResetIgnoredDirectoryFaces(context.Background(), "2011/Party")
	if err != nil || n != 654 {
		t.Fatalf("bulk: %d %v", n, err)
	}
	rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN SELECT f.id FROM photo_faces f INDEXED BY idx_photo_faces_path CROSS JOIN media_index m ON m.path=f.path CROSS JOIN photo_people p ON p.id=f.person_id WHERE f.path>=? AND f.path<? AND f.ignored=1 AND p.name='' AND m.admin_only=0`, "2011/Party/", "2011/Party0")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	indexed := false
	for rows.Next() {
		var a, b, c int
		var detail string
		if err := rows.Scan(&a, &b, &c, &detail); err != nil {
			t.Fatal(err)
		}
		indexed = indexed || strings.Contains(detail, "SEARCH f USING INDEX idx_photo_faces_path (path>?")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !indexed {
		t.Fatal("missing bounded path index search")
	}
}
