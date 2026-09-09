package photos

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/facerec"
)

func assertRecognitionCounts(t *testing.T, l *Library, matched, created, unknown int) {
	t.Helper()
	s, err := l.FaceStatus(context.Background())
	if err != nil || s.RecognitionMatched != matched || s.RecognitionNew != created || s.RecognitionUnknown != unknown {
		t.Fatalf("recognition counts = %+v, %v; want %d/%d/%d", s, err, matched, created, unknown)
	}
	percent := 0.0
	if matched+created > 0 {
		percent = 100 * float64(matched) / float64(matched+created)
	}
	if math.Abs(s.RecognitionMatchPercent-percent) > .001 {
		t.Fatalf("percent = %f, want %f", s.RecognitionMatchPercent, percent)
	}
}

func TestRecognitionAssignmentCountsAndCorrections(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
	assertRecognitionCounts(t, l, 0, 0, 0)
	finishFace(t, l, 0)
	job := finishFace(t, l, 0)
	finishFace(t, l, 1)
	assertRecognitionCounts(t, l, 1, 2, 0)
	b, _ := l.AutomaticFaces(ctx, "b.jpg")
	if err := l.EditFaces(ctx, []int64{b[0].ID}, 0, false, "Changed"); err != nil {
		t.Fatal(err)
	}
	assertRecognitionCounts(t, l, 1, 2, 0)
	// Reanalysis preserves the original decision when carrying a correction.
	if err := l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(0)}}); err != nil {
		t.Fatal(err)
	}
	assertRecognitionCounts(t, l, 1, 2, 0)
	if _, err := l.ReconcileFacesBatch(ctx, 100); err != nil {
		t.Fatal(err)
	}
	assertRecognitionCounts(t, l, 1, 2, 0)
	b, _ = l.AutomaticFaces(ctx, "b.jpg")
	if err := l.EditFaces(ctx, []int64{b[0].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	assertRecognitionCounts(t, l, 0, 2, 0)
	if err := l.ClearFaces(ctx); err != nil {
		t.Fatal(err)
	}
	assertRecognitionCounts(t, l, 0, 0, 0)
}

func TestRecognitionAssignmentRollbackAndXMP(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	writeXMPFace(t, filepath.Join(l.Root(), "a.jpg"), "Marie", .35, .35, .3, .3)
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	finishFace(t, l, 0)
	assertRecognitionCounts(t, l, 0, 1, 0)
	job, err := l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.index.db.Exec(`CREATE TRIGGER fail_recognition BEFORE UPDATE OF status ON photo_face_jobs BEGIN SELECT RAISE(ABORT,'failure'); END`); err != nil {
		t.Fatal(err)
	}
	result := facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(0)}}
	if err = l.CommitFaceResult(ctx, job, result); err == nil {
		t.Fatal("expected rollback")
	}
	assertRecognitionCounts(t, l, 0, 1, 0)
	if _, err = l.index.db.Exec(`DROP TRIGGER fail_recognition`); err != nil {
		t.Fatal(err)
	}
	if err = l.CommitFaceResult(ctx, job, result); err != nil {
		t.Fatal(err)
	}
	assertRecognitionCounts(t, l, 1, 1, 0)
}

func TestRecognitionAssignmentMigrationAndRestart(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	finishFace(t, l, 0)
	// Simulate the last released schema without inventing historical decisions.
	if _, err := l.index.db.Exec(`DROP INDEX idx_face_recognition_assignment; ALTER TABLE photo_faces DROP COLUMN recognition_assignment; UPDATE schema_migrations SET version=27 WHERE component='photos'`); err != nil {
		t.Fatal(err)
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
	assertRecognitionCounts(t, resumed, 0, 0, 1)
	finishFace(t, resumed, 0)
	assertRecognitionCounts(t, resumed, 1, 0, 1)
	if err := setupFaceAssignmentStats(ctx, resumed.index.db); err != nil {
		t.Fatal(err)
	}
	var id, parent, unused int
	var plan string
	if err := resumed.index.db.QueryRow(`EXPLAIN QUERY PLAN SELECT count(*) FROM photo_faces WHERE ignored=0 AND recognition_assignment='matched'`).Scan(&id, &parent, &unused, &plan); err != nil || !strings.Contains(plan, "idx_face_recognition_assignment") {
		t.Fatalf("unindexed count: %s %v", plan, err)
	}
	if err := resumed.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	assertRecognitionCounts(t, again, 1, 0, 1)
}
