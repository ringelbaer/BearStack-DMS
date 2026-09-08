package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamedPeopleIndexUpgradeAndCursorQuery(t *testing.T) {
	ctx := context.Background()
	l, p, session := labelFixture(t)
	if err := l.RenamePerson(ctx, p.ID, "Anna"); err != nil {
		t.Fatal(err)
	}
	p, _ = l.LabelPerson(ctx, p.ID, 0)
	for _, stmt := range []string{
		`DROP INDEX idx_photo_people_named_id`,
		`UPDATE schema_migrations SET version=23 WHERE component='photos'`,
		`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000) INSERT INTO photo_people(name) SELECT '' FROM n`,
	} {
		if _, err := l.index.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := runPhotoSchemaMigrations(ctx, l.index.db); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := l.LabelPerson(ctx, p.ID, 0)
	fresh, _ := l.LabelSession(ctx)
	if after.Name != p.Name || after.Revision != p.Revision || fresh.Dataset != session.Dataset {
		t.Fatal("migration changed data")
	}
	rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN SELECT p.id FROM photo_people p WHERE p.name<>'' AND p.id>? AND p.id<=? AND `+labelExists+` ORDER BY p.id LIMIT 21`, 0, fresh.UpperID)
	if err != nil {
		t.Fatal(err)
	}
	plan := ""
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	err = rows.Err()
	rows.Close()
	if err != nil || !strings.Contains(plan, "idx_photo_people_named_id") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("unbounded cursor plan: %s (%v)", plan, err)
	}
	page, err := l.LabelNamedPeople(ctx, 0, fresh.UpperID)
	if err != nil || len(page.People) != 1 || page.HasNext {
		t.Fatalf("page: %+v %v", page, err)
	}
}

func TestNamedPersonDuplicateRenameDoesNotMerge(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	a := labelAction(p, s, "detach")
	a.FaceID = p.FaceID
	r, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.RenamePerson(ctx, p.ID, "Anna"); err != nil {
		t.Fatal(err)
	}
	if err = l.RenamePerson(ctx, r.NewID, "Berta"); err != nil {
		t.Fatal(err)
	}
	p, _ = l.LabelPerson(ctx, p.ID, 0)
	a = labelAction(p, s, "rename")
	a.Name = "Berta"
	if _, err = l.ApplyLabelAction(ctx, "m", p.ID, a); !errors.Is(err, ErrLabelNameExists) {
		t.Fatal(err)
	}
	a.AllowDuplicate = true
	if _, err = l.ApplyLabelAction(ctx, "m", p.ID, a); err != nil {
		t.Fatal(err)
	}
	names, err := l.LabelSuggestions(ctx, "Berta", true)
	if err != nil || len(names) != 2 {
		t.Fatalf("unexpected merge: %+v %v", names, err)
	}
}

func TestLabelNamedPeoplePaginationAndProtection(t *testing.T) {
	ctx := context.Background()
	l, source, session := labelFixture(t)
	if !session.NamedPeople {
		t.Fatal("missing capability")
	}
	for i := 0; i < 45; i++ {
		result, err := l.index.db.Exec(`INSERT INTO photo_people(name,name_fold,manual_name) VALUES('Anna','anna',1)`)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		_, err = l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT path,directory,?,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, id, source.FaceID)
		if err != nil {
			t.Fatal(err)
		}
	}
	bounded, err := l.LabelNamedPeople(ctx, 0, session.UpperID)
	if err != nil || len(bounded.People) != 0 {
		t.Fatalf("upper: %+v %v", bounded, err)
	}
	session, _ = l.LabelSession(ctx)
	seen := map[int64]bool{}
	cursor := int64(0)
	for _, size := range []int{20, 20, 5} {
		page, err := l.LabelNamedPeople(ctx, cursor, session.UpperID)
		if err != nil || len(page.People) != size || page.HasNext != (size == 20) {
			t.Fatalf("page: %+v %v", page, err)
		}
		for _, p := range page.People {
			if p.ID <= cursor || seen[p.ID] || p.Name != "Anna" || p.Count != 1 || p.FaceID == 0 || len(p.Faces) != 0 {
				t.Fatalf("person: %+v", p)
			}
			seen[p.ID] = true
		}
		cursor = page.Next
	}
	if len(seen) != 45 {
		t.Fatal(len(seen))
	}
	if err := os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	page, err := l.LabelNamedPeople(ctx, 0, session.UpperID)
	if err != nil || len(page.People) != 0 || page.HasNext {
		t.Fatalf("private: %+v %v", page, err)
	}
}

func TestLabelNamedManagementRevisionsReceiptsAndLastFace(t *testing.T) {
	ctx := context.Background()
	l, p, session := labelFixture(t)
	if err := l.RenamePerson(ctx, p.ID, "Anna"); err != nil {
		t.Fatal(err)
	}
	p, _ = l.LabelPerson(ctx, p.ID, 0)
	for _, kind := range []string{"name", "detach", "ignore"} {
		a := labelAction(p, session, kind)
		a.Name = "Wrong"
		a.FaceID = p.FaceID
		if _, err := l.ApplyLabelAction(ctx, "m", p.ID, a); !errors.Is(err, ErrLabelConflict) {
			t.Fatalf("legacy %s: %v", kind, err)
		}
	}
	a := labelAction(p, session, "favorite")
	favorite := true
	a.Favorite = &favorite
	a.FaceID = p.FaceID
	r, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
	if err != nil || r.Faces != 1 || r.Groups != 0 {
		t.Fatalf("favorite: %+v %v", r, err)
	}
	if again, err := l.ApplyLabelAction(ctx, "m", p.ID, a); err != nil || again != r {
		t.Fatalf("replay: %+v %v", again, err)
	}
	stale := labelAction(p, session, "rename")
	stale.Name = "New"
	if err := l.RenamePerson(ctx, p.ID, "Webänderung"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ApplyLabelAction(ctx, "m", p.ID, stale); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("stale: %v", err)
	}
	p, _ = l.LabelPerson(ctx, p.ID, 0)
	if !p.Faces[0].Favorite {
		t.Fatal("favorite missing")
	}
	a = labelAction(p, session, "rename")
	a.Name = "  Anna   Neu  "
	if _, err := l.ApplyLabelAction(ctx, "m", p.ID, a); err != nil {
		t.Fatal(err)
	}
	p, _ = l.LabelPerson(ctx, p.ID, 0)
	if p.Name != "Anna Neu" {
		t.Fatal(p.Name)
	}
	// Every face, including the last, becomes a manually unassigned group.
	for i := 0; i < 3; i++ {
		a = labelAction(p, session, "unassign")
		a.OperationID += fmt.Sprint(i)
		a.FaceID = p.Faces[0].ID
		r, err = l.ApplyLabelAction(ctx, "m", p.ID, a)
		if err != nil || r.NewID == 0 || r.Faces != 1 || r.Groups != 0 {
			t.Fatalf("unassign: %+v %v", r, err)
		}
		if again, err := l.ApplyLabelAction(ctx, "m", p.ID, a); err != nil || again != r {
			t.Fatalf("unassign replay: %+v %v", again, err)
		}
		newPerson, err := l.LabelPerson(ctx, r.NewID, 0)
		if err != nil || newPerson.Name != "" || newPerson.Count != 1 || newPerson.Faces[0].Favorite {
			t.Fatalf("new person: %+v %v", newPerson, err)
		}
		face, err := l.Face(ctx, a.FaceID)
		if err != nil || face.Ignored || !face.Manual {
			t.Fatalf("face: %+v %v", face, err)
		}
		if _, err := os.Stat(filepath.Join(l.Root(), face.Path)); err != nil {
			t.Fatal("original removed", err)
		}
		p, err = l.LabelPerson(ctx, r.SourceID, 0)
		if i < 2 && err != nil || i == 2 && !errors.Is(err, sql.ErrNoRows) {
			t.Fatal(err)
		}
	}
	page, err := l.LabelNamedPeople(ctx, 0, session.UpperID)
	if err != nil || len(page.People) != 0 {
		t.Fatalf("empty named group: %+v %v", page, err)
	}
}

func TestLabelNamedManagementValidationAndRollback(t *testing.T) {
	for _, kind := range []string{"rename", "unassign", "favorite"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			l, p, s := labelFixture(t)
			a := labelAction(p, s, kind)
			a.Name = "Anna"
			a.FaceID = p.FaceID
			favorite := true
			a.Favorite = &favorite
			if _, err := l.ApplyLabelAction(ctx, "m", p.ID, a); !errors.Is(err, ErrLabelConflict) {
				t.Fatalf("unnamed accepted: %v", err)
			}
			if err := l.RenamePerson(ctx, p.ID, "Old"); err != nil {
				t.Fatal(err)
			}
			p, _ = l.LabelPerson(ctx, p.ID, 0)
			a.Revision = p.Revision
			if kind == "favorite" {
				a.Favorite = nil
				if _, err := l.ApplyLabelAction(ctx, "m", p.ID, a); !errors.Is(err, ErrLabelInvalid) {
					t.Fatal(err)
				}
				a.Favorite = &favorite
			}
			if kind != "rename" {
				a.FaceID = 999999
				if _, err := l.ApplyLabelAction(ctx, "m", p.ID, a); !errors.Is(err, ErrLabelConflict) {
					t.Fatal(err)
				}
				a.FaceID = p.FaceID
			}
			if _, err := l.index.db.Exec(`CREATE TRIGGER fail_named_receipt BEFORE INSERT ON photo_labeling_actions BEGIN SELECT RAISE(ABORT,'rollback'); END`); err != nil {
				t.Fatal(err)
			}
			if _, err := l.ApplyLabelAction(ctx, "m", p.ID, a); err == nil {
				t.Fatal("expected rollback")
			}
			after, err := l.LabelPerson(ctx, p.ID, 0)
			if err != nil || after.Revision != p.Revision || after.Count != p.Count || after.Name != p.Name || after.Faces[0].Favorite {
				t.Fatalf("partial mutation: %+v %v", after, err)
			}
			if err := os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := l.ApplyLabelAction(ctx, "m", p.ID, a); !errors.Is(err, ErrLabelConflict) {
				t.Fatalf("private: %v", err)
			}
		})
	}
}
