package photos

import (
	"context"
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
			page, err := l.People(ctx, 0, 1, "", false)
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
			merged, err := l.People(ctx, 0, 1, "", false)
			if err != nil || len(merged.People) != 1 || merged.People[0].ID != target || merged.People[0].Name != tc.want || merged.People[0].Count != 3 {
				t.Fatalf("merge: %+v %v", merged, err)
			}
			if tc.want != "" {
				known, err := l.People(ctx, 0, 1, tc.want, true)
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
