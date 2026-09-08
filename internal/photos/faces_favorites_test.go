package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"bearstack/internal/facerec"
)

func selectedReferences(t *testing.T, l *Library) []int64 {
	t.Helper()
	rows, err := l.index.db.Query(`SELECT face_id FROM photo_face_references ORDER BY face_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestFaceFavoritesSelection(t *testing.T) {
	ctx := context.Background()
	paths := []string{"a/1.jpg", "a/2.jpg", "a/3.jpg", "b/1.jpg", "b/2.jpg", "c/1.jpg"}
	l := faceLibrary(t, paths...)
	var faces []RecognizedFace
	for range paths {
		job := finishFace(t, l, 0)
		f, err := l.AutomaticFaces(ctx, job.Path)
		if err != nil {
			t.Fatal(err)
		}
		faces = append(faces, f[0])
	}
	if err := l.SetFaceReferenceLimit(ctx, 3); err != nil {
		t.Fatal(err)
	}
	check := func(want ...int64) {
		t.Helper()
		prepareReferenceGraph(t, l)
		slices.Sort(want)
		if got := selectedReferences(t, l); !slices.Equal(got, want) {
			t.Fatalf("references %v, want %v", got, want)
		}
	}
	favorite := func(i int, value bool) {
		t.Helper()
		if _, err := l.SetFaceFavorite(ctx, faces[i].ID, faces[i].PersonID, value); err != nil {
			t.Fatal(err)
		}
	}
	// Even high-confidence/manual duplicates cannot crowd out other folders.
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET manual=1 WHERE directory='a'`); err != nil {
		t.Fatal(err)
	}
	check(faces[0].ID, faces[3].ID, faces[5].ID)
	favorite(2, true)
	check(faces[2].ID, faces[3].ID, faces[5].ID)
	favorite(1, true)
	check(faces[1].ID, faces[2].ID, faces[3].ID)
	favorite(0, true)
	favorite(4, true)
	check(faces[0].ID, faces[1].ID, faces[2].ID, faces[4].ID) // four favorites exceed target three
	favorite(0, false)
	favorite(1, false)
	check(faces[2].ID, faces[4].ID, faces[5].ID)
	if err := l.SetFaceReferenceLimit(ctx, 100); err != nil {
		t.Fatal(err)
	}
	check(faces[0].ID, faces[1].ID, faces[2].ID, faces[3].ID, faces[4].ID, faces[5].ID)
	if _, err := l.NextFaceJob(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unexpected inference: %v", err)
	}
}

