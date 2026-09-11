package photos

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/facerec"
)

func matchingVector(score float32) []float32 {
	v := make([]float32, facerec.Dimensions)
	v[0] = score
	v[1] = float32(math.Sqrt(1 - float64(score)*float64(score)))
	return v
}

func seedMatchingPeople(t *testing.T, l *Library, paths []string, vectors [][]float32, counts []int) {
	t.Helper()
	ctx := context.Background()
	if err := l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for p, vector := range vectors {
		if _, err := tx.Exec(`INSERT INTO photo_people(id) VALUES(?)`, p+1); err != nil {
			t.Fatal(err)
		}
		for range counts[p] {
			if _, err := tx.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model) VALUES(?,?,?,.2,.2,.3,.3,.99,?,?)`, paths[p], parentPath(paths[p]), p+1, encodeVector(vector), facerec.Model); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	prepareReferenceGraph(t, l)
}

func TestFaceCandidatesDoNotStarveAfterExclusion(t *testing.T) {
	ctx := context.Background()
	paths := []string{"a.jpg", "b.jpg", "c.jpg"}
	l := faceLibrary(t, paths...)
	seedMatchingPeople(t, l, paths, [][]float32{matchingVector(.9), matchingVector(.8), faceDetection(2).Embedding}, []int{30, 30, 1})
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	q := &countedFaceMatchQuery{tx: tx}
	got, err := l.nearestPerson(ctx, q, faceDetection(0).Embedding, map[int64]bool{1: true})
	if err != nil || got != 2 {
		t.Fatalf("clear person B after excluding A: got %d, %v", got, err)
	}
	if q.calls != 2 {
		t.Fatalf("%d metadata queries, want named IDs and one bounded batch", q.calls)
	}
}

func TestFaceCandidatesContinueAfterLiveFiltering(t *testing.T) {
	for _, change := range []string{"private", "private-name", "ignored", "deleted", "reassigned", "quality"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			paths := []string{"private/a.jpg", "b.jpg", "c.jpg", "d.jpg"}
			l := faceLibrary(t, paths...)
			seedMatchingPeople(t, l, paths, [][]float32{matchingVector(.99), matchingVector(.9), matchingVector(.7), matchingVector(.1)}, []int{30, 30, 30, 30})
			tx, err := l.index.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			switch change {
			case "private", "private-name":
				if err = os.WriteFile(filepath.Join(l.Root(), "private/.adminonly"), nil, 0600); err != nil {
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
				_, err = tx.Exec(`UPDATE photo_faces SET person_id=4 WHERE person_id=1`)
			case "quality":
				_, err = tx.Exec(`UPDATE photo_faces SET reference_eligible=0 WHERE person_id=1`)
			}
			if err != nil {
				t.Fatal(err)
			}
			q := &countedFaceMatchQuery{tx: tx}
			got, err := l.facePersonCandidates(ctx, q, faceDetection(0).Embedding, nil, 2)
			if err != nil || len(got) != 2 || got[0].person != 2 || got[1].person != 3 {
				t.Fatalf("filtered leading person: %+v, %v", got, err)
			}
			if q.calls != 2 {
				t.Fatalf("%d queries, want initial pair and one replacement group", q.calls)
			}
		})
	}
}

func TestFaceCandidatesExactDuplicateRecallAndTies(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(42))
	paths := make([]string, 100)
	vectors := make([][]float32, 100)
	counts := make([]int, 100)
	for p := range vectors {
		v := make([]float32, facerec.Dimensions)
		for d := range v {
			v[d] = float32(rng.NormFloat64())
		}
		d := faceDetection(0)
		d.Embedding = v
		if err := facerec.Validate(&d); err != nil {
			t.Fatal(err)
		}
		paths[p], vectors[p], counts[p] = "a.jpg", v, 30
	}
	l := faceLibrary(t, "a.jpg")
	seedMatchingPeople(t, l, paths, vectors, counts)
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for p, v := range vectors {
		q := &countedFaceMatchQuery{tx: tx}
		got, err := l.nearestPerson(ctx, q, v, nil)
		if err != nil || got != int64(p+1) {
			t.Fatalf("identical stored reference of person %d: got %d, %v", p+1, got, err)
		}
		if q.calls != 2 {
			t.Fatalf("queried entire corpus instead of best groups: %d queries", q.calls)
		}
	}
	// A score tie must produce stable person-ID order, while the established
	// margin prevents turning that ordering into an automatic identity.
	clear(vectors[0])
	got, err := l.facePersonCandidates(ctx, tx, vectors[0], nil, 3)
	if err != nil || len(got) != 3 || got[0].person != 1 || got[1].person != 2 || got[2].person != 3 {
		t.Fatalf("stable ties: %+v, %v", got, err)
	}
	if got[0].face != 1 || got[1].face != 31 || got[2].face != 61 {
		t.Fatalf("stable supporting reference IDs: %+v", got)
	}
}

func TestFaceCandidatesReturnVisibleScoreWitness(t *testing.T) {
	ctx := context.Background()
	paths := []string{"private/a.jpg", "b.jpg"}
	l := faceLibrary(t, paths...)
	seedMatchingPeople(t, l, paths, [][]float32{matchingVector(.99), matchingVector(.1)}, []int{2, 1})
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET path='b.jpg',embedding=? WHERE id=2`, encodeVector(matchingVector(.8))); err != nil {
		t.Fatal(err)
	}
	l.faceRuntime.graph = nil
	prepareReferenceGraph(t, l)
	if err := os.WriteFile(filepath.Join(l.Root(), "private/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	got, err := l.facePersonCandidates(ctx, tx, faceDetection(0).Embedding, nil, 2)
	if err != nil || len(got) != 2 || got[0].person != 1 || got[0].face != 2 || got[0].score != float64(float32(.8)) {
		t.Fatalf("must return the visible reference supplying score, not min ID: %+v, %v", got, err)
	}
}

func TestFaceCandidatesFindOnlyVisiblePerson(t *testing.T) {
	ctx := context.Background()
	paths := []string{"a.jpg", "private/b.jpg"}
	l := faceLibrary(t, paths...)
	seedMatchingPeople(t, l, paths, [][]float32{matchingVector(.9), matchingVector(.1)}, []int{30, 30})
	if err := os.WriteFile(filepath.Join(l.Root(), "private/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	got, err := l.nearestPerson(ctx, tx, faceDetection(0).Embedding, nil)
	if err != nil || got != 1 {
		t.Fatalf("private graph member must not hide unique visible match: %d, %v", got, err)
	}
}

func TestFaceReferenceMoveSurvivesTargetBeforeSourceSync(t *testing.T) {
	for _, favorite := range []bool{false, true} {
		t.Run(fmt.Sprint(favorite), func(t *testing.T) {
			ctx := context.Background()
			paths := []string{"a.jpg", "b.jpg"}
			l := faceLibrary(t, paths...)
			seedMatchingPeople(t, l, paths, [][]float32{matchingVector(.9), matchingVector(.1)}, []int{1, 1})
			if _, err := l.index.db.Exec(`UPDATE photo_faces SET favorite=? WHERE id=1; UPDATE photo_face_reference_settings SET pending=1,cursor=0`, !favorite); err != nil {
				t.Fatal(err)
			}
			prepareReferenceGraph(t, l)
			tx, err := l.index.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`UPDATE photo_faces SET person_id=2,favorite=? WHERE id=1`, favorite); err != nil {
				t.Fatal(err)
			}
			for _, person := range []int64{1, 2} {
				if err := refreshFaceReferencesTx(ctx, tx, person); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			// Deterministically exercise the order that an affected-person map is
			// allowed to take: target installation precedes old source cleanup.
			for _, person := range []int64{2, 1} {
				if err := l.syncFaceGraphPeople(ctx, map[int64]bool{person: true}, l.faceRuntime.revision); err != nil {
					t.Fatal(err)
				}
			}
			if l.faceRuntime.people[1] != 2 || l.faceRuntime.faceReferenceVector(1) == nil {
				t.Fatal("source cleanup removed the target's moved reference")
			}
			_, inOrdinary := l.faceRuntime.graph.vectors[1]
			_, inFavorites := l.faceRuntime.favorites[1]
			if inFavorites != favorite || inOrdinary == favorite {
				t.Fatalf("moved reference in ordinary=%v favorites=%v, favorite=%v", inOrdinary, inFavorites, favorite)
			}
		})
	}
}

func BenchmarkFacePersonRanking(b *testing.B) {
	for _, total := range []int{30000, 300000} {
		b.Run(fmt.Sprint(total), func(b *testing.B) {
			rt := faceRuntime{graph: newFaceVectorIndex(), nodes: make(map[int64][]int64), favorites: make(map[int64][]float32)}
			data := make([]float32, total*facerec.Dimensions)
			for i := 0; i < total; i++ {
				v := data[i*facerec.Dimensions : (i+1)*facerec.Dimensions]
				v[i%facerec.Dimensions] = 1
				id, person := int64(i+1), int64(i/30+1)
				rt.graph.vectors[id] = v
				rt.nodes[person] = append(rt.nodes[person], id)
				rt.graph.groups[person] = append(rt.graph.groups[person], v)
			}
			query := faceDetection(0).Embedding
			named := make(map[int64]bool)
			for person := range rt.nodes {
				if person%10 == 0 {
					named[person] = true
				}
			}
			for _, scope := range []facePersonScope{facePersonsAll, facePersonsNamed, facePersonsUnnamed} {
				b.Run(map[facePersonScope]string{facePersonsAll: "all", facePersonsNamed: "named_10_percent", facePersonsUnnamed: "unnamed_90_percent"}[scope], func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						if _, err := rt.rankFacePersonsInScope(context.Background(), query, nil, named, scope); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
