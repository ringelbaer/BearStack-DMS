package photos

import (
	"context"
	"fmt"
	"testing"
)

func TestLabelMergeSideActionsPreserveOtherSide(t *testing.T) {
	for _, action := range []string{"ignore", "name", "assign"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", action, reverse), func(t *testing.T) {
				ctx := context.Background()
				l, pair, original := labelMergeFixture(t, false)
				source, other := pair.Source, pair.Target
				if reverse {
					source, other = other, source
				}
				a := LabelAction{Action: action, OperationID: "individual-side-0001", Dataset: original.Dataset, Revision: source.Revision}
				if action == "name" {
					a.Name = "Ada"
				} else if action == "assign" {
					result, err := l.index.db.Exec(`INSERT INTO photo_people(name,name_fold,manual_name) VALUES('Ada','ada',1)`)
					if err != nil {
						t.Fatal(err)
					}
					a.TargetID, _ = result.LastInsertId()
					if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT path,directory,?,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, a.TargetID, source.FaceID); err != nil {
						t.Fatal(err)
					}
					target, err := l.LabelPerson(ctx, a.TargetID, 0)
					if err != nil {
						t.Fatal(err)
					}
					a.TargetRevision = target.Revision
				}
				receipt, err := l.ApplyLabelAction(ctx, "editor", source.ID, a)
				if err != nil || receipt.Faces != source.Count || receipt.SourceID != source.ID {
					t.Fatalf("side action: %+v %v", receipt, err)
				}
				if replay, err := l.ApplyLabelAction(ctx, "editor", source.ID, a); err != nil || replay != receipt {
					t.Fatalf("receipt replay: %+v %v", replay, err)
				}
				unchanged, err := l.LabelPerson(ctx, other.ID, 0)
				if err != nil || unchanged.Revision != other.Revision || unchanged.Count != other.Count || unchanged.Name != other.Name {
					t.Fatalf("other group changed: %+v %v", unchanged, err)
				}
				// The other side remains actionable with its originally reviewed revision,
				// even though changing the first group invalidates the pair suggestion.
				if _, err := l.ApplyLabelAction(ctx, "editor", other.ID, LabelAction{Action: "ignore", OperationID: "individual-side-0002", Dataset: a.Dataset, Revision: other.Revision}); err != nil {
					t.Fatal(err)
				}
				var rejected int
				if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_face_merge_suggestions WHERE rejected=1`).Scan(&rejected); err != nil || rejected != 0 {
					t.Fatalf("individual actions must not reject the pair: %d %v", rejected, err)
				}
			})
		}
	}
}
