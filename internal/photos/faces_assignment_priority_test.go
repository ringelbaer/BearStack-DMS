package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/facerec"
)

func TestFaceRecognitionNamedPriority(t *testing.T) {
	for _, favorite := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			scores []float32
			named  []int64
			margin float64
			want   int64 // Zero means a new group must be created.
		}{
			{"named before stronger unnamed", []float32{.65, .95}, []int64{1}, .08, 1},
			{"named ignores close unnamed", []float32{.65, .66}, []int64{1}, .08, 1},
			{"named tie with unnamed", []float32{.65, .65}, []int64{2}, .08, 2},
			{"named margin within named groups", []float32{.7, .6, .99}, []int64{1, 2}, .08, 1},
			{"named exact margin boundary", []float32{.625, .5, .99}, []int64{1, 2}, .125, 1},
			{"ambiguous named falls back", []float32{.7, .68, .6}, []int64{1, 2}, .08, 3},
			{"weak named falls back", []float32{.54, .6}, []int64{1}, .08, 2},
			{"below threshold named runner up counts", []float32{.6, .54, .65}, []int64{1, 2}, .08, 3},
			{"ambiguous unnamed creates group", []float32{.54, .65, .6}, []int64{1}, .08, 0},
			{"both stages ambiguous", []float32{.7, .68, .65, .6}, []int64{1, 2}, .08, 0},
			{"only named ambiguous", []float32{.7, .68}, []int64{1, 2}, .08, 0},
			{"only unnamed duplicates in one group", []float32{.65}, nil, .2, 1},
			{"only named duplicates in one group", []float32{.65}, []int64{1}, .2, 1},
			{"weak groups create group", []float32{.5, .5}, []int64{1}, .08, 0},
			{"zero margin keeps named priority", []float32{.7, .7, .99}, []int64{1, 2}, 0, 1},
			{"zero margin unnamed ties", []float32{.7, .7}, nil, 0, 1},
			{"no references creates group", nil, nil, .08, 0},
		} {
			t.Run(fmt.Sprintf("%s/favorite=%v", tc.name, favorite), func(t *testing.T) {
				ctx := context.Background()
				paths := make([]string, len(tc.scores))
				vectors := make([][]float32, len(tc.scores))
				counts := make([]int, len(tc.scores))
				for i, score := range tc.scores {
					paths[i], vectors[i], counts[i] = fmt.Sprintf("ref-%d.jpg", i), matchingVector(score), 3
				}
				l := faceLibrary(t, append(paths, "z-query.jpg")...)
				seedMatchingPeople(t, l, paths, vectors, counts)
				for _, person := range tc.named {
					if err := l.RenamePerson(ctx, person, fmt.Sprintf("Person %d", person)); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := l.index.db.Exec(`UPDATE photo_faces SET favorite=?;
 UPDATE photo_face_jobs SET status='done' WHERE path<>'z-query.jpg'`, favorite); err != nil {
					t.Fatal(err)
				}
				thresholds := DefaultFaceThresholds()
				thresholds.AssignmentMargin = tc.margin
				if err := l.SetFaceThresholds(ctx, thresholds); err != nil {
					t.Fatal(err)
				}
				job := finishFace(t, l, 0)
				faces, err := l.AutomaticFaces(ctx, job.Path)
				if err != nil || job.Path != "z-query.jpg" || len(faces) != 1 {
					t.Fatalf("query result: %s %+v %v", job.Path, faces, err)
				}
				want, assignment := tc.want, "matched"
				if want == 0 {
					want, assignment = int64(len(tc.scores)+1), "new"
				}
				var gotAssignment string
				if err := l.index.db.QueryRow(`SELECT recognition_assignment FROM photo_faces WHERE id=?`, faces[0].ID).Scan(&gotAssignment); err != nil {
					t.Fatal(err)
				}
				if faces[0].PersonID != want || gotAssignment != assignment {
					t.Fatalf("person=%d assignment=%s, want %d %s", faces[0].PersonID, gotAssignment, want, assignment)
				}
			})
		}
	}
}

func TestFaceRecognitionPriorityExcludesAlreadyUsedGroup(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "z.jpg")
	seedMatchingPeople(t, l, []string{"a.jpg", "b.jpg"}, [][]float32{matchingVector(.65), matchingVector(.95)}, []int{3, 3})
	if err := l.RenamePerson(ctx, 1, "Named"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`UPDATE photo_face_jobs SET status='done' WHERE path<>'z.jpg'`); err != nil {
		t.Fatal(err)
	}
	job, err := l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, second := faceDetection(0), faceDetection(0)
	second.X = .6
	if err := l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{first, second}}); err != nil {
		t.Fatal(err)
	}
	faces, err := l.AutomaticFaces(ctx, "z.jpg")
	if err != nil || len(faces) != 2 || faces[0].PersonID != 1 || faces[1].PersonID != 2 {
		t.Fatalf("second face must fall back after excluding the used named group: %+v %v", faces, err)
	}
}

