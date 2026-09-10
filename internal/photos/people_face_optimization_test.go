package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"bearstack/internal/facerec"
)

func namedSuggestionFixture(t *testing.T) *Library {
	t.Helper()
	l := faceLibrary(t, "source.jpg", "A/one.jpg", "B/two.jpg", "C/three.jpg")
	seedMatchingPeople(t, l, []string{"source.jpg", "A/one.jpg", "B/two.jpg", "C/three.jpg"}, [][]float32{faceDetection(0).Embedding, matchingVector(.9), matchingVector(.8), matchingVector(.7)}, []int{1, 3, 3, 3})
	for id := int64(2); id <= 4; id++ {
		if err := l.RenamePerson(context.Background(), id, fmt.Sprint("Person ", id)); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func TestFaceCandidateValidationStartsWithIDs(t *testing.T) {
	l := faceSuggestionPerfFixture(t, 100)
	for _, analyzed := range []bool{false, true} {
		if analyzed {
			if _, err := l.index.db.Exec("ANALYZE"); err != nil {
				t.Fatal(err)
			}
		}
		args := make([]any, 512)
		for i := range args {
			args[i] = i + 1
		}
		rows, err := l.index.db.Query("EXPLAIN QUERY PLAN "+faceCandidateValidationSQL(len(args)), args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var a, b, c int
			var detail string
			if err := rows.Scan(&a, &b, &c, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(plan) == 0 || !strings.Contains(plan[0], "SEARCH f USING INTEGER PRIMARY KEY") || strings.Contains(strings.Join(plan, "\n"), "SCAN m") {
			t.Fatalf("analyzed=%t: %v", analyzed, plan)
		}
	}
}

func TestNamedFaceSnapshotReusesUnchangedGroups(t *testing.T) {
	l := namedSuggestionFixture(t)
	ctx := context.Background()
	first, err := l.namedFaceReferences(ctx, facerec.Model)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.groups) != 3 || len(first.faces[1]) != 0 {
		t.Fatalf("unnamed source cached: %+v", first.groups)
	}
	warm, err := l.namedFaceReferences(ctx, facerec.Model)
	if err != nil || warm != first {
		t.Fatalf("warm cache rebuilt: %v", err)
	}
	if err := l.RenamePerson(ctx, 2, "Changed"); err != nil {
		t.Fatal(err)
	}
	second, err := l.namedFaceReferences(ctx, facerec.Model)
	if err != nil {
		t.Fatal(err)
	}
	if &second.faces[3][0] != &first.faces[3][0] || second.faces[2][0].revision == first.faces[2][0].revision {
		t.Fatal("unchanged group copied or changed group stale")
	}
	if _, err := l.SetFaceFavorite(ctx, 4, 2, true); err != nil {
		t.Fatal(err)
	}
	third, err := l.namedFaceReferences(ctx, facerec.Model)
	if err != nil {
		t.Fatal(err)
	}
	if &third.faces[3][0] != &second.faces[3][0] || third.faces[2][0].revision == second.faces[2][0].revision {
		t.Fatal("favorite did not invalidate only its group")
	}
	if err := l.RenamePerson(ctx, 1, "Now named"); err != nil {
		t.Fatal(err)
	}
	next, err := l.namedFaceReferences(ctx, facerec.Model)
	if err != nil || len(next.faces[1]) != 1 {
		t.Fatalf("newly named group missing: %v", err)
	}
	if err := l.RenamePerson(ctx, 2, ""); err != nil {
		t.Fatal(err)
	}
	next, err = l.namedFaceReferences(ctx, facerec.Model)
	if err != nil || len(next.faces[2]) != 0 {
		t.Fatalf("unnamed group retained: %v", err)
	}
	if len(first.groups) != 3 || first.faces[2][0].revision == third.faces[2][0].revision {
		t.Fatal("published snapshot mutated")
	}
}

func TestNamedPendingReferencesMatchGlobalSelection(t *testing.T) {
	l := namedSuggestionFixture(t)
	ctx := context.Background()
	// One group spans folders, with differing trust, quality and confidence.
	if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,manual,favorite,reference_eligible)
 SELECT path,directory,2,x,y,width,height,.5,embedding,model,1,0,1 FROM photo_faces WHERE person_id IN(3,4);
 UPDATE photo_faces SET favorite=1 WHERE id IN(3,4,12);
 UPDATE photo_faces SET reference_eligible=0 WHERE id IN(2,3,13);
 UPDATE photo_faces SET ignored=1 WHERE id=14;
 UPDATE photo_faces SET drawn=1 WHERE id=15;
 UPDATE photo_face_state SET revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{1, 4, 100} {
		if err := l.SetFaceReferenceLimit(ctx, limit); err != nil {
			t.Fatal(err)
		}
		snapshot, err := l.namedFaceReferences(ctx, facerec.Model)
		if err != nil {
			t.Fatal(err)
		}
		var pending bool
		if err := l.index.db.QueryRow(`SELECT pending FROM photo_face_reference_settings WHERE id=1`).Scan(&pending); err != nil || !pending {
			t.Fatalf("modal performed global rebuild: %v", err)
		}
		prepareReferenceGraph(t, l)
		for _, g := range snapshot.groups {
			want := slices.Clone(l.faceRuntime.nodes[g.id])
			slices.Sort(want)
			var got []int64
			for _, ref := range snapshot.faces[g.id] {
				got = append(got, ref.id)
			}
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Fatalf("limit=%d person=%d got=%v want=%v", limit, g.id, got, want)
			}
		}
	}
}

func TestFaceSuggestionCacheWaitIsCancellable(t *testing.T) {
	l := namedSuggestionFixture(t)
	loading := make(chan struct{})
	l.faceSuggestions.loading = loading
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := l.namedFaceReferences(ctx, facerec.Model); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cache wait ignored cancellation")
	}
	l.faceSuggestions.loading = nil
	close(loading)
	// A worker holding its reference lock must not block a cold modal cache.
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := l.SuggestPeopleForFace(ctx, 1); err != nil {
		t.Fatal(err)
	}
}

