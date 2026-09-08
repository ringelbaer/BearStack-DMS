package photos

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type countedFaceMatchQuery struct {
	tx    *sql.Tx
	calls int
	err   error
}

func (q *countedFaceMatchQuery) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	q.calls++
	if q.err != nil {
		return nil, q.err
	}
	return q.tx.QueryContext(ctx, query, args...)
}

func TestNearestPersonBatchesCandidatesAndHonorsCurrentVisibility(t *testing.T) {
	for _, favorite := range []bool{false, true} {
		name := "ordinary"
		if favorite {
			name = "favorites"
		}
		t.Run(name, func(t *testing.T) { testNearestPersonVisibility(t, favorite) })
	}
}
func testNearestPersonVisibility(t *testing.T, favorite bool) {
	for _, variant := range []string{"match", "excluded", "ignored", "deleted", "reassigned", "private", "private-name", "query-error", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "refs/a.jpg", "refs/b.jpg", "z/c.jpg")
			finishFace(t, l, 0)
			finishFace(t, l, 0)
			finishFace(t, l, 1)
			faces, err := l.AutomaticFaces(ctx, "refs/a.jpg")
			if err != nil || len(faces) != 1 {
				t.Fatalf("faces %v %v", faces, err)
			}
			person := faces[0].PersonID
			if favorite {
				if _, err := l.index.db.Exec(`UPDATE photo_faces SET favorite=1; UPDATE photo_face_state SET revision=revision+1`); err != nil {
					t.Fatal(err)
				}
				prepareReferenceGraph(t, l)
			}
			tx, err := l.index.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			q := &countedFaceMatchQuery{tx: tx}
			excluded := map[int64]bool{}
			wantErr := error(nil)
			switch variant {
			case "excluded":
				excluded[person] = true
			case "ignored":
				if _, err := tx.Exec(`UPDATE photo_faces SET ignored=1 WHERE person_id=?`, person); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if _, err := tx.Exec(`DELETE FROM photo_faces WHERE person_id=?`, person); err != nil {
					t.Fatal(err)
				}
			case "reassigned":
				if _, err := tx.Exec(`UPDATE photo_faces SET person_id=(SELECT id FROM photo_people WHERE id<>? LIMIT 1) WHERE person_id=?`, person, person); err != nil {
					t.Fatal(err)
				}
			case "private":
				if err := os.WriteFile(filepath.Join(l.Root(), "refs/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "private-name":
				if _, err := tx.Exec(`UPDATE photo_people SET name_source='z/c.jpg' WHERE id=?`, person); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(l.Root(), "z/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "query-error":
				wantErr = errors.New("database unavailable")
				q.err = wantErr
			case "canceled":
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
				wantErr = context.Canceled
			}
			got, err := l.nearestPerson(ctx, q, faceDetection(0).Embedding, excluded)
			if !errors.Is(err, wantErr) {
				t.Fatalf("error: %v, want %v", err, wantErr)
			}
			want := int64(0)
			if variant == "match" {
				want = person
			}
			if got != want {
				t.Fatalf("matched %d, want %d", got, want)
			}
			if q.calls != 1 {
				t.Fatalf("%d candidate queries, want one batch", q.calls)
			}
		})
	}
}
