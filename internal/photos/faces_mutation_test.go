package photos

import (
	"context"
	"reflect"
	"testing"
)

func TestFaceMutationsRollbackWhenRevisionCannotBeSaved(t *testing.T) {
	for _, action := range []string{"favorite", "group ignore"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a.jpg")
			_, photo := finishGroupPhoto(t, l, 6, 0)
			prepareReferenceGraph(t, l)
			graph := l.faceRuntime.graph
			references := selectedReferences(t, l)
			var before, after int64
			if err := l.index.db.QueryRow(`SELECT revision FROM photo_face_state WHERE id=1`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err := l.index.db.Exec(`CREATE TRIGGER fail_mutation_revision BEFORE UPDATE OF revision ON photo_face_state BEGIN SELECT RAISE(ABORT,'injected revision failure'); END`); err != nil {
				t.Fatal(err)
			}
			var err error
			if action == "favorite" {
				face := photo.Faces[0]
				_, err = l.SetFaceFavorite(ctx, face.ID, face.PersonID, true)
			} else {
				_, err = l.IgnoreGroupPhoto(ctx, photo.Path, photo.Revision)
			}
			if err == nil {
				t.Fatal("revision failure was not propagated")
			}
			var changed int
			if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE favorite=1 OR ignored=1`).Scan(&changed); err != nil || changed != 0 {
				t.Fatalf("partial face change: %d %v", changed, err)
			}
			if err := l.index.db.QueryRow(`SELECT revision FROM photo_face_state WHERE id=1`).Scan(&after); err != nil || after != before {
				t.Fatalf("revision changed: %d -> %d (%v)", before, after, err)
			}
			if got := selectedReferences(t, l); !reflect.DeepEqual(got, references) {
				t.Fatalf("references changed: %v -> %v", references, got)
			}
			if l.faceRuntime.graph != graph {
				t.Fatal("rolled-back mutation changed the cache")
			}
		})
	}
}

func TestFaceMutationCacheFailurePreservesCommittedData(t *testing.T) {
	for _, failure := range []string{"canceled", "stale graph"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a.jpg", "b.jpg")
			finishFace(t, l, 0)
			finishFace(t, l, 1)
			faces, err := l.AutomaticFaces(ctx, "a.jpg")
			if err != nil {
				t.Fatal(err)
			}
			face := faces[0]
			if _, err := l.SetFaceFavorite(ctx, face.ID, face.PersonID, true); err != nil {
				t.Fatal(err)
			}
			prepareReferenceGraph(t, l)
			l.faceRuntime.mu.Lock()
			baseRevision := l.faceRuntime.revision
			syncCtx := ctx
			if failure == "canceled" {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				syncCtx = canceled
			} else {
				baseRevision++
			}
			l.syncFaceMutation(syncCtx, map[int64]bool{face.PersonID: true}, baseRevision, baseRevision+1)
			invalid := l.faceRuntime.graph == nil
			l.faceRuntime.mu.Unlock()
			if !invalid {
				t.Fatal("cache was not invalidated")
			}
			current, err := l.Face(ctx, face.ID)
			if err != nil || !current.Favorite {
				t.Fatalf("committed favorite lost: %+v %v", current, err)
			}
			prepareReferenceGraph(t, l)
			if l.faceRuntime.favorites[face.ID] == nil {
				t.Fatal("next analysis did not recover committed reference")
			}
		})
	}
}
