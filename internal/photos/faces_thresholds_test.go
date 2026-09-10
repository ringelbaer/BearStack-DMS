package photos

import (
	"context"
	"math"
	"testing"
)

func TestFaceThresholdsPersistenceAndValidation(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	defaults := DefaultFaceThresholds()
	if got, err := l.FaceThresholds(ctx); err != nil || got != defaults {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	for _, invalid := range []float64{.39, .71, math.NaN(), math.Inf(1)} {
		v := defaults
		v.AssignmentSimilarity = invalid
		if err := l.SetFaceThresholds(ctx, v); err == nil {
			t.Fatalf("accepted similarity %v", invalid)
		}
	}
	for _, invalid := range []float64{-.01, .21, math.NaN(), math.Inf(-1)} {
		v := defaults
		v.SuggestionMargin = invalid
		if err := l.SetFaceThresholds(ctx, v); err == nil {
			t.Fatalf("accepted margin %v", invalid)
		}
	}
	v := FaceThresholds{.4, 0, .7, .2, .6, .1}
	if err := l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.FaceThresholds(ctx); err != nil || got != v {
		t.Fatalf("persisted: %+v %v", got, err)
	}
}

func TestFaceThresholdsRebuildAndRejection(t *testing.T) {
	ctx := context.Background()
	l, _, _ := reconciliationFixture(t, .52, true)
	completeReconciliation(t, l)
	rows, err := l.FaceMergeSuggestions(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("initial: %+v %v", rows, err)
	}
	old := rows[0]
	v := DefaultFaceThresholds()
	v.SuggestionSimilarity = .6
	if err = l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	rows, err = l.FaceMergeSuggestions(ctx, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("stale cache: %+v %v", rows, err)
	}
	if err = l.AcceptFaceMergeSuggestion(ctx, old.ID, old.SourceRevision, old.TargetRevision); err == nil {
		t.Fatal("accepted invalidated suggestion")
	}
	completeReconciliation(t, l)
	rows, err = l.FaceMergeSuggestions(ctx, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("threshold ignored: %+v %v", rows, err)
	}
	v.SuggestionSimilarity = .4
	if err = l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	completeReconciliation(t, l)
	rows, err = l.FaceMergeSuggestions(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("lower threshold: %+v %v", rows, err)
	}
	r := rows[0]
	if err = l.RejectFaceMergeSuggestion(ctx, r.ID, r.SourceRevision, r.TargetRevision); err != nil {
		t.Fatal(err)
	}
	v.ReconcileSimilarity = .4
	v.ReconcileMargin = 0
	if err = l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	if state := completeReconciliation(t, l); state.Reassigned != 0 {
		t.Fatal("rejection lost")
	}
	rows, err = l.FaceMergeSuggestions(ctx, 10)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rejected pair returned: %+v %v", rows, err)
	}
	state, _ := l.FaceReconciliationStatus(ctx)
	if err = l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	if after, _ := l.FaceReconciliationStatus(ctx); after != state {
		t.Fatal("unchanged save scheduled a new scan")
	}
}

func TestFaceThresholdsAutomaticReconciliation(t *testing.T) {
	for _, threshold := range []float64{.5, .7} {
		l, source, target := reconciliationFixture(t, .6, true)
		v := DefaultFaceThresholds()
		v.ReconcileSimilarity = threshold
		if err := l.SetFaceThresholds(context.Background(), v); err != nil {
			t.Fatal(err)
		}
		completeReconciliation(t, l)
		got, err := l.Face(context.Background(), source.ID)
		if err != nil || (got.PersonID == target.PersonID) != (threshold == .5) {
			t.Fatalf("threshold %v: %+v %v", threshold, got, err)
		}
	}
}