func TestFaceFavoriteAtomicPersistenceAndMembership(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "a2.jpg", "b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	f := faces[0]
	other, _ := l.AutomaticFaces(ctx, "b.jpg")
	if _, err := l.SetFaceFavorite(ctx, f.ID, other[0].PersonID, true); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("stale membership: %v", err)
	}
	previousGraph := l.faceRuntime.graph
	if _, err := l.SetFaceFavorite(ctx, f.ID, f.PersonID, true); err != nil {
		t.Fatal(err)
	}
	if l.faceRuntime.graph != previousGraph || l.faceRuntime.favorites[f.ID] == nil {
		t.Fatal("favorite change rebuilt the full graph instead of syncing the person")
	}
	var before, after int64
	l.index.db.QueryRow(`SELECT revision FROM photo_face_state`).Scan(&before)
	prepareReferenceGraph(t, l)
	graph := l.faceRuntime.graph
	if _, err := l.SetFaceFavorite(ctx, f.ID, f.PersonID, true); err != nil {
		t.Fatal(err)
	}
	l.index.db.QueryRow(`SELECT revision FROM photo_face_state`).Scan(&after)
	if before != after || graph != l.faceRuntime.graph {
		t.Fatal("idempotent assignment invalidated references")
	}
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_favorite_reference BEFORE INSERT ON photo_face_references BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetFaceFavorite(ctx, f.ID, f.PersonID, false); err == nil {
		t.Fatal("expected reference failure")
	}
	current, err := l.Face(ctx, f.ID)
	if err != nil || !current.Favorite {
		t.Fatalf("favorite not rolled back: %+v %v", current, err)
	}
	if !slices.Contains(selectedReferences(t, l), f.ID) {
		t.Fatal("references not rolled back")
	}
	if _, err := l.index.db.Exec(`DROP TRIGGER fail_favorite_reference`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	l.Close()
	l, err = New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	current, err = l.Face(ctx, f.ID)
	if err != nil || !current.Favorite {
		t.Fatalf("restart lost favorite: %+v %v", current, err)
	}
	if err = l.EditFaces(ctx, []int64{f.ID}, other[0].PersonID, false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = l.SetFaceFavorite(ctx, f.ID, f.PersonID, false); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("moved face accepted old group: %v", err)
	}
	current, err = l.Face(ctx, f.ID)
	if err != nil || !current.Favorite {
		t.Fatal("move lost favorite")
	}
	if err = l.EditFaces(ctx, []int64{f.ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(selectedReferences(t, l), f.ID) {
		t.Fatal("ignored favorite remains a reference")
	}
	if _, err = l.SetFaceFavorite(ctx, f.ID, current.PersonID, false); !errors.Is(err, ErrLabelConflict) {
		t.Fatalf("ignored mutation: %v", err)
	}
	if err = l.EditFaces(ctx, []int64{f.ID}, other[0].PersonID, false, ""); err != nil {
		t.Fatal(err)
	}
	current, err = l.Face(ctx, f.ID)
	if err != nil || !current.Favorite || !slices.Contains(selectedReferences(t, l), f.ID) {
		t.Fatal("restore lost favorite")
	}
	if err = l.MergePeople(ctx, other[0].PersonID, f.PersonID); err != nil {
		t.Fatal(err)
	}
	current, err = l.Face(ctx, f.ID)
	if err != nil || !current.Favorite || current.PersonID != f.PersonID {
		t.Fatalf("merge lost favorite: %+v %v", current, err)
	}
}

func TestFaceFavoriteRedetection(t *testing.T) {
	for _, scenario := range []string{"same", "unmatched", "ambiguous"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a.jpg")
			finishFace(t, l, 0)
			faces, _ := l.AutomaticFaces(ctx, "a.jpg")
			f := faces[0]
			if _, err := l.SetFaceFavorite(ctx, f.ID, f.PersonID, true); err != nil {
				t.Fatal(err)
			}
			if err := l.PrepareFaceQueue(ctx, "next-model"); err != nil {
				t.Fatal(err)
			}
			job, err := l.NextFaceJob(ctx)
			if err != nil {
				t.Fatal(err)
			}
			detections := []facerec.Detection{faceDetection(0)}
			if scenario == "unmatched" {
				detections[0].X = .65
			}
			if scenario == "ambiguous" {
				detections = append(detections, faceDetection(1))
			}
			if err := l.CommitFaceResult(ctx, job, facerec.Result{Model: "next-model", Faces: detections}); err != nil {
				t.Fatal(err)
			}
			faces, err = l.AutomaticFaces(ctx, "a.jpg")
			if err != nil {
				t.Fatal(err)
			}
			for _, face := range faces {
				if face.Favorite != (scenario == "same") {
					t.Fatalf("incorrect carried favorite: %+v", face)
				}
			}
			if scenario == "same" && faces[0].PersonID != f.PersonID {
				t.Fatal("favorite changed person")
			}
		})
	}
}

