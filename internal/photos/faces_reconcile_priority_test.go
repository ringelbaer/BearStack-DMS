package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func reconciliationPriorityFixture(t *testing.T, scores []float32, named []int64, margin float64) *Library {
	t.Helper()
	paths := []string{"source/query.jpg"}
	vectors := [][]float32{matchingVector(1)}
	counts := []int{1}
	for i, score := range scores {
		paths = append(paths, fmt.Sprintf("target-%d/photo.jpg", i+2))
		vectors = append(vectors, matchingVector(score))
		counts = append(counts, 1)
	}
	l := faceLibrary(t, paths...)
	seedMatchingPeople(t, l, paths, vectors, counts)
	// Only the query is eligible as a source; targets remain valid references.
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET manual=1 WHERE person_id<>1`); err != nil {
		t.Fatal(err)
	}
	for _, id := range named {
		if err := l.RenamePerson(context.Background(), id, fmt.Sprintf("Named %d", id)); err != nil {
			t.Fatal(err)
		}
	}
	thresholds := DefaultFaceThresholds()
	thresholds.SuggestionMargin = margin
	if err := l.SetFaceThresholds(context.Background(), thresholds); err != nil {
		t.Fatal(err)
	}
	if err := l.ScheduleFaceReconciliation(context.Background()); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestFaceReconciliationNamedSuggestionPriority(t *testing.T) {
	for _, tc := range []struct {
		name   string
		scores []float32
		named  []int64
		margin float64
		want   []int64
	}{
		{"named before three stronger unnamed", []float32{.52, .9, .8, .7}, []int64{2}, 0, []int64{2}},
		{"review priority does not trigger automatic reassignment", []float32{.65, .95}, []int64{2}, 0, []int64{2}},
		{"named fill limit in score order", []float32{.5, .6, .55, .95}, []int64{2, 3, 4}, 0, []int64{3, 4, 2}},
		{"weak named falls back", []float32{.4, .6, .55}, []int64{2}, 0, []int64{3, 4}},
		{"ambiguous named falls back", []float32{.55, .54, .85, .6}, []int64{2, 3}, .1, []int64{4}},
		{"margin within each scope", []float32{.6, .45, .61, .4}, []int64{2, 3}, .1, []int64{2}},
		{"below threshold runner up counts", []float32{.5, .44, .9}, []int64{2, 3}, .1, []int64{4}},
		{"named ties use stable IDs", []float32{.52, .52, .9}, []int64{2, 3}, 0, []int64{2, 3}},
		{"only unnamed", []float32{.8, .7, .6, .5}, nil, 0, []int64{2, 3, 4}},
		{"all below threshold", []float32{.3, .2}, []int64{2}, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := reconciliationPriorityFixture(t, tc.scores, tc.named, tc.margin)
			if state := completeReconciliation(t, l); state.Reassigned != 0 {
				t.Fatalf("suggestion priority changed automatic assignment: %+v", state)
			}
			rows, err := l.FaceMergeSuggestions(context.Background(), 100)
			if err != nil {
				t.Fatal(err)
			}
			var got []int64
			for _, row := range rows {
				if row.SourceID != 1 {
					t.Fatalf("unexpected source: %+v", row)
				}
				got = append(got, row.TargetID)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("targets=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestFaceMergeNamedPriorityUsesScoreIndex(t *testing.T) {
	l := reconciliationPriorityFixture(t, []float32{.52, .9}, []int64{2}, 0)
	completeReconciliation(t, l)
	for _, named := range []bool{true, false} {
		rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN `+faceMergeSuggestionSelect+faceMergeSuggestionPriorityOrder, named, 60)
		if err != nil {
			t.Fatal(err)
		}
		var plan string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			plan += detail + "\n"
		}
		err = rows.Err()
		rows.Close()
		if err != nil || !strings.Contains(plan, "idx_face_merge_suggestions_visible") || strings.Contains(plan, "TEMP B-TREE") {
			t.Fatalf("named=%v: suggestion order lost its index: %s (%v)", named, plan, err)
		}
	}
}

