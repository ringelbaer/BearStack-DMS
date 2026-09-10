package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func manualMergeFixture(t *testing.T) (*Library, []LabelPerson, LabelAction) {
	t.Helper()
	ctx := context.Background()
	l, first, session := labelFixture(t)
	for i := range 2 {
		p, err := l.LabelPerson(ctx, first.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		action := labelAction(p, session, "detach")
		action.OperationID += fmt.Sprint(i)
		action.FaceID = p.Faces[0].ID
		if _, err := l.ApplyLabelAction(ctx, "test", p.ID, action); err != nil {
			t.Fatal(err)
		}
	}
	session, _ = l.LabelSession(ctx)
	page, err := l.LabelMergeGroups(ctx, 0, session.UpperID, true)
	if err != nil || len(page.People) != 3 || !session.ManualMerge {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	a := labelAction(page.People[0], session, "merge_groups")
	for _, p := range page.People {
		a.Groups = append(a.Groups, LabelGroupRef{p.ID, p.Revision})
	}
	return l, page.People, a
}

func TestManualGroupMergeAtomicNamesAndReplay(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"unnamed", "named-source", "named-target", "rename"} {
		t.Run(mode, func(t *testing.T) {
			l, people, a := manualMergeFixture(t)
			wantName := ""
			if mode != "unnamed" {
				if err := l.RenamePerson(ctx, people[1].ID, "Anna"); err != nil {
					t.Fatal(err)
				}
				wantName = "Anna"
				if mode == "named-target" {
					if err := l.RenamePerson(ctx, people[0].ID, "Berta"); err != nil {
						t.Fatal(err)
					}
					wantName = "Berta"
				}
			}
			// An ignored face is transferred with the group but never restored.
			if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,ignored)
                SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model,1 FROM photo_faces WHERE id=?`, people[1].FaceID); err != nil {
				t.Fatal(err)
			}
			for i, p := range people {
				fresh, err := l.LabelPerson(ctx, p.ID, 0)
				if err != nil {
					t.Fatal(err)
				}
				a.Groups[i].Revision = fresh.Revision
			}
			a.Revision = a.Groups[0].Revision
			if mode == "rename" {
				a.Action = "name_groups"
				a.Name = "  Neue   Person  "
				wantName = "Neue Person"
			}
			receipt, err := l.ApplyLabelAction(ctx, "editor", people[0].ID, a)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.TargetID != people[0].ID || receipt.Faces != 3 || receipt.SourceRevision <= a.Revision {
				t.Fatalf("receipt=%+v", receipt)
			}
			merged, err := l.LabelPerson(ctx, people[0].ID, 0)
			if err != nil || merged.Count != 3 || merged.Name != wantName {
				t.Fatalf("merged=%+v err=%v", merged, err)
			}
			var ignored int
			if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE person_id=? AND ignored=1`, people[0].ID).Scan(&ignored); err != nil || ignored != 1 {
				t.Fatalf("ignored=%d err=%v", ignored, err)
			}
			for _, p := range people[1:] {
				if _, err := l.LabelPerson(ctx, p.ID, 0); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("source survived: %v", err)
				}
			}
			replay, err := l.ApplyLabelAction(ctx, "editor", people[0].ID, a)
			if err != nil || replay != receipt {
				t.Fatalf("replay=%+v err=%v", replay, err)
			}
			if _, err := l.LabelReceipt(ctx, "other", a.OperationID, a.Dataset); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("receipt leaked: %v", err)
			}
			a.Groups[1], a.Groups[2] = a.Groups[2], a.Groups[1]
			if _, err := l.ApplyLabelAction(ctx, "editor", people[0].ID, a); !errors.Is(err, ErrLabelConflict) {
				t.Fatalf("payload collision: %v", err)
			}
		})
	}
}