func TestFaceFavoriteMigration(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	if _, err := l.index.db.Exec(`DROP INDEX idx_face_reference_folders; DROP INDEX idx_face_favorites; ALTER TABLE photo_faces DROP COLUMN favorite; UPDATE schema_migrations SET version=21 WHERE component='photos'; UPDATE photo_face_reference_settings SET reference_limit=5,pending=0,cursor=0`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	l.Close()
	l, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var limit, pending int
	if err := l.index.db.QueryRow(`SELECT reference_limit,pending FROM photo_face_reference_settings`).Scan(&limit, &pending); err != nil || limit != 5 || pending != 1 {
		t.Fatalf("migration %d %d %v", limit, pending, err)
	}
	faces, err := l.AutomaticFaces(ctx, "a.jpg")
	if err != nil || len(faces) != 1 || faces[0].Favorite {
		t.Fatalf("migration data %+v %v", faces, err)
	}
	prepareReferenceGraph(t, l)
	if faceReferenceCount(t, l) != 1 {
		t.Fatal("migration lost references")
	}
	if _, err := l.NextFaceJob(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("migration queued inference")
	}
}

func TestFaceFavoritesExactMatchingAndBatches(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "refs/a.jpg", "z/b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	faces, _ := l.AutomaticFaces(ctx, "refs/a.jpg")
	f := faces[0]
	// All 601 favorites must be present despite a limit of one. Only the final,
	// lower-confidence favorite resembles the query; the others are orthogonal.
	_, err := l.index.db.Exec(`WITH RECURSIVE n(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<600)
 INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,favorite)
 SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model,1 FROM photo_faces,n WHERE id=?;
 UPDATE photo_faces SET favorite=1 WHERE person_id=?`, f.ID, f.PersonID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=?,confidence=.8 WHERE id=(SELECT max(id) FROM photo_faces)`, encodeVector(faceDetection(2).Embedding)); err != nil {
		t.Fatal(err)
	}
	if err := l.SetFaceReferenceLimit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	prepareReferenceGraph(t, l)
	if len(l.faceRuntime.favorites) != 601 || l.faceRuntime.graph.Len() != 1 {
		t.Fatalf("favorites %d graph %d", len(l.faceRuntime.favorites), l.faceRuntime.graph.Len())
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	q := &countedFaceMatchQuery{tx: tx}
	got, err := l.nearestPerson(ctx, q, faceDetection(2).Embedding, nil)
	if err != nil || got != f.PersonID || q.calls != 2 {
		t.Fatalf("exact match %d, queries %d: %v", got, q.calls, err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "refs/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := l.nearestPerson(ctx, q, faceDetection(2).Embedding, nil); err != nil || got != 0 {
		t.Fatalf("private favorite matched: %d %v", got, err)
	}
}

func BenchmarkFaceReferenceSelection(b *testing.B) {
	for _, favorites := range []int{0, 10, 100} {
		for _, count := range []int{1000, 10000, 100000} {
			b.Run(fmt.Sprintf("%d/favorites_%d", count, favorites), func(b *testing.B) {
				root := b.TempDir()
				l, err := New(root, filepath.Join(root, "cache"), filepath.Join(root, "photos.db"), 60)
				if err != nil {
					b.Fatal(err)
				}
				defer l.Close()
				_, err = l.index.db.Exec(`INSERT INTO photo_people(id) VALUES(1);
  INSERT INTO media_index(path,directory,name,size_bytes,mod_time_unix_nano,type,mime_type,indexed_at) VALUES('a.jpg','','a.jpg',1,1,'image','image/jpeg','');
  WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?)
  INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model,favorite)
  SELECT 'a.jpg',printf('folder-%03d',x%100),1,0,0,.3,.3,.99,zeroblob(512),'',x<=? FROM n`, count, favorites)
				if err != nil {
					b.Fatal(err)
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					tx, err := l.index.db.Begin()
					if err != nil {
						b.Fatal(err)
					}
					if err := refreshFaceReferencesTx(context.Background(), tx, 1); err != nil {
						tx.Rollback()
						b.Fatal(err)
					}
					if err := tx.Commit(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func TestFaceFavoritesConcurrentAssignments(t *testing.T) {
	ctx := context.Background()
	paths := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"}
	l := faceLibrary(t, paths...)
	var faces []RecognizedFace
	for range paths {
		job := finishFace(t, l, 0)
		f, err := l.AutomaticFaces(ctx, job.Path)
		if err != nil {
			t.Fatal(err)
		}
		faces = append(faces, f[0])
	}
	if err := l.SetFaceReferenceLimit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, len(faces)+1)
	for _, f := range faces {
		go func() {
			_, err := l.SetFaceFavorite(ctx, f.ID, f.PersonID, true)
			results <- err
		}()
	}
	// Build/search concurrently with writes, using the same runtime serialization
	// as new face processing. The final state must retain every successful write.
	go func() {
		l.faceRuntime.mu.Lock()
		defer l.faceRuntime.mu.Unlock()
		err := l.ensureFaceGraph(ctx, facerec.Model)
		if err == nil {
			tx, e := l.index.db.BeginTx(ctx, nil)
			err = e
			if err == nil {
				_, err = l.nearestPerson(ctx, tx, faceDetection(0).Embedding, nil)
				tx.Rollback()
			}
		}
		results <- err
	}()
	for range len(faces) + 1 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	prepareReferenceGraph(t, l)
	if len(l.faceRuntime.favorites) != len(faces) || faceReferenceCount(t, l) != len(faces) {
		t.Fatal("concurrent favorite writes lost")
	}
}
