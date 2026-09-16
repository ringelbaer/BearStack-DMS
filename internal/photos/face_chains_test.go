package photos

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/facerec"
)

func chainFixture(t *testing.T) (*Library, FaceChainSelection) {
	t.Helper()
	ctx := context.Background()
	paths := []string{"A/a.jpg", "B/b.jpg", "F/f.jpg", "Y/y.jpg", "Z/z.jpg", "Named/n.jpg", "Other/o.jpg"}
	l := faceLibrary(t, paths...)
	seedMatchingPeople(t, l, paths, [][]float32{faceDetection(0).Embedding, faceDetection(0).Embedding, faceDetection(1).Embedding, faceDetection(2).Embedding, faceDetection(3).Embedding, faceDetection(0).Embedding, faceDetection(4).Embedding}, []int{2, 1, 2, 2, 1, 2, 1})
	for id, axis := range map[int64]int{1: 0, 2: 1, 3: 0, 4: 1, 5: 2, 6: 2, 7: 3, 8: 3, 9: 0, 10: 4, 11: 4} {
		if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=?`, encodeVector(faceDetection(axis).Embedding), id); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.RenamePerson(ctx, 6, "Named"); err != nil {
		t.Fatal(err)
	}
	if err := l.SetFaceReferenceLimit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	chain, err := l.NextFaceChain(ctx, FaceChainSearch{Hops: 2})
	if err != nil {
		t.Fatal(err)
	}
	selection := FaceChainSelection{Dataset: chain.Dataset}
	for _, g := range chain.Groups {
		selection.Groups = append(selection.Groups, g.LabelGroupRef)
	}
	return l, selection
}

func TestFaceChainsHopsAllFacesAndNamedBoundary(t *testing.T) {
	l, _ := chainFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		hops int
		want string
	}{{1, "1:0 2:1 3:1 "}, {2, "1:0 2:1 3:1 4:2 "}, {3, "1:0 2:1 3:1 4:2 5:3 "}} {
		chain, err := l.NextFaceChain(ctx, FaceChainSearch{Hops: tc.hops})
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		for _, g := range chain.Groups {
			got += fmt.Sprintf("%d:%d ", g.ID, g.Depth)
		}
		if got != tc.want {
			t.Fatalf("hops=%d: %s want %s", tc.hops, got, tc.want)
		}
	}
	// With all seen groups skipped, the named bridge must not lead to group 7.
	chain, err := l.NextFaceChain(ctx, FaceChainSearch{Hops: 5, After: 1, ExcludedGroups: []int64{1, 2, 3, 4, 5}})
	if err != nil || len(chain.Groups) != 0 || chain.After != 7 {
		t.Fatalf("skip or named boundary: %+v %v", chain, err)
	}
	chain, err = l.NextFaceChain(ctx, FaceChainSearch{Hops: 2, After: chain.After})
	if err != nil || chain.HasMore || len(chain.Groups) != 0 {
		t.Fatalf("end: %+v %v", chain, err)
	}
	var suggestions int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_face_merge_suggestions`).Scan(&suggestions); err != nil || suggestions != 0 {
		t.Fatalf("search persisted proposals: %d %v", suggestions, err)
	}
}

