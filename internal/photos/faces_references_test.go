package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"bearstack/internal/facerec"
)

func faceReferenceCount(t *testing.T, l *Library) int {
	t.Helper()
	var count int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func prepareReferenceGraph(t *testing.T, l *Library) {
	t.Helper()
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	if err := l.ensureFaceGraph(context.Background(), facerec.Model); err != nil {
		t.Fatal(err)
	}
}

func TestFaceReferenceLimitRebuildAndPersistence(t *testing.T) {
	ctx := context.Background()
	var paths []string
	for i := 0; i < 35; i++ {
		paths = append(paths, fmt.Sprintf("%02d.jpg", i))
	}
	l := faceLibrary(t, paths...)
	for range paths {
		finishFace(t, l, 0)
	}
	if limit, err := l.FaceReferenceLimit(ctx); err != nil || limit != 30 {
		t.Fatalf("default %d %v", limit, err)
	}
	if n := faceReferenceCount(t, l); n != 30 {
		t.Fatalf("default references: %d", n)
	}
	// Preserve manually selected examples when shrinking, even if they arrived last.
	faces, err := l.AutomaticFaces(ctx, paths[34])
	if err != nil {
		t.Fatal(err)
	}
	if err = l.EditFaces(ctx, []int64{faces[0].ID}, faces[0].PersonID, false, ""); err != nil {
		t.Fatal(err)
	}
	if err = l.SetFaceReferenceLimit(ctx, 1); err != nil {
		t.Fatal(err)
	}
	prepareReferenceGraph(t, l)
	var selected int64
	if err = l.index.db.QueryRow(`SELECT face_id FROM photo_face_references`).Scan(&selected); err != nil || selected != faces[0].ID {
		t.Fatalf("manual reference lost: %d %v", selected, err)
	}
	if err = l.SetFaceReferenceLimit(ctx, 100); err != nil {
		t.Fatal(err)
	}
	prepareReferenceGraph(t, l)
	if n := faceReferenceCount(t, l); n != 35 {
		t.Fatalf("existing faces not incorporated: %d", n)
	}
	if _, err = l.NextFaceJob(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("photos requeued: %v", err)
	}
	graph := l.faceRuntime.graph
	if err = l.SetFaceReferenceLimit(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if l.faceRuntime.graph != graph {
		t.Fatal("unchanged settings discarded graph")
	}
	for _, limit := range []int{0, -1, 101} {
		if err = l.SetFaceReferenceLimit(ctx, limit); err == nil {
			t.Fatalf("invalid limit %d accepted", limit)
		}
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if limit, err := reopened.FaceReferenceLimit(ctx); err != nil || limit != 100 {
		t.Fatalf("restarted limit %d %v", limit, err)
	}
	prepareReferenceGraph(t, reopened)
	if len(reopened.faceRuntime.people) != 35 {
		t.Fatal("graph lost references after restart")
	}
}

func TestFaceReferenceMigrationAndResumableRefresh(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	// More than one checkpoint batch; vector data is already stored, no inference.
	_, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(2) UNION ALL SELECT x+1 FROM n WHERE x<220) INSERT INTO photo_people(id) SELECT x FROM n;
 INSERT INTO photo_faces(path,person_id,x,y,width,height,confidence,embedding,model) SELECT f.path,p.id,f.x,f.y,f.width,f.height,f.confidence,f.embedding,f.model FROM photo_people p CROSS JOIN photo_faces f WHERE f.id=1 AND p.id>1;
 DROP TABLE photo_face_reference_settings;
 UPDATE schema_migrations SET version=19 WHERE component='photos'`)
	if err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	l.Close()
	l, err = New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if limit, err := l.FaceReferenceLimit(ctx); err != nil || limit != 30 {
		t.Fatalf("migration limit %d %v", limit, err)
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = refreshFaceReferenceBatch(ctx, tx); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var cursor int
	if err = l.index.db.QueryRow(`SELECT cursor FROM photo_face_reference_settings`).Scan(&cursor); err != nil || cursor != 100 {
		t.Fatalf("checkpoint %d %v", cursor, err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err = l.rebuildFaceReferences(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation %v", err)
	}
	l.Close()
	resumed, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	prepareReferenceGraph(t, resumed)
	if n := faceReferenceCount(t, resumed); n != 220 {
		t.Fatalf("resumed references %d", n)
	}
	var pending bool
	if err = resumed.index.db.QueryRow(`SELECT pending FROM photo_face_reference_settings`).Scan(&pending); err != nil || pending {
		t.Fatalf("refresh still pending %v %v", pending, err)
	}
}

func TestFaceReferenceLimitPreservesAmbiguityCheck(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	a, err := l.AutomaticFaces(ctx, "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	b, err := l.AutomaticFaces(ctx, "b.jpg")
	if err != nil {
		t.Fatal(err)
	}
	vector := make([]float32, 128)
	vector[0] = .96
	vector[1] = .28
	if _, err = l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=?`, encodeVector(vector), b[0].ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 29; i++ {
		if _, err = l.index.db.Exec(`INSERT INTO photo_faces(path,person_id,x,y,width,height,confidence,embedding,model) SELECT path,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, a[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	// Refresh both persons after fixture edits. The closest 30 nodes all belong to A.
	if _, err = l.index.db.Exec(`UPDATE photo_face_reference_settings SET pending=1,cursor=0`); err != nil {
		t.Fatal(err)
	}
	prepareReferenceGraph(t, l)
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if id, err := l.nearestPerson(ctx, tx, faceDetection(0).Embedding, nil); err != nil || id != 0 {
		t.Fatalf("second person's ambiguity hidden by reference cap: chose %d", id)
	}
}