func TestFaceSuggestionStreamDoesNotHoldMutationLock(t *testing.T) {
	l := namedSuggestionFixture(t)
	ctx := context.Background()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		once := false
		_, err := l.SuggestPeopleForFaceStream(ctx, 1, func(PeopleSuggestions) error {
			if !once {
				once = true
				close(entered)
				<-release
			}
			return nil
		})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("no first hit")
	}
	wait, cancel := context.WithTimeout(ctx, time.Second)
	_, err := l.SuggestPeopleForFace(wait, 1)
	cancel()
	if err != nil {
		close(release)
		<-done
		t.Fatalf("parallel search blocked: %v", err)
	}
	wait, cancel = context.WithTimeout(ctx, time.Second)
	_, err = l.SetFaceFavorite(wait, 2, 2, true)
	cancel()
	close(release)
	streamErr := <-done
	if err != nil {
		t.Fatalf("edit blocked by slow stream: %v", err)
	}
	if streamErr != nil && !errors.Is(streamErr, ErrLabelConflict) {
		t.Fatal(streamErr)
	}
}

func TestFaceSuggestionChangesDuringStream(t *testing.T) {
	for _, change := range []string{"source_move", "source_ignore", "source_private", "source_deleted", "source_clear", "candidate_move", "candidate_private", "threshold"} {
		t.Run(change, func(t *testing.T) {
			l := namedSuggestionFixture(t)
			ctx := context.Background()
			changed := false
			out, err := l.SuggestPeopleForFaceStream(ctx, 1, func(PeopleSuggestions) error {
				if changed {
					return nil
				}
				changed = true
				switch change {
				case "source_move":
					return l.EditFaces(ctx, []int64{1}, 3, false, "")
				case "source_ignore":
					return l.EditFaces(ctx, []int64{1}, 0, true, "")
				case "source_private":
					return os.WriteFile(filepath.Join(l.Root(), AdminOnlyMarkerName), nil, 0600)
				case "source_deleted":
					_, err := l.index.db.Exec(`DELETE FROM photo_faces WHERE id=1`)
					return err
				case "source_clear":
					return l.ClearFaces(ctx)
				case "candidate_move":
					return l.EditFaces(ctx, []int64{2}, 3, false, "")
				case "candidate_private":
					return os.WriteFile(filepath.Join(l.Root(), "A", AdminOnlyMarkerName), nil, 0600)
				case "threshold":
					v := DefaultFaceThresholds()
					v.SuggestionSimilarity = .7
					return l.SetFaceThresholds(ctx, v)
				}
				return nil
			})
			if !changed {
				t.Fatal("no intermediate result")
			}
			if change == "source_clear" && l.faceSuggestions.snapshot != nil {
				t.Fatal("clearing faces retained named reference cache")
			}
			if strings.HasPrefix(change, "source_") || change == "threshold" {
				if err == nil {
					t.Fatalf("obsolete source succeeded: %+v", out)
				}
			} else if err == nil {
				for _, p := range out.People {
					if p.ID == 2 {
						t.Fatalf("stale/private candidate: %+v", out)
					}
				}
			} else if !errors.Is(err, ErrLabelConflict) && !errors.Is(err, sql.ErrNoRows) {
				t.Fatal(err)
			}
		})
	}
}