func TestFaceThresholdsRecognitionAndMargin(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	seedMatchingPeople(t, l, []string{"a.jpg", "b.jpg"}, [][]float32{matchingVector(.6), matchingVector(.5)}, []int{1, 1})
	for _, tc := range []struct {
		similarity, margin float64
		want               int64
	}{{.55, .08, 1}, {.65, .08, 0}, {.55, .15, 0}, {.4, 0, 1}} {
		v := DefaultFaceThresholds()
		v.AssignmentSimilarity = tc.similarity
		v.AssignmentMargin = tc.margin
		if err := l.SetFaceThresholds(ctx, v); err != nil {
			t.Fatal(err)
		}
		got, err := l.nearestPerson(ctx, l.index.db, faceDetection(0).Embedding, nil)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: got %d %v", tc, got, err)
		}
	}
}

func TestFaceThresholdsReviewMargin(t *testing.T) {
	candidates := []facePersonCandidate{{score: .625}, {score: .5}, {score: .45}}
	for _, tc := range []struct {
		index  int
		margin float64
		want   bool
	}{{0, 0, true}, {1, 0, true}, {0, .125, true}, {0, .2, false}, {1, .1, false}} {
		if got := reviewCandidateAllowed(candidates, tc.index, tc.margin); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}

func TestFaceThresholdsMigrationPreservesExistingFaces(t *testing.T) {
	ctx := context.Background()
	l, source, _ := reconciliationFixture(t, .52, true)
	completeReconciliation(t, l)
	if _, err := l.index.db.Exec(`DROP TABLE photo_face_thresholds; UPDATE schema_migrations SET version=29 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.FaceThresholds(ctx); err != nil || got != DefaultFaceThresholds() {
		t.Fatalf("migrated defaults: %+v %v", got, err)
	}
	if got, err := reopened.Face(ctx, source.ID); err != nil || got.PersonID != source.PersonID {
		t.Fatalf("migration changed face: %+v %v", got, err)
	}
	if rows, err := reopened.FaceMergeSuggestions(ctx, 10); err != nil || len(rows) != 1 {
		t.Fatalf("migration lost proposals: %+v %v", rows, err)
	}
}

func TestFaceThresholdsNamedSearchMarginAndStreaming(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
	for i := 0; i < 3; i++ {
		finishFace(t, l, i)
	}
	a, _ := l.AutomaticFaces(ctx, "a.jpg")
	b, _ := l.AutomaticFaces(ctx, "b.jpg")
	c, _ := l.AutomaticFaces(ctx, "c.jpg")
	for i, f := range []RecognizedFace{b[0], c[0]} {
		if err := l.RenamePerson(ctx, f.PersonID, []string{"Near", "Far"}[i]); err != nil {
			t.Fatal(err)
		}
		setReconciliationVector(t, l, f.ID, []float64{.6, .5}[i])
	}
	for _, tc := range []struct {
		similarity, margin float64
		count              int
	}{{.45, 0, 2}, {.65, 0, 0}, {.45, .05, 1}, {.45, .15, 0}} {
		v := DefaultFaceThresholds()
		v.SuggestionSimilarity = tc.similarity
		v.SuggestionMargin = tc.margin
		if err := l.SetFaceThresholds(ctx, v); err != nil {
			t.Fatal(err)
		}
		completeReconciliation(t, l)
		if rows, err := l.FaceMergeSuggestions(ctx, 10); err != nil || len(rows) != tc.count {
			t.Fatalf("merge review %+v: %+v %v", tc, rows, err)
		}
		got, err := l.SuggestPeopleForFace(ctx, a[0].ID)
		if err != nil || len(got.People) != tc.count {
			t.Fatalf("search %+v: %+v %v", tc, got, err)
		}
		streamed, err := l.SuggestPeopleForFaceStream(ctx, a[0].ID, func(part PeopleSuggestions) error {
			if tc.margin > 0 {
				t.Fatal("emitted unproven partial margin")
			}
			return nil
		})
		if err != nil || len(streamed.People) != tc.count {
			t.Fatalf("stream %+v: %+v %v", tc, streamed, err)
		}
	}
}
