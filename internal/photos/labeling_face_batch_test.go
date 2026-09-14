package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLabelFaceBatchAtomicSelectionAndReceipts(t *testing.T) {
	for _, kind := range []string{"unassign_faces", "name_faces", "assign_faces", "ignore_faces"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			l, p, s := labelFixture(t)
			if !s.NamedFaceBatch {
				t.Fatal("missing capability")
			}
			if err := l.RenamePerson(ctx, p.ID, "Anna"); err != nil {
				t.Fatal(err)
			}
			if _, err := l.index.db.Exec(`UPDATE photo_faces SET favorite=1 WHERE person_id=?`, p.ID); err != nil {
				t.Fatal(err)
			}
			p, _ = l.LabelPerson(ctx, p.ID, 0)
			ids := []int64{p.Faces[0].ID, p.Faces[1].ID}
			a := labelAction(p, s, kind)
			a.FaceIDs = ids
			if kind == "name_faces" {
				a.Name = "Berta"
			}
			if kind == "assign_faces" {
				split := labelAction(p, s, "unassign")
				split.FaceID = p.Faces[2].ID
				r, err := l.ApplyLabelAction(ctx, "m", p.ID, split)
				if err != nil {
					t.Fatal(err)
				}
				if err = l.RenamePerson(ctx, r.NewID, "Berta"); err != nil {
					t.Fatal(err)
				}
				target, _ := l.LabelPerson(ctx, r.NewID, 0)
				a.TargetID = target.ID
				a.TargetRevision = target.Revision
				p, _ = l.LabelPerson(ctx, p.ID, 0)
				a.Revision = p.Revision
			}
			r, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
			if err != nil || r.Faces != 2 || r.Groups != 0 {
				t.Fatalf("batch: %+v %v", r, err)
			}
			replay, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
			if err != nil || replay != r {
				t.Fatalf("replay: %+v %v", replay, err)
			}
			receipt, err := l.LabelReceipt(ctx, "m", a.OperationID, s.Dataset)
			if err != nil || receipt != r {
				t.Fatalf("receipt: %+v %v", receipt, err)
			}
			a.FaceIDs = []int64{ids[0]}
			if _, err = l.ApplyLabelAction(ctx, "m", p.ID, a); !errors.Is(err, ErrLabelConflict) {
				t.Fatalf("changed replay: %v", err)
			}
			for _, id := range ids {
				f, err := l.Face(ctx, id)
				if err != nil || !f.Manual || f.Ignored != (kind == "ignore_faces") {
					t.Fatalf("face: %+v %v", f, err)
				}
				if f.Favorite != (kind != "unassign_faces") {
					t.Fatalf("favorite: %+v", f)
				}
				if kind == "unassign_faces" && (f.PersonID != r.NewID || f.Favorite) {
					t.Fatalf("reset: %+v", f)
				}
				if (kind == "name_faces" || kind == "assign_faces") && f.PersonID != r.TargetID {
					t.Fatalf("assignment: %+v", f)
				}
				if _, err = os.Stat(filepath.Join(l.Root(), f.Path)); err != nil {
					t.Fatal("original removed", err)
				}
			}
			if kind != "assign_faces" {
				left, err := l.LabelPerson(ctx, p.ID, 0)
				if err != nil || left.Name != "Anna" || left.Count != 1 || left.Faces[0].ID != p.Faces[2].ID {
					t.Fatalf("unselected changed: %+v %v", left, err)
				}
			} else {
				target, err := l.LabelPerson(ctx, r.TargetID, 0)
				if err != nil || target.Name != "Berta" || target.Count != 3 {
					t.Fatalf("last faces: %+v %v", target, err)
				}
			}
		})
	}
}

