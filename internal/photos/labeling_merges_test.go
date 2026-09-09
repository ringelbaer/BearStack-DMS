package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func labelMergeFixture(t *testing.T, named bool) (*Library, *LabelMergeSuggestion, LabelAction) {
	t.Helper()
	l, _, _ := reconciliationFixture(t, .52, named)
	completeReconciliation(t, l)
	s, err := l.LabelNextMergeSuggestion(context.Background())
	if err != nil || s == nil {
		t.Fatalf("suggestion=%+v error=%v", s, err)
	}
	session, err := l.LabelSession(context.Background())
	if err != nil || !session.MergeSuggestions {
		t.Fatalf("session=%+v error=%v", session, err)
	}
	a := LabelAction{OperationID: "merge-decision-0001", Dataset: session.Dataset, Revision: s.Source.Revision,
		TargetID: s.Target.ID, TargetRevision: s.Target.Revision, SuggestionID: s.ID}
	return l, s, a
}

func TestLabelMergeDecisionAndReplay(t *testing.T) {
	ctx := context.Background()
	for _, action := range []string{"accept_merge", "reject_merge"} {
		for _, named := range []bool{false, true} {
			t.Run(action+map[bool]string{false: "/unnamed", true: "/named"}[named], func(t *testing.T) {
				l, s, a := labelMergeFixture(t, named)
				a.Action = action
				for _, p := range []LabelPerson{s.Source, s.Target} {
					if len(p.Faces) != 1 || p.Faces[0].ID != p.FaceID || p.Faces[0].DisplayPath == "" || len(p.Faces[0].OriginalKey) != 64 || p.Faces[0].Bounds.Width <= 0 {
						t.Fatalf("incomplete portrait: %+v", p)
					}
				}
				r, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, a)
				if err != nil || r.Action != action || r.TargetID != s.Target.ID || r.Groups != 0 {
					t.Fatalf("receipt=%+v error=%v", r, err)
				}
				if again, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, a); err != nil || again != r {
					t.Fatalf("replay=%+v error=%v", again, err)
				}
				if receipt, err := l.LabelReceipt(ctx, "editor", a.OperationID, a.Dataset); err != nil || receipt != r {
					t.Fatalf("lookup=%+v error=%v", receipt, err)
				}
				if _, err := l.LabelReceipt(ctx, "other", a.OperationID, a.Dataset); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("receipt leaked to another actor: %v", err)
				}
				if next, err := l.LabelNextMergeSuggestion(ctx); err != nil || next != nil {
					t.Fatalf("decided pair still offered: %+v %v", next, err)
				}
				if action == "accept_merge" {
					p, err := l.LabelPerson(ctx, s.Target.ID, 0)
					if err != nil || p.Count != s.Source.Count+s.Target.Count || p.Name != s.Target.Name || r.Faces != s.Source.Count || r.SourceRevision != 0 {
						t.Fatalf("merge result=%+v receipt=%+v error=%v", p, r, err)
					}
				} else {
					if r.Faces != 0 || r.SourceRevision != s.Source.Revision {
						t.Fatalf("rejection changed faces: %+v", r)
					}
					if err := l.ScheduleFaceReconciliation(ctx); err != nil {
						t.Fatal(err)
					}
					completeReconciliation(t, l)
					if next, err := l.LabelNextMergeSuggestion(ctx); err != nil || next != nil {
						t.Fatalf("rejected pair regenerated: %+v %v", next, err)
					}
				}
				a.OperationID = "merge-decision-0002"
				if _, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, a); !errors.Is(err, ErrLabelConflict) {
					t.Fatalf("second decision accepted: %v", err)
				}
			})
		}
	}
}

