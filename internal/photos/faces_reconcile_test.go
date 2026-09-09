package photos

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/facerec"
)

func reconciliationFixture(t *testing.T, score float64, named bool) (*Library, RecognizedFace, RecognizedFace) {
	t.Helper()
	l := faceLibrary(t, "target/a.jpg", "source/b.jpg")
	// Queue order is lexical: source first, then target.
	j := finishFace(t, l, 1)
	source, err := l.AutomaticFaces(context.Background(), j.Path)
	if err != nil {
		t.Fatal(err)
	}
	j = finishFace(t, l, 0)
	target, err := l.AutomaticFaces(context.Background(), j.Path)
	if err != nil {
		t.Fatal(err)
	}
	if named {
		if err := l.RenamePerson(context.Background(), target[0].PersonID, "Confirmed"); err != nil {
			t.Fatal(err)
		}
	}
	setReconciliationVector(t, l, source[0].ID, score)
	return l, source[0], target[0]
}

func setReconciliationVector(t *testing.T, l *Library, id int64, score float64) {
	t.Helper()
	v := faceDetection(0).Embedding
	v[0], v[1] = float32(score), float32(math.Sqrt(1-score*score))
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=?; UPDATE photo_face_state SET revision=revision+1 WHERE id=1`, encodeVector(v), id); err != nil {
		t.Fatal(err)
	}
	if err := l.ScheduleFaceReconciliation(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func completeReconciliation(t *testing.T, l *Library) FaceReconciliationProgress {
	t.Helper()
	for range 30 {
		state, err := l.ReconcileFacesBatch(context.Background(), 100)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Pending {
			return state
		}
	}
	t.Fatal("reconciliation did not finish")
	return FaceReconciliationProgress{}
}

func TestFaceReconciliationMovesOnlyUnconfirmedFaces(t *testing.T) {
	for _, protect := range []string{"none", "manual", "favorite", "ignored", "manual group", "named group", "weak source", "other model", "unconfirmed target", "same photo", "private source", "private target", "XMP conflict"} {
		t.Run(protect, func(t *testing.T) {
			l, source, target := reconciliationFixture(t, .85, true)
			var statement string
			var arg int64 = source.ID
			switch protect {
			case "manual":
				statement = `UPDATE photo_faces SET manual=1 WHERE id=?`
			case "favorite":
				statement = `UPDATE photo_faces SET favorite=1 WHERE id=?`
			case "ignored":
				statement = `UPDATE photo_faces SET ignored=1 WHERE id=?`
			case "manual group":
				statement, arg = `UPDATE photo_people SET manual_name=1 WHERE id=?`, source.PersonID
			case "named group":
				statement, arg = `UPDATE photo_people SET name='Already named' WHERE id=?`, source.PersonID
			case "weak source":
				statement = `UPDATE photo_faces SET reference_eligible=0 WHERE id=?`
			case "other model":
				statement = `UPDATE photo_faces SET model='obsolete' WHERE id=?`
			case "unconfirmed target":
				statement, arg = `UPDATE photo_people SET manual_name=0 WHERE id=?`, target.PersonID
			case "same photo":
				_, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT ?,?,person_id,.6,.1,.2,.2,confidence,embedding,model FROM photo_faces WHERE id=?`, source.Path, parentPath(source.Path), target.ID)
				if err != nil {
					t.Fatal(err)
				}
			case "XMP conflict":
				faces, _ := json.Marshal([]Face{{Name: "Imported ambiguous name", X: source.X, Y: source.Y, Width: source.Width, Height: source.Height}})
				if _, err := l.index.db.Exec(`UPDATE media_index SET faces=? WHERE path=?`, string(faces), source.Path); err != nil {
					t.Fatal(err)
				}
			case "private source", "private target":
				path := source.Path
				if protect == "private target" {
					path = target.Path
				}
				if err := os.WriteFile(filepath.Join(l.Root(), parentPath(path), ".adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if statement != "" {
				if _, err := l.index.db.Exec(statement, arg); err != nil {
					t.Fatal(err)
				}
			}
			state := completeReconciliation(t, l)
			want := int64(0)
			if protect == "none" {
				want = 1
			}
			if state.Reassigned != want {
				t.Fatalf("reassigned=%d want=%d", state.Reassigned, want)
			}
			if protect == "none" {
				face, err := l.Face(context.Background(), source.ID)
				if err != nil || face.PersonID != target.PersonID || face.Manual {
					t.Fatalf("moved face=%+v err=%v", face, err)
				}
				var queued int
				if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_face_jobs WHERE status<>'done'`).Scan(&queued); err != nil || queued != 0 {
					t.Fatalf("reassociation scheduled inference: %d %v", queued, err)
				}
			}
		})
	}
}

func TestFaceReconciliationSuggestionWitnessStillBelongsToTarget(t *testing.T) {
	l, source, target := reconciliationFixture(t, .52, true)
	tx, err := l.index.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	item := faceSuggestionEvidence{source: source.PersonID, target: target.PersonID, face: source.ID, targetFace: target.ID, score: .52}
	if _, err := tx.Exec(`UPDATE photo_faces SET person_id=? WHERE id=?`, source.PersonID, target.ID); err != nil {
		t.Fatal(err)
	}
	added, err := cacheFaceMergeSuggestion(context.Background(), tx, item, facerec.Model)
	if err != nil || added {
		t.Fatalf("stale matching witness cached: %v %v", added, err)
	}
}

func TestFaceReconciliationDoesNotRefillPrivateSuggestionPage(t *testing.T) {
	l := faceLibrary(t, "a/a.jpg", "b/b.jpg", "c/c.jpg", "d/d.jpg")
	var faces []RecognizedFace
	for i := range 4 {
		j := finishFace(t, l, i)
		f, err := l.AutomaticFaces(context.Background(), j.Path)
		if err != nil {
			t.Fatal(err)
		}
		faces = append(faces, f[0])
	}
	tx, err := l.index.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		source, target := faces[i*2], faces[i*2+1]
		added, err := cacheFaceMergeSuggestion(context.Background(), tx, faceSuggestionEvidence{source: source.PersonID, target: target.PersonID, face: source.ID, targetFace: target.ID, score: .8 - float64(i)*.1}, facerec.Model)
		if err != nil || !added {
			t.Fatalf("fixture candidate missing: %v %v", added, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"a", "c"} {
		if err := os.WriteFile(filepath.Join(l.Root(), directory, ".adminonly"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := l.FaceMergeSuggestions(context.Background(), 1)
	if err != nil || len(rows) != 0 {
		t.Fatalf("private replacement candidate leaked: %+v %v", rows, err)
	}
}

func TestFaceReconciliationSuggestionsAcceptRejectAndRevisions(t *testing.T) {
	for _, named := range []bool{false, true} {
		t.Run(map[bool]string{false: "unnamed pair", true: "named target"}[named], func(t *testing.T) {
			l, source, target := reconciliationFixture(t, .52, named)
			completeReconciliation(t, l)
			suggestions, err := l.FaceMergeSuggestions(context.Background(), 100)
			if err != nil || len(suggestions) != 1 {
				t.Fatalf("suggestions=%+v err=%v", suggestions, err)
			}
			s := suggestions[0]
			if math.Abs(s.Score-.52) > .0001 {
				t.Fatalf("score=%f", s.Score)
			}
			if err := l.AcceptFaceMergeSuggestion(context.Background(), s.ID, s.SourceRevision, s.TargetRevision); err != nil {
				t.Fatal(err)
			}
			a, err := l.Face(context.Background(), source.ID)
			if err != nil {
				t.Fatal(err)
			}
			b, err := l.Face(context.Background(), target.ID)
			if err != nil || a.PersonID != b.PersonID {
				t.Fatalf("accepted merge mismatch: %+v %+v %v", a, b, err)
			}
			if _, err := l.FaceMergeSuggestions(context.Background(), 100); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("stale and rejected", func(t *testing.T) {
		l, source, target := reconciliationFixture(t, .52, true)
		completeReconciliation(t, l)
		rows, _ := l.FaceMergeSuggestions(context.Background(), 10)
		s := rows[0]
		if err := l.RenamePerson(context.Background(), target.PersonID, "Changed"); err != nil {
			t.Fatal(err)
		}
		if err := l.AcceptFaceMergeSuggestion(context.Background(), s.ID, s.SourceRevision, s.TargetRevision); !errors.Is(err, ErrLabelConflict) {
			t.Fatalf("stale accept: %v", err)
		}
		if err := l.RejectFaceMergeSuggestion(context.Background(), s.ID, s.SourceRevision, s.TargetRevision); !errors.Is(err, ErrLabelConflict) {
			t.Fatalf("stale reject: %v", err)
		}
		completeReconciliation(t, l)
		rows, _ = l.FaceMergeSuggestions(context.Background(), 10)
		s = rows[0]
		if err := l.RejectFaceMergeSuggestion(context.Background(), s.ID, s.SourceRevision, s.TargetRevision); err != nil {
			t.Fatal(err)
		}
		setReconciliationVector(t, l, source.ID, .95)
		if _, err := l.SetFaceFavorite(context.Background(), target.ID, target.PersonID, true); err != nil {
			t.Fatal(err)
		}
		if err := l.RenamePerson(context.Background(), target.PersonID, "Renamed again"); err != nil {
			t.Fatal(err)
		}
		state := completeReconciliation(t, l)
		if state.Reassigned != 0 {
			t.Fatal("rejected identity pair moved automatically")
		}
		rows, err := l.FaceMergeSuggestions(context.Background(), 10)
		if err != nil || len(rows) != 0 {
			t.Fatalf("rejected suggestion returned: %+v %v", rows, err)
		}
	})
}

func TestFaceReconciliationResumesAndCoalesces(t *testing.T) {
	l, _, _ := reconciliationFixture(t, .52, true)
	ctx := context.Background()
	first, err := l.ReconcileFacesBatch(ctx, 1)
	if err != nil || !first.Pending || first.Processed != 1 || first.Cursor == 0 {
		t.Fatalf("first batch=%+v err=%v", first, err)
	}
	for range 4 {
		if err := l.ScheduleFaceReconciliation(ctx); err != nil {
			t.Fatal(err)
		}
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	resumed, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	status, err := resumed.FaceReconciliationStatus(ctx)
	if err != nil || status.Cursor != first.Cursor || status.Upper != first.Upper {
		t.Fatalf("restart lost cursor: %+v %v", status, err)
	}
	second, err := resumed.ReconcileFacesBatch(ctx, 1)
	if err != nil || second.Processed != 2 || second.Cursor != first.Upper || !second.Pending {
		t.Fatalf("dirty pass did not finish original cursor: %+v %v", second, err)
	}
	final := completeReconciliation(t, resumed)
	if final.Processed != 2 {
		t.Fatalf("dirty requests not coalesced: %+v", final)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := resumed.ReconcileFacesBatch(canceled, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestFaceReconciliationRollsBackAndRestarts(t *testing.T) {
	l, source, _ := reconciliationFixture(t, .9, true)
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_reassociation BEFORE UPDATE OF person_id ON photo_faces WHEN new.person_id<>old.person_id BEGIN SELECT RAISE(ABORT,'injected reassociation failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReconcileFacesBatch(context.Background(), 100); err == nil {
		t.Fatal("expected write failure")
	}
	state, err := l.FaceReconciliationStatus(context.Background())
	if err != nil || state.Cursor != 0 || state.Reassigned != 0 || state.Processed != 0 {
		t.Fatalf("failed transaction advanced progress: %+v %v", state, err)
	}
	face, err := l.Face(context.Background(), source.ID)
	if err != nil || face.PersonID != source.PersonID {
		t.Fatalf("failed transaction moved source: %+v %v", face, err)
	}
	if _, err := l.index.db.Exec(`DROP TRIGGER fail_reassociation`); err != nil {
		t.Fatal(err)
	}
	if state := completeReconciliation(t, l); state.Reassigned != 1 {
		t.Fatalf("retry failed: %+v", state)
	}
}

func TestFaceReconciliationPrivateSuggestionsAndClear(t *testing.T) {
	for _, action := range []string{"list", "accept", "reject"} {
		t.Run(action, func(t *testing.T) {
			l, source, _ := reconciliationFixture(t, .52, true)
			completeReconciliation(t, l)
			rows, err := l.FaceMergeSuggestions(context.Background(), 10)
			if err != nil || len(rows) != 1 {
				t.Fatalf("missing fixture suggestion: %+v %v", rows, err)
			}
			s := rows[0]
			if err := os.WriteFile(filepath.Join(l.Root(), parentPath(source.Path), ".adminonly"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "list":
				rows, err = l.FaceMergeSuggestions(context.Background(), 10)
				if err != nil || len(rows) != 0 {
					t.Fatalf("private suggestion visible: %+v %v", rows, err)
				}
			case "accept":
				if err := l.AcceptFaceMergeSuggestion(context.Background(), s.ID, s.SourceRevision, s.TargetRevision); !errors.Is(err, ErrLabelConflict) {
					t.Fatalf("private accept: %v", err)
				}
			case "reject":
				if err := l.RejectFaceMergeSuggestion(context.Background(), s.ID, s.SourceRevision, s.TargetRevision); !errors.Is(err, ErrLabelConflict) {
					t.Fatalf("private reject: %v", err)
				}
			}
			if err := l.ClearFaces(context.Background()); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_face_merge_suggestions`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("clear retained suggestions: %d %v", count, err)
			}
		})
	}
}
