package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabelCandidatesOnlyChecksCandidatePage(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "first/a.jpg", "later/b.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "first/a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces: %v %v", faces, err)
	}
	for i := 0; i < 50; i++ {
		result, err := l.index.db.Exec(`INSERT INTO photo_people(name,manual_name) VALUES('',1)`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		path, dir := "first/a.jpg", "first"
		if i == 49 {
			path, dir = "later/b.jpg", "later"
		}
		_, err = l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT ?,?,?,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, path, dir, id, faces[0].ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(filepath.Join(l.Root(), "later"), filepath.Join(t.TempDir(), "offline")); err != nil {
		t.Fatal(err)
	}
	session, err := l.LabelSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	trace := NewListTrace()
	page, err := l.LabelCandidates(ContextWithListTrace(ctx, trace), 0, session.UpperID)
	if err != nil || len(page.People) != 20 || !page.HasNext {
		t.Fatalf("first page: %+v %v", page, err)
	}
	for _, step := range trace.Snapshot().Steps {
		if step.Name == "photos.faces.visibility" {
			for _, field := range step.Fields {
				if field.Key == "directories" && field.Value != "1" {
					t.Fatalf("unexpected scan: %+v", step)
				}
			}
		}
	}
	second, err := l.LabelCandidates(ctx, page.Next, session.UpperID)
	if err != nil || len(second.People) != 20 || second.People[0].ID <= page.Next {
		t.Fatalf("second: %+v %v", second, err)
	}
	// A directory is still checked, and fails closed, when its page is reached.
	if _, err = l.LabelCandidates(ctx, second.Next, session.UpperID); err == nil {
		t.Fatal("offline directory was exposed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = l.LabelCandidates(canceled, 0, session.UpperID); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestLabelCandidatesIncludesNewlyPrivateImportedName(t *testing.T) {
	for _, merge := range []bool{false, true} {
		t.Run(fmt.Sprint(merge), func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "first/a.jpg", "source/z.jpg")
			finishFace(t, l, 0)
			faces, err := l.AutomaticFaces(ctx, "first/a.jpg")
			if err != nil || len(faces) != 1 {
				t.Fatalf("faces: %v %v", faces, err)
			}
			id := faces[0].PersonID
			if _, err = l.index.db.Exec(`UPDATE photo_people SET name='Imported',name_fold='imported',manual_name=0,name_source='source/z.jpg' WHERE id=?`, id); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(l.Root(), "source/.adminonly"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			var page LabelCandidates
			if merge {
				page, err = l.LabelMergeGroups(ctx, 0, id, false)
			} else {
				page, err = l.LabelCandidates(ctx, 0, id)
			}
			if err != nil || len(page.People) != 1 || page.People[0].ID != id || page.People[0].Name != "" {
				t.Fatalf("new unnamed group missed: %+v %v", page, err)
			}
		})
	}
}

func TestLabelCandidateIndexMigrationAndPlan(t *testing.T) {
	ctx := context.Background()
	l, _, session := labelFixture(t)
	for _, sql := range []string{
		`DROP INDEX idx_photo_people_label_candidates`,
		`UPDATE schema_migrations SET version=30 WHERE component='photos'`,
		`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000) INSERT INTO photo_people(name,manual_name) SELECT 'Manual',1 FROM n`,
	} {
		if _, err := l.index.db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := runPhotoSchemaMigrations(ctx, l.index.db); err != nil {
			t.Fatal(err)
		}
	}
	current, err := l.LabelSession(ctx)
	if err != nil || current.Dataset != session.Dataset {
		t.Fatalf("identity changed: %+v %v", current, err)
	}
	candidates, _ := labelPeopleUnnamed.predicates()
	rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN SELECT p.id FROM photo_people p WHERE `+candidates+` AND p.id>? AND p.id<=? AND `+labelExists+` ORDER BY p.id LIMIT 21`, 0, current.UpperID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan string
	for rows.Next() {
		var a, b, c int
		var detail string
		if err := rows.Scan(&a, &b, &c, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "idx_photo_people_label_candidates") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatal(plan)
	}
}
