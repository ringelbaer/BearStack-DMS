package photos

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func mergeNamingDestination(t *testing.T, l *Library, face int64) LabelPerson {
	t.Helper()
	result, err := l.index.db.Exec(`INSERT INTO photo_people(name,name_fold,manual_name) VALUES('Ada','ada',1)`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT path,directory,?,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, id, face)
	if err != nil {
		t.Fatal(err)
	}
	p, err := l.LabelPerson(context.Background(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLabelMergeNamingAtomicAndReplay(t *testing.T) {
	for _, assign := range []bool{false, true} {
		t.Run(map[bool]string{false: "name", true: "assign"}[assign], func(t *testing.T) {
			ctx := context.Background()
			l, s, a := labelMergeFixture(t, false)
			a.Action = "name_merge"
			a.Name = "  Ada  "
			destination := s.Target.ID
			count := s.Source.Count + s.Target.Count
			if assign {
				p := mergeNamingDestination(t, l, s.Source.FaceID)
				a.Name = ""
				a.AssignID = p.ID
				a.AssignRevision = p.Revision
				destination = p.ID
				count += p.Count
			}
			receipt, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, a)
			if err != nil {
				t.Fatal(err)
			}
			p, err := l.LabelPerson(ctx, destination, 0)
			if err != nil || p.Name != "Ada" || p.Count != count || receipt.TargetID != destination || receipt.Groups != 2 || receipt.Faces != s.Source.Count+s.Target.Count {
				t.Fatalf("person=%+v receipt=%+v err=%v", p, receipt, err)
			}
			again, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, a)
			if err != nil || again != receipt {
				t.Fatalf("replay=%+v %v", again, err)
			}
			if next, err := l.LabelNextMergeSuggestion(ctx); err != nil || next != nil {
				t.Fatalf("next=%+v %v", next, err)
			}
			a.Name = "Changed"
			if _, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, a); err == nil {
				t.Fatal("different intent reused receipt")
			}
		})
	}
}

func TestLabelMergeNamingValidationAndRollback(t *testing.T) {
	for _, scenario := range []string{"empty", "ambiguous", "same destination", "missing revision", "stale source", "stale pair target", "stale assignment", "unnamed assignment", "named source", "named target", "duplicate", "allow duplicate", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			l, s, a := labelMergeFixture(t, false)
			a.Action = "name_merge"
			a.Name = "Ada"
			want := ErrLabelConflict
			switch scenario {
			case "empty":
				a.Name = " "
				want = ErrLabelInvalid
			case "ambiguous":
				a.AssignID = 99
				a.AssignRevision = 1
				want = ErrLabelInvalid
			case "same destination":
				a.Name = ""
				a.AssignID = s.Target.ID
				a.AssignRevision = s.Target.Revision
				want = ErrLabelInvalid
			case "missing revision":
				a.Name = ""
				a.AssignID = 99
				want = ErrLabelInvalid
			case "stale source":
				a.Revision++
			case "stale pair target":
				a.TargetRevision++
			case "named source":
				if err := l.RenamePerson(ctx, s.Source.ID, "Other"); err != nil {
					t.Fatal(err)
				}
			case "named target":
				if err := l.RenamePerson(ctx, s.Target.ID, "Other"); err != nil {
					t.Fatal(err)
				}
			case "stale assignment", "unnamed assignment":
				p := mergeNamingDestination(t, l, s.Source.FaceID)
				a.Name = ""
				a.AssignID = p.ID
				a.AssignRevision = p.Revision
				if scenario == "stale assignment" {
					a.AssignRevision++
				} else {
					if err := l.RenamePerson(ctx, p.ID, ""); err != nil {
						t.Fatal(err)
					}
				}
			case "duplicate", "allow duplicate":
				mergeNamingDestination(t, l, s.Source.FaceID)
				want = ErrLabelNameExists
				if scenario == "allow duplicate" {
					a.AllowDuplicate = true
					want = nil
				}
			case "rollback":
				if _, err := l.index.db.Exec(`CREATE TRIGGER fail_naming_receipt BEFORE INSERT ON photo_labeling_actions BEGIN SELECT RAISE(ABORT,'rollback'); END`); err != nil {
					t.Fatal(err)
				}
			}
			_, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, a)
			if scenario == "rollback" {
				if err == nil {
					t.Fatal("expected rollback")
				}
			} else if !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			if want == nil {
				return
			}
			for _, before := range []LabelPerson{s.Source, s.Target} {
				after, err := l.LabelPerson(ctx, before.ID, 0)
				if err != nil || after.Count != before.Count {
					t.Fatalf("partial merge: %+v %v", after, err)
				}
				if scenario != "named source" && scenario != "named target" && after.Name != "" {
					t.Fatalf("partial rename: %+v", after)
				}
			}
			if _, err := l.LabelReceipt(ctx, "editor", a.OperationID, a.Dataset); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("unexpected receipt: %v", err)
			}
		})
	}
}