func TestLabelMergeConflictsAndRollback(t *testing.T) {
	ctx := context.Background()
	for _, action := range []string{"accept_merge", "reject_merge"} {
		for _, scenario := range []string{"source revision", "target revision", "suggestion", "dataset", "missing suggestion", "same group", "private source", "private target", "rollback"} {
			t.Run(action+"/"+scenario, func(t *testing.T) {
				l, s, a := labelMergeFixture(t, true)
				a.Action = action
				want := ErrLabelConflict
				switch scenario {
				case "source revision":
					if err := l.RenamePerson(ctx, s.Source.ID, "Changed"); err != nil {
						t.Fatal(err)
					}
				case "target revision":
					if err := l.RenamePerson(ctx, s.Target.ID, "Changed"); err != nil {
						t.Fatal(err)
					}
				case "suggestion":
					a.SuggestionID++
				case "dataset":
					a.Dataset = "another dataset"
				case "missing suggestion":
					a.SuggestionID = 0
					want = ErrLabelInvalid
				case "same group":
					a.TargetID = s.Source.ID
					want = ErrLabelInvalid
				case "private source", "private target":
					p := s.Source
					if scenario == "private target" {
						p = s.Target
					}
					face, err := l.Face(ctx, p.FaceID)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(l.Root(), parentPath(face.Path), ".adminonly"), nil, 0600); err != nil {
						t.Fatal(err)
					}
				case "rollback":
					if _, err := l.index.db.Exec(`CREATE TRIGGER fail_merge_receipt BEFORE INSERT ON photo_labeling_actions BEGIN SELECT RAISE(ABORT,'rollback'); END`); err != nil {
						t.Fatal(err)
					}
				}
				_, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, a)
				if scenario == "rollback" {
					if err == nil {
						t.Fatal("expected receipt failure")
					}
					if next, err := l.LabelNextMergeSuggestion(ctx); err != nil || next == nil || next.ID != s.ID {
						t.Fatalf("mutation escaped rollback: %+v %v", next, err)
					}
				} else if !errors.Is(err, want) {
					t.Fatalf("got %v want %v", err, want)
				}
				if _, err := l.LabelReceipt(ctx, "editor", a.OperationID, a.Dataset); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("failed action has receipt: %v", err)
				}
			})
		}
	}
}

func TestLabelMergeUsesMatchingWitnessBeyondFirstPage(t *testing.T) {
	l, s, _ := labelMergeFixture(t, true)
	ctx := context.Background()
	// A large group whose matching reference is its last face must still produce
	// exactly one portrait, found using the face-ID index instead of offset scans.
	var last int64
	for range 80 {
		r, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, s.Target.FaceID)
		if err != nil {
			t.Fatal(err)
		}
		last, err = r.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_face_merge_suggestions(id,source_id,target_id,source_revision,target_revision,source_face_id,target_face_id,score,model)
 VALUES(?,?,?,?,(SELECT revision FROM photo_person_revisions WHERE person_id=?),?,?,.52,(SELECT model FROM photo_face_state WHERE id=1))`, s.ID, s.Source.ID, s.Target.ID, s.Source.Revision, s.Target.ID, s.Source.FaceID, last); err != nil {
		t.Fatal(err)
	}
	next, err := l.LabelNextMergeSuggestion(ctx)
	if err != nil || next == nil || next.Target.Count != 81 || len(next.Target.Faces) != 1 || next.Target.Faces[0].ID != last {
		t.Fatalf("wrong witness: %+v %v", next, err)
	}
}

func TestLabelMergeConcurrentOppositeDecisionsCommitOnlyOnce(t *testing.T) {
	l, s, a := labelMergeFixture(t, true)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, action := range []string{"accept_merge", "reject_merge"} {
		go func() {
			<-start
			decision := a
			decision.Action = action
			decision.OperationID = "concurrent-" + action
			_, err := l.ApplyLabelAction(ctx, "editor", s.Source.ID, decision)
			results <- err
		}()
	}
	close(start)
	commits, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			commits++
		case errors.Is(err, ErrLabelConflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if commits != 1 || conflicts != 1 {
		t.Fatalf("commits=%d conflicts=%d", commits, conflicts)
	}
	var receipts int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_labeling_actions`).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("receipts=%d error=%v", receipts, err)
	}
}