func TestFaceReconciliationSkipsUnnamedValidationAfterNamedMatch(t *testing.T) {
	ctx := context.Background()
	l := reconciliationPriorityFixture(t, []float32{.52, .9}, []int64{2}, 0)
	prepareReferenceGraph(t, l)
	ranked, err := l.faceRuntime.rankFacePersons(ctx, matchingVector(1), map[int64]bool{1: true})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	q := &countedFaceMatchQuery{tx: tx}
	got, err := l.faceReconciliationSuggestions(ctx, q, 1, matchingVector(1), ranked, map[int64]bool{2: true})
	// One exclusion lookup and one bounded named-reference validation.
	if err != nil || len(got) != 1 || got[0].person != 2 || q.calls != 2 {
		t.Fatalf("candidates=%+v queries=%d error=%v", got, q.calls, err)
	}
}

func TestFaceReconciliationNamedPriorityValidatesReferences(t *testing.T) {
	for _, change := range []string{"private", "ignored", "quality", "folder exclusion", "same photo", "same group photo"} {
		t.Run(change, func(t *testing.T) {
			l := reconciliationPriorityFixture(t, []float32{.6, .52, .9}, []int64{2, 3}, 0)
			var statement string
			switch change {
			case "private":
				if err := os.WriteFile(filepath.Join(l.Root(), "target-2/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "ignored":
				statement = `UPDATE photo_faces SET ignored=1 WHERE person_id=2`
			case "quality":
				statement = `UPDATE photo_faces SET reference_eligible=0 WHERE person_id=2`
			case "folder exclusion":
				statement = `INSERT INTO person_folder_exclusions(person_id,directory) VALUES(2,'source')`
			case "same photo":
				statement = `UPDATE photo_faces SET path='source/query.jpg',directory='source' WHERE person_id=2`
			case "same group photo":
				statement = `INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,manual)
 SELECT 'target-2/photo.jpg','target-2',person_id,.6,.1,.2,.2,confidence,embedding,model,1 FROM photo_faces WHERE person_id=1`
			}
			if statement != "" {
				if _, err := l.index.db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			completeReconciliation(t, l)
			rows, err := l.FaceMergeSuggestions(context.Background(), 100)
			if err != nil || len(rows) != 1 || rows[0].TargetID != 3 {
				t.Fatalf("invalid named target displaced valid targets: %+v %v", rows, err)
			}
		})
	}
}

func TestFaceMergeNamedPriorityAppliesBeforeLimitAndExclusion(t *testing.T) {
	ctx := context.Background()
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("named source=%v", reverse), func(t *testing.T) {
			l := reconciliationPriorityFixture(t, []float32{.52, .9}, nil, 0)
			completeReconciliation(t, l)
			// Existing unnamed pairs remain cached when another target is named.
			if err := l.RenamePerson(ctx, 2, "Named"); err != nil {
				t.Fatal(err)
			}
			completeReconciliation(t, l)
			if reverse {
				if _, err := l.index.db.Exec(`UPDATE photo_face_merge_suggestions SET source_id=target_id,target_id=source_id,
 source_revision=target_revision,target_revision=source_revision,source_face_id=target_face_id,target_face_id=source_face_id WHERE target_id=2`); err != nil {
					t.Fatal(err)
				}
			}
			rows, err := l.FaceMergeSuggestions(ctx, 1)
			if err != nil || len(rows) != 1 || (rows[0].SourceID != 2 && rows[0].TargetID != 2) {
				t.Fatalf("web priority before limit: %+v %v", rows, err)
			}
			next, err := l.LabelNextMergeSuggestion(ctx)
			if err != nil || next == nil || next.ID != rows[0].ID {
				t.Fatalf("native priority: %+v %v", next, err)
			}
			next, err = l.LabelNextMergeSuggestion(ctx, [2]int64{1, 2})
			if err != nil || next == nil || next.Source.ID != 1 || next.Target.ID != 3 {
				t.Fatalf("excluded named pair must allow unnamed fallback: %+v %v", next, err)
			}
			if err := l.RejectFaceMergeSuggestion(ctx, rows[0].ID, rows[0].SourceRevision, rows[0].TargetRevision); err != nil {
				t.Fatal(err)
			}
			completeReconciliation(t, l)
			next, err = l.LabelNextMergeSuggestion(ctx)
			if err != nil || next == nil || next.Target.ID != 3 {
				t.Fatalf("rejected named pair returned: %+v %v", next, err)
			}
		})
	}
}