func TestNamedFaceCacheClearDoesNotRepublishInflightLoad(t *testing.T) {
	l := namedSuggestionFixture(t)
	l.index.db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := l.index.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan error, 1)
	go func() { _, err := l.namedFaceReferences(ctx, facerec.Model); done <- err }()
	for {
		l.faceSuggestions.mu.Lock()
		loading := l.faceSuggestions.loading != nil
		l.faceSuggestions.mu.Unlock()
		if loading {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	l.faceSuggestions.clear()
	conn.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if l.faceSuggestions.snapshot != nil {
		t.Fatal("old load republished after cache clear")
	}
	if _, err := l.namedFaceReferences(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	if l.faceSuggestions.snapshot == nil {
		t.Fatal("new load could not publish")
	}
}

func TestFaceSuggestionMarginIncludesBelowThresholdRunnerUp(t *testing.T) {
	l := namedSuggestionFixture(t)
	ctx := context.Background()
	for person, score := range map[int]float32{2: .46, 3: .44, 4: .1} {
		if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE person_id=?`, encodeVector(matchingVector(score)), person); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.index.db.Exec(`UPDATE photo_face_state SET revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	v := DefaultFaceThresholds()
	v.SuggestionMargin = .05
	if err := l.SetFaceThresholds(ctx, v); err != nil {
		t.Fatal(err)
	}
	out, err := l.SuggestPeopleForFaceStream(ctx, 1, func(PeopleSuggestions) error { t.Error("unproven partial margin"); return nil })
	if err != nil || len(out.People) != 0 {
		t.Fatalf("runner-up discarded: %+v %v", out, err)
	}
}

func TestFaceSuggestionStreamFinalMatchesJSON(t *testing.T) {
	l := faceSuggestionPerfFixture(t, 100)
	ctx := context.Background()
	want, err := l.SuggestPeopleForFace(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	updates := 0
	got, err := l.SuggestPeopleForFaceStream(ctx, 1, func(p PeopleSuggestions) error {
		updates++
		if len(p.People) > 20 {
			t.Error("unbounded ranking")
		}
		return nil
	})
	if err != nil || !reflect.DeepEqual(got, want) || updates > 1+int(time.Since(start)/faceSuggestionUpdateInterval) {
		t.Fatalf("updates=%d got=%+v want=%+v err=%v", updates, got, want, err)
	}
}
