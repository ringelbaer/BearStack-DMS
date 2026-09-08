package photos

import (
	"context"
	"strings"
	"testing"
)

func TestMergePeoplePreservesNamedGroup(t *testing.T) {
	for _, tc := range []struct{ label, target, source, other, want string }{
		{"unnamed target", "", "Petra", "", "Petra"},
		{"named target", "Petra", "", "", "Petra"},
		{"named additional source", "", "", "Petra", "Petra"},
		{"multiple names preserve target", "Petra", "Marie", "Anna", "Petra"},
		{"multiple sources first name", "", "Marie", "Anna", "Marie"},
		{"all unnamed", "", "", "", ""},
	} {
		t.Run(tc.label, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
			for i := 0; i < 3; i++ {
				finishFace(t, l, i)
			}
			page, err := l.People(ctx, 0, 1, "", false, false)
			if err != nil || len(page.People) != 3 {
				t.Fatalf("people: %+v %v", page, err)
			}
			target, source, other := page.People[0].ID, page.People[1].ID, page.People[2].ID
			for id, name := range map[int64]string{target: tc.target, source: tc.source, other: tc.other} {
				if name != "" {
					if err := l.RenamePerson(ctx, id, name); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := l.MergePeople(ctx, source, target, other); err != nil {
				t.Fatal(err)
			}
			merged, err := l.People(ctx, 0, 1, "", false, false)
			if err != nil || len(merged.People) != 1 || merged.People[0].ID != target || merged.People[0].Name != tc.want || merged.People[0].Count != 3 {
				t.Fatalf("merge: %+v %v", merged, err)
			}
			if tc.want != "" {
				known, err := l.People(ctx, 0, 1, tc.want, true, false)
				if err != nil || len(known.People) != 1 {
					t.Fatalf("name search: %+v %v", known, err)
				}
				var manual bool
				if err := l.index.db.QueryRowContext(ctx, `SELECT manual_name FROM photo_people WHERE id=?`, target).Scan(&manual); err != nil || !manual {
					t.Fatalf("manual name lost: %v %v", manual, err)
				}
			}
		})
	}
}

func TestMergePeopleNamedAtomic(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
	for i := 0; i < 3; i++ {
		finishFace(t, l, i)
	}
	page, err := l.People(ctx, 0, 1, "", false, false)
	if err != nil || len(page.People) != 3 {
		t.Fatalf("people: %+v %v", page, err)
	}
	target, source, other := page.People[0].ID, page.People[1].ID, page.People[2].ID
	if err := l.RenamePerson(ctx, target, "Vorher"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		extra int64
	}{{"", other}, {strings.Repeat("x", 201), other}, {"Neu", 999999}, {"Neu", target}} {
		if err := l.MergePeopleNamed(ctx, source, target, tc.name, tc.extra); err == nil {
			t.Fatal("accepted invalid merge")
		}
	}
	// Force a failure after the target was renamed to verify transaction rollback.
	if _, err := l.index.db.ExecContext(ctx, `CREATE TRIGGER reject_named_merge BEFORE UPDATE OF person_id ON photo_faces BEGIN SELECT RAISE(ABORT, 'test rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := l.MergePeopleNamed(ctx, source, target, "Neu", other); err == nil {
		t.Fatal("expected rollback")
	}
	page, err = l.People(ctx, 0, 1, "", false, false)
	if err != nil || len(page.People) != 3 {
		t.Fatalf("partial merge: %+v %v", page, err)
	}
	var name string
	if err := l.index.db.QueryRowContext(ctx, `SELECT name FROM photo_people WHERE id=?`, target).Scan(&name); err != nil || name != "Vorher" {
		t.Fatalf("partial rename: %q %v", name, err)
	}
	if _, err := l.index.db.ExecContext(ctx, `DROP TRIGGER reject_named_merge`); err != nil {
		t.Fatal(err)
	}
	if err := l.MergePeopleNamed(ctx, source, target, "  Gemeinsam  ", other, source); err != nil {
		t.Fatal(err)
	}
	page, err = l.People(ctx, 0, 1, "Gemeinsam", true, false)
	if err != nil || len(page.People) != 1 || page.People[0].Name != "Gemeinsam" || page.People[0].Count != 3 || page.People[0].ID != target {
		t.Fatalf("named merge: %+v %v", page, err)
	}
}