func TestFaceRecognitionPriorityValidatesNamedReferences(t *testing.T) {
	for _, change := range []string{"none", "private", "private-name", "ignored", "deleted", "reassigned", "quality", "unnamed", "new-name", "private-fallback"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			paths := []string{"private/a.jpg", "b.jpg", "c.jpg"}
			l := faceLibrary(t, paths...)
			seedMatchingPeople(t, l, paths, [][]float32{matchingVector(.9), matchingVector(.7), matchingVector(.99)}, []int{3, 3, 3})
			for _, person := range []int64{1, 2} {
				if err := l.RenamePerson(ctx, person, fmt.Sprintf("Person %d", person)); err != nil {
					t.Fatal(err)
				}
			}
			prepareReferenceGraph(t, l)
			tx, err := l.index.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			switch change {
			case "private", "private-name":
				if err := os.WriteFile(filepath.Join(l.Root(), "private/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				if change == "private-name" {
					_, err = tx.Exec(`UPDATE photo_faces SET path='b.jpg' WHERE person_id=1; UPDATE photo_people SET name_source='private/a.jpg' WHERE id=1`)
				}
			case "ignored":
				_, err = tx.Exec(`UPDATE photo_faces SET ignored=1 WHERE person_id=1`)
			case "deleted":
				_, err = tx.Exec(`DELETE FROM photo_faces WHERE person_id=1`)
			case "reassigned":
				_, err = tx.Exec(`UPDATE photo_faces SET person_id=3 WHERE person_id=1`)
			case "quality":
				_, err = tx.Exec(`UPDATE photo_faces SET reference_eligible=0 WHERE person_id=1`)
			case "unnamed":
				_, err = tx.Exec(`UPDATE photo_people SET name='' WHERE id=1`)
			case "new-name":
				_, err = tx.Exec(`UPDATE photo_people SET name='New name' WHERE id=3`)
			case "private-fallback":
				if err := os.WriteFile(filepath.Join(l.Root(), "private/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				_, err = tx.Exec(`UPDATE photo_faces SET path='private/a.jpg' WHERE person_id=2`)
			}
			if err != nil {
				t.Fatal(err)
			}
			q := &countedFaceMatchQuery{tx: tx}
			got, err := l.nearestPerson(ctx, q, faceDetection(0).Embedding, nil)
			want := int64(2)
			if change == "none" {
				want = 1
			}
			queries := 2 // Named IDs and one batch; unnamed references are not read.
			if change == "private-fallback" {
				want, queries = 3, 3
			} else if change == "new-name" {
				want = 3
			}
			if err != nil || got != want || q.calls != queries {
				t.Fatalf("person=%d queries=%d err=%v, want %d and %d queries", got, q.calls, err, want, queries)
			}
		})
	}
}

func TestFaceRecognitionNamedLookupUsesPartialIndex(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<20000)
 INSERT INTO photo_people(name) SELECT CASE WHEN x=1 THEN 'Named' ELSE '' END FROM n`); err != nil {
		t.Fatal(err)
	}
	for _, analyze := range []bool{false, true} {
		if analyze {
			if _, err := l.index.db.Exec(`ANALYZE`); err != nil {
				t.Fatal(err)
			}
		}
		rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN ` + faceNamedPeopleSQL)
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
		if err != nil || !strings.Contains(plan, "idx_photo_people_named_id") || strings.Contains(plan, "TEMP B-TREE") {
			t.Fatalf("analyze=%v: unbounded name lookup: %s (%v)", analyze, plan, err)
		}
		named, err := faceNamedPeople(context.Background(), l.index.db)
		if err != nil || len(named) != 1 || !named[1] {
			t.Fatalf("named IDs: %v %v", named, err)
		}
	}
}

func TestFaceRecognitionPrioritySynchronizesNames(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	seedMatchingPeople(t, l, []string{"a.jpg", "b.jpg"}, [][]float32{matchingVector(.65), matchingVector(.95)}, []int{3, 3})
	for _, name := range []string{"Named", "", "Named again"} {
		tx, err := l.index.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`UPDATE photo_people SET name=? WHERE id=1`, name); err != nil {
			t.Fatal(err)
		}
		affected := map[int64]bool{1: true}
		revision, err := refreshFaceMutationTx(ctx, tx, affected)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := l.syncFaceGraphPeople(ctx, affected, revision); err != nil {
			t.Fatal(err)
		}
		for _, rebuild := range []bool{false, true} {
			if rebuild {
				l.faceRuntime.graph = nil
				prepareReferenceGraph(t, l)
			}
			got, err := l.nearestPerson(ctx, l.index.db, faceDetection(0).Embedding, nil)
			want := int64(1)
			if name == "" {
				want = 2
			}
			if err != nil || got != want {
				t.Fatalf("name=%q rebuild=%v: person=%d err=%v, want %d", name, rebuild, got, err, want)
			}
		}
	}
}