func TestManualGroupMergeRejectsStaleOrInvalidSelectionWithoutPartialChanges(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []string{"revision", "dataset", "missing", "duplicate", "single", "too-many", "target", "empty-name", "private", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			l, people, a := manualMergeFixture(t)
			want := ErrLabelInvalid
			switch scenario {
			case "revision":
				a.Groups[2].Revision++
				want = ErrLabelConflict
			case "dataset":
				a.Dataset = "other"
				want = ErrLabelConflict
			case "missing":
				a.Groups[2].ID = 9999
				want = ErrLabelConflict
			case "duplicate":
				a.Groups[2] = a.Groups[1]
			case "single":
				a.Groups = a.Groups[:1]
			case "too-many":
				for len(a.Groups) <= MaxLabelMergeGroups {
					a.Groups = append(a.Groups, LabelGroupRef{int64(100 + len(a.Groups)), 1})
				}
			case "target":
				a.TargetID = people[1].ID
			case "empty-name":
				a.Action = "name_groups"
			case "private":
				if err := os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				want = ErrLabelConflict
			case "rollback":
				if _, err := l.index.db.Exec(`CREATE TRIGGER fail_manual_receipt BEFORE INSERT ON photo_labeling_actions BEGIN SELECT RAISE(ABORT,'rollback'); END`); err != nil {
					t.Fatal(err)
				}
			}
			_, err := l.ApplyLabelAction(ctx, "editor", people[0].ID, a)
			if scenario == "rollback" {
				if err == nil {
					t.Fatal("expected rollback")
				}
			} else if !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			if scenario != "private" {
				for _, p := range people {
					got, err := l.LabelPerson(ctx, p.ID, 0)
					if err != nil || got.Count != p.Count || got.Revision != p.Revision {
						t.Fatalf("partial mutation: %+v %v", got, err)
					}
				}
			}
			if _, err := l.LabelReceipt(ctx, "editor", a.OperationID, a.Dataset); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("receipt after failure: %v", err)
			}
		})
	}
}

func TestManualGroupNamingDuplicateProtection(t *testing.T) {
	ctx := context.Background()
	l, people, a := manualMergeFixture(t)
	if err := l.RenamePerson(ctx, people[2].ID, "Anna"); err != nil {
		t.Fatal(err)
	}
	a.Action = "name_groups"
	a.Name = "Anna"
	a.Groups = a.Groups[:2]
	if _, err := l.ApplyLabelAction(ctx, "editor", people[0].ID, a); !errors.Is(err, ErrLabelNameExists) {
		t.Fatalf("duplicate: %v", err)
	}
	a.AllowDuplicate = true
	if _, err := l.ApplyLabelAction(ctx, "editor", people[0].ID, a); err != nil {
		t.Fatal(err)
	}
	if p, err := l.LabelPerson(ctx, people[2].ID, 0); err != nil || p.Count != 1 {
		t.Fatalf("unselected group changed: %+v %v", p, err)
	}
}

func TestManualGroupPagesFilterCursorPortraitsAndProtection(t *testing.T) {
	ctx := context.Background()
	l, people, _ := manualMergeFixture(t)
	for i := 0; i < 45; i++ {
		name := ""
		if i%2 == 0 {
			name = "Named"
		}
		result, err := l.index.db.Exec(`INSERT INTO photo_people(name,manual_name) VALUES(?,1)`, name)
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
            SELECT path,directory,?,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, id, people[0].FaceID); err != nil {
			t.Fatal(err)
		}
	}
	session, err := l.LabelSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, named := range []bool{false, true} {
		ids := []int64{}
		after := int64(0)
		for {
			page, err := l.LabelMergeGroups(ctx, after, session.UpperID, named)
			if err != nil || len(page.People) > 20 {
				t.Fatalf("page=%+v %v", page, err)
			}
			for _, p := range page.People {
				if p.ID <= after || (!named && p.Name != "") || len(p.Faces) != 1 || p.Faces[0].ID != p.FaceID || len(p.Faces[0].OriginalKey) != 64 || p.Faces[0].Bounds.Width <= 0 {
					t.Fatalf("invalid item: %+v", p)
				}
				ids = append(ids, p.ID)
			}
			if !page.HasNext {
				break
			}
			if page.Next <= after {
				t.Fatal("cursor did not advance")
			}
			after = page.Next
		}
		want := 48
		if !named {
			want = 25
		}
		if len(ids) != want {
			t.Fatalf("named=%v got=%d want=%d", named, len(ids), want)
		}
		if page, err := l.LabelMergeGroups(ctx, 0, people[2].ID, named); err != nil || !reflect.DeepEqual([]int64{page.People[0].ID, page.People[1].ID, page.People[2].ID}, []int64{people[0].ID, people[1].ID, people[2].ID}) {
			t.Fatalf("upper bound: %+v %v", page, err)
		}
	}
	if err := os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if page, err := l.LabelMergeGroups(ctx, 0, session.UpperID, true); err != nil || len(page.People) != 0 {
		t.Fatalf("private groups leaked: %+v %v", page, err)
	}
}