func TestFaceChainsRespectEdgesVisibilityAndThreshold(t *testing.T) {
	for _, scenario := range []string{"rejected", "same photo", "ignored", "private", "model", "threshold"} {
		t.Run(scenario, func(t *testing.T) {
			l, _ := chainFixture(t)
			var stmt string
			switch scenario {
			case "rejected":
				stmt = `INSERT INTO photo_face_merge_suggestions(source_id,target_id,source_revision,target_revision,source_face_id,target_face_id,score,model,rejected) VALUES(1,3,1,1,2,4,1,'test',1)`
			case "same photo":
				stmt = `UPDATE photo_faces SET path='A/a.jpg' WHERE person_id=3`
			case "ignored":
				stmt = `UPDATE photo_faces SET ignored=1 WHERE person_id=3`
			case "model":
				stmt = `UPDATE photo_faces SET model='old' WHERE person_id=3`
			case "private":
				if err := os.WriteFile(filepath.Join(l.Root(), "F/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "threshold":
				v := make([]float32, facerec.Dimensions)
				v[1] = .5
				v[50] = .8660254
				if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=4`, encodeVector(v)); err != nil {
					t.Fatal(err)
				}
				thresholds := DefaultFaceThresholds()
				thresholds.SuggestionSimilarity = .6
				if err := l.SetFaceThresholds(context.Background(), thresholds); err != nil {
					t.Fatal(err)
				}
			}
			if stmt != "" {
				if _, err := l.index.db.Exec(stmt); err != nil {
					t.Fatal(err)
				}
			}
			chain, err := l.NextFaceChain(context.Background(), FaceChainSearch{Hops: 3})
			if err != nil || len(chain.Groups) != 2 || chain.Groups[1].ID != 2 {
				t.Fatalf("forbidden bridge: %+v %v", chain, err)
			}
		})
	}
}

func TestFaceChainsRequestSimilarity(t *testing.T) {
	l, _ := chainFixture(t)
	ctx := context.Background()
	v := make([]float32, facerec.Dimensions)
	v[1], v[50] = .5, .8660254
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=4`, encodeVector(v)); err != nil {
		t.Fatal(err)
	}
	thresholds := DefaultFaceThresholds()
	thresholds.SuggestionSimilarity = .6
	if err := l.SetFaceThresholds(ctx, thresholds); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		similarity float64
		groups     int
	}{{.6, 2}, {.49, 5}, {.51, 2}, {0, 6}, {1, 2}} {
		chain, err := l.NextFaceChain(ctx, FaceChainSearch{Hops: 3, Similarity: &tc.similarity})
		if err != nil || chain.Similarity != tc.similarity || len(chain.Groups) != tc.groups {
			t.Fatalf("similarity %g: %+v %v", tc.similarity, chain, err)
		}
	}
	chain, err := l.NextFaceChain(ctx, FaceChainSearch{Hops: 3})
	if err != nil || chain.Similarity != .6 || len(chain.Groups) != 2 {
		t.Fatalf("omitted similarity must retain the global default: %+v %v", chain, err)
	}
	if got, err := l.FaceThresholds(ctx); err != nil || got != thresholds {
		t.Fatalf("search changed global thresholds: %+v %v", got, err)
	}
	for _, value := range []float64{-.01, 1.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := l.NextFaceChain(ctx, FaceChainSearch{Hops: 2, Similarity: &value}); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("invalid similarity %g accepted: %v", value, err)
		}
	}
}

func TestFaceChainSelectionPagesAndAtomicAssignment(t *testing.T) {
	l, selection := chainFixture(t)
	ctx := context.Background()
	page, err := l.FaceChainFaces(ctx, FaceChainPageRequest{FaceChainSelection: selection, Page: 1})
	if err != nil || page.Total != 7 || len(page.Faces) != 7 || page.Faces[0].FolderName != "A" {
		t.Fatalf("page: %+v %v", page, err)
	}
	a := FaceChainAssignment{FaceChainSelection: selection, OperationID: "chain-test-operation-0001", ExcludedFaces: []int64{1}, TargetID: 6, TargetName: "Named"}
	receipt, err := l.AssignFaceChain(ctx, "tester", a)
	if err != nil || receipt.Faces != 6 || receipt.TargetID != 6 {
		t.Fatalf("assign: %+v %v", receipt, err)
	}
	replay, err := l.AssignFaceChain(ctx, "tester", a)
	if err != nil || replay != receipt {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	stored, err := l.LabelReceipt(ctx, "tester", a.OperationID, a.Dataset)
	if err != nil || stored != receipt {
		t.Fatalf("receipt: %+v %v", stored, err)
	}
	for _, id := range []int64{1, 9, 10, 11} {
		f, err := l.Face(ctx, id)
		want := map[int64]int64{1: 1, 9: 6, 10: 6, 11: 7}[id]
		if err != nil || f.PersonID != want || f.Manual {
			t.Fatalf("untouched face %d: %+v %v", id, f, err)
		}
	}
	for _, id := range []int64{2, 3, 4, 5, 6, 7} {
		f, err := l.Face(ctx, id)
		if err != nil || f.PersonID != 6 || !f.Manual {
			t.Fatalf("selected face: %+v %v", f, err)
		}
	}
	a.ExcludedFaces = nil
	if _, err := l.AssignFaceChain(ctx, "tester", a); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("changed replay accepted: %v", err)
	}
}

func TestFaceChainAssignmentConflictAndRollback(t *testing.T) {
	for _, scenario := range []string{"renamed source", "source revision", "target rename", "target revision", "private", "changed source", "deleted source", "all deselected", "foreign exclusion", "duplicate exclusion", "receipt rollback", "dataset", "new name"} {
		t.Run(scenario, func(t *testing.T) {
			l, selection := chainFixture(t)
			ctx := context.Background()
			a := FaceChainAssignment{FaceChainSelection: selection, OperationID: "chain-test-operation-0002", TargetID: 6, TargetName: "Named"}
			switch scenario {
			case "renamed source":
				if err := l.RenamePerson(ctx, 3, "Newly named"); err != nil {
					t.Fatal(err)
				}
			case "source revision":
				a.Groups[1].Revision++
			case "target rename":
				if err := l.RenamePerson(ctx, 6, "Changed"); err != nil {
					t.Fatal(err)
				}
			case "target revision":
				a.TargetRevision = 999999
			case "private":
				if err := os.WriteFile(filepath.Join(l.Root(), "Y/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "all deselected":
				a.ExcludedFaces = []int64{1, 2, 3, 4, 5, 6, 7}
			case "changed source":
				if err := os.WriteFile(filepath.Join(l.Root(), "Y/y.jpg"), []byte("replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			case "deleted source":
				if err := os.Remove(filepath.Join(l.Root(), "Y/y.jpg")); err != nil {
					t.Fatal(err)
				}
			case "foreign exclusion":
				a.ExcludedFaces = []int64{9}
			case "duplicate exclusion":
				a.ExcludedFaces = []int64{1, 1}
			case "receipt rollback":
				if _, err := l.index.db.Exec(`CREATE TRIGGER chain_fail BEFORE INSERT ON photo_labeling_actions BEGIN SELECT RAISE(ABORT,'fail'); END`); err != nil {
					t.Fatal(err)
				}
			case "dataset":
				a.Dataset = "00000000000000000000000000000000"
			case "new name":
				a.TargetID = 0
				a.TargetName = ""
				a.Name = "New person"
			}
			receipt, err := l.AssignFaceChain(ctx, "tester", a)
			if scenario == "new name" {
				if err != nil || receipt.TargetID <= 7 || receipt.Faces != 7 {
					t.Fatalf("new name: %+v %v", receipt, err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid assignment accepted")
			}
			f, err := l.Face(ctx, 1)
			if err != nil || f.PersonID != 1 || f.Manual {
				t.Fatalf("partial assignment: %+v %v", f, err)
			}
			var count int
			if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_labeling_actions`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("receipt on failure: %d %v", count, err)
			}
		})
	}
}

func TestFaceChainPaginationSelectionBeyondBatchLimit(t *testing.T) {
	l, selection := chainFixture(t)
	ctx := context.Background()
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<600)
 INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT f.path,f.directory,f.person_id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_faces f CROSS JOIN n WHERE f.id=1`); err != nil {
		t.Fatal(err)
	}
	if err := l.index.db.QueryRow(`SELECT revision FROM photo_person_revisions WHERE person_id=1`).Scan(&selection.Groups[0].Revision); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for page := 1; page <= 11; page++ {
		p, err := l.FaceChainFaces(ctx, FaceChainPageRequest{FaceChainSelection: selection, Page: page})
		if err != nil || p.Total != 607 || p.Pages != 11 {
			t.Fatalf("page %d: %+v %v", page, p, err)
		}
		for _, f := range p.Faces {
			if seen[f.ID] {
				t.Fatal("duplicate across pages")
			}
			seen[f.ID] = true
		}
	}
	if len(seen) != 607 {
		t.Fatalf("missing faces: %d", len(seen))
	}
	chain, err := l.NextFaceChain(ctx, FaceChainSearch{Hops: 2})
	if err != nil || len(chain.Groups) != 4 {
		t.Fatalf("source vector blocks: %+v %v", chain, err)
	}
	a := FaceChainAssignment{FaceChainSelection: selection, OperationID: "chain-large-selection-0001", ExcludedFaces: []int64{1, 6, 600}, TargetID: 6, TargetName: "Named"}
	r, err := l.AssignFaceChain(ctx, "tester", a)
	if err != nil || r.Faces != 604 {
		t.Fatalf("cross-page assignment: %+v %v", r, err)
	}
}

func TestFaceChainLimitNeverReturnsTruncatedSelection(t *testing.T) {
	l, _ := chainFixture(t)
	// Synthetic indexed sources are enough for read-only matching; every group
	// has a different path so the same-photo veto cannot mask the chain limit.
	for _, statement := range []string{
		`WITH RECURSIVE n(x) AS (VALUES(8) UNION ALL SELECT x+1 FROM n WHERE x<1008) INSERT INTO photo_people(id) SELECT x FROM n`,
		`INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at)
 SELECT 'synthetic-'||id||'.jpg','face.jpg','','photo','image/jpeg',1,1,'2026-09-16' FROM photo_people WHERE id>=8`,
		`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT 'synthetic-'||p.id||'.jpg','',p.id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_people p CROSS JOIN photo_faces f WHERE p.id>=8 AND f.id=1`,
	} {
		if _, err := l.index.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	chain, err := l.NextFaceChain(context.Background(), FaceChainSearch{Hops: 1})
	if !errors.Is(err, ErrFaceChainLarge) || len(chain.Groups) != 0 {
		t.Fatalf("truncated chain: %d %v", len(chain.Groups), err)
	}
}

func TestFaceChainRejectsChangedOriginalOnUnseenPage(t *testing.T) {
	l, selection := chainFixture(t)
	ctx := context.Background()
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<60)
 INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) SELECT f.path,f.directory,f.person_id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_faces f CROSS JOIN n WHERE f.id=1`); err != nil {
		t.Fatal(err)
	}
	if err := l.index.db.QueryRow(`SELECT revision FROM photo_person_revisions WHERE person_id=1`).Scan(&selection.Groups[0].Revision); err != nil {
		t.Fatal(err)
	}
	page, err := l.FaceChainFaces(ctx, FaceChainPageRequest{FaceChainSelection: selection, Page: 1})
	if err != nil || page.Total != 67 {
		t.Fatalf("page: %+v %v", page, err)
	}
	for _, f := range page.Faces {
		if f.Path == "Y/y.jpg" {
			t.Fatal("test must change a source beyond the viewed page")
		}
	}
	if err := os.Remove(filepath.Join(l.Root(), "Y/y.jpg")); err != nil {
		t.Fatal(err)
	}
	_, err = l.AssignFaceChain(ctx, "tester", FaceChainAssignment{FaceChainSelection: selection, OperationID: "unseen-original-change-0001", TargetID: 6, TargetName: "Named"})
	if !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("unseen change: %v", err)
	}
	f, err := l.Face(ctx, 1)
	if err != nil || f.PersonID != 1 || f.Manual {
		t.Fatalf("partial assignment: %+v %v", f, err)
	}
}

func TestFaceChainCanceledGateAndValidation(t *testing.T) {
	l, _ := chainFixture(t)
	for _, r := range []FaceChainSearch{{Hops: 0}, {Hops: 6}, {Hops: 2, After: -1}, {Hops: 2, ExcludedGroups: []int64{1, 1}}} {
		if _, err := l.NextFaceChain(context.Background(), r); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("invalid %+v: %v", r, err)
		}
	}
	l.faceChainGate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.NextFaceChain(ctx, FaceChainSearch{Hops: 2}); !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked cancellation: %v", err)
	}
	<-l.faceChainGate
}