func TestLabelFaceBatchValidationConflictAndRollback(t *testing.T) {
	for _, scenario := range []string{"empty", "duplicate", "limit", "negative", "foreign", "ignored", "stale", "unnamed", "target_stale", "target_missing", "duplicate_name", "rollback", "private"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			l, p, s := labelFixture(t)
			if scenario != "unnamed" {
				if err := l.RenamePerson(ctx, p.ID, "Anna"); err != nil {
					t.Fatal(err)
				}
			}
			p, _ = l.LabelPerson(ctx, p.ID, 0)
			a := labelAction(p, s, "unassign_faces")
			a.FaceIDs = []int64{p.Faces[0].ID, p.Faces[1].ID}
			expected := ErrLabelConflict
			switch scenario {
			case "empty":
				a.FaceIDs = nil
				expected = ErrLabelInvalid
			case "duplicate":
				a.FaceIDs = []int64{p.FaceID, p.FaceID}
				expected = ErrLabelInvalid
			case "limit":
				a.FaceIDs = make([]int64, 501)
				expected = ErrLabelInvalid
			case "negative":
				a.FaceIDs[1] = -1
				expected = ErrLabelInvalid
			case "foreign":
				a.FaceIDs[1] = 999999
			case "ignored":
				if err := l.EditFaces(ctx, []int64{a.FaceIDs[1]}, 0, true, ""); err != nil {
					t.Fatal(err)
				}
				p, _ = l.LabelPerson(ctx, p.ID, 0)
				a.Revision = p.Revision
			case "stale":
				a.Revision++
			case "duplicate_name":
				a.Action = "name_faces"
				a.Name = "Anna"
				expected = ErrLabelNameExists
			case "target_missing":
				a.Action = "assign_faces"
				a.TargetID = 999999
				a.TargetRevision = 1
			case "target_stale":
				split := labelAction(p, s, "unassign")
				split.FaceID = p.Faces[2].ID
				r, err := l.ApplyLabelAction(ctx, "m", p.ID, split)
				if err != nil {
					t.Fatal(err)
				}
				if err = l.RenamePerson(ctx, r.NewID, "Berta"); err != nil {
					t.Fatal(err)
				}
				target, _ := l.LabelPerson(ctx, r.NewID, 0)
				p, _ = l.LabelPerson(ctx, p.ID, 0)
				a.Revision = p.Revision
				a.Action = "assign_faces"
				a.TargetID = target.ID
				a.TargetRevision = target.Revision + 1
			case "rollback":
				if _, err := l.index.db.Exec(`CREATE TRIGGER fail_batch_receipt BEFORE INSERT ON photo_labeling_actions BEGIN SELECT RAISE(ABORT,'rollback'); END`); err != nil {
					t.Fatal(err)
				}
				expected = nil
			case "private":
				if err := os.WriteFile(filepath.Join(l.Root(), ".adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
			if err == nil || expected != nil && !errors.Is(err, expected) {
				t.Fatalf("error: %v want %v", err, expected)
			}
			if scenario == "private" {
				return
			}
			after, err := l.LabelPerson(ctx, p.ID, 0)
			if err != nil || after.Count != p.Count || after.Revision != p.Revision || after.Name != p.Name {
				t.Fatalf("partial mutation: %+v %v", after, err)
			}
			if _, err = l.LabelReceipt(ctx, "m", a.OperationID, s.Dataset); err == nil {
				t.Fatal("failed action receipted")
			}
		})
	}
}

func TestLabelFaceBatchAcceptsFullLimitAndRemovesEmptySource(t *testing.T) {
	ctx := context.Background()
	l, p, s := labelFixture(t)
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<497)
 INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT f.path,f.directory,f.person_id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_faces f,n WHERE f.id=?`, p.FaceID); err != nil {
		t.Fatal(err)
	}
	if err := l.RenamePerson(ctx, p.ID, "Anna"); err != nil {
		t.Fatal(err)
	}
	p, err := l.LabelPerson(ctx, p.ID, 0)
	if err != nil || p.Count != 500 {
		t.Fatalf("fixture: %+v %v", p, err)
	}
	rows, err := l.index.db.Query(`SELECT id FROM photo_faces WHERE person_id=? ORDER BY id`, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	a := labelAction(p, s, "unassign_faces")
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		a.FaceIDs = append(a.FaceIDs, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	r, err := l.ApplyLabelAction(ctx, "m", p.ID, a)
	if err != nil || r.Faces != 500 {
		t.Fatalf("full batch: %+v %v", r, err)
	}
	reset, err := l.LabelPerson(ctx, r.NewID, 0)
	if err != nil || reset.Count != 500 || reset.Name != "" {
		t.Fatalf("new group: %+v %v", reset, err)
	}
	if _, err = l.LabelPerson(ctx, p.ID, 0); err == nil {
		t.Fatal("empty source still visible")
	}
}
