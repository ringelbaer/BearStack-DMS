package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bearstack/internal/facerec"
)

func faceLibrary(t *testing.T, paths ...string) *Library {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0750); err != nil {
			t.Fatal(err)
		}
		writeJPEG(t, filepath.Join(root, p), color.White)
	}
	l, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 60)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	if _, err = l.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	return l
}
func faceDetection(axis int) facerec.Detection {
	v := make([]float32, facerec.Dimensions)
	v[axis] = 1
	return facerec.Detection{X: .2, Y: .2, Width: .3, Height: .3, Confidence: .99, Embedding: v}
}
func finishFace(t *testing.T, l *Library, axis int) FaceJob {
	t.Helper()
	ctx := context.Background()
	if err := l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	j, err := l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.CommitFaceResult(ctx, j, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(axis)}}); err != nil {
		t.Fatal(err)
	}
	return j
}
func TestFacesGroupSearchAndOverrides(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	p, err := l.People(ctx, 0, 1, "")
	if err != nil || len(p.People) != 2 {
		t.Fatalf("groups=%+v err=%v", p, err)
	}
	a, _ := l.AutomaticFaces(ctx, "a.jpg")
	b, _ := l.AutomaticFaces(ctx, "b.jpg")
	if a[0].PersonID != b[0].PersonID {
		t.Fatal("same face not grouped")
	}
	if err = l.RenamePerson(ctx, a[0].PersonID, "Jürgen"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`person:Juergen`, `face:Jürgen`, `person:j*gen`, `person:Juergen OR file_name:c.jpg`, `-person:Juergen`} {
		list, e := l.List(ctx, ListOptions{Query: q, PageSize: 60})
		want := 2
		if q == `person:Juergen OR file_name:c.jpg` {
			want = 3
		}
		if q == `-person:Juergen` {
			want = 1
		}
		if e != nil || list.Total != want {
			t.Errorf("%s total=%d want %d err=%v", q, list.Total, want, e)
		}
	}
	if err = l.EditFaces(ctx, []int64{b[0].ID}, 0, false, "Andere Person"); err != nil {
		t.Fatal(err)
	}
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	if _, err = l.NextFaceJob(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unchanged rescheduled: %v", err)
	}
	b, _ = l.AutomaticFaces(ctx, "b.jpg")
	if len(b) != 1 || !b[0].Manual || b[0].Name != "Andere Person" {
		t.Fatalf("override lost: %+v", b)
	}
	// A model generation change may carry an unambiguous manual region forward.
	if err = l.PrepareFaceQueue(ctx, "next-model"); err != nil {
		t.Fatal(err)
	}
	_, err = l.index.db.Exec(`UPDATE photo_face_jobs SET status='done' WHERE path<>'b.jpg'`)
	if err != nil {
		t.Fatal(err)
	}
	j, err := l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.CommitFaceResult(ctx, j, facerec.Result{Model: "next-model", Faces: []facerec.Detection{faceDetection(0)}}); err != nil {
		t.Fatal(err)
	}
	b, _ = l.AutomaticFaces(ctx, "b.jpg")
	if len(b) != 1 || !b[0].Manual || b[0].Name != "Andere Person" {
		t.Fatalf("model upgrade lost override: %+v", b)
	}
	if err = l.EditFaces(ctx, []int64{b[0].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	b, _ = l.AutomaticFaces(ctx, "b.jpg")
	if len(b) != 0 {
		t.Fatal("ignored face still visible")
	}
}
func TestFacesXMPAndPrivateInvalidation(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg", "b.jpg")
	writeXMPFace(t, filepath.Join(l.Root(), "album/a.jpg"), "Marie", .35, .35, .3, .3)
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	a, _ := l.AutomaticFaces(ctx, "album/a.jpg")
	if len(a) != 1 || a[0].Name != "Marie" {
		t.Fatalf("XMP not seeded: %+v", a)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "album/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Face(ctx, a[0].ID); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("private thumbnail access: %v", err)
	}
	p, err := l.People(ctx, 0, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.People) != 1 || p.People[0].Count != 1 || p.People[0].Name != "" {
		t.Fatalf("private count/name leaked: %+v", p)
	}
	var refs int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references r JOIN photo_faces f ON f.id=r.face_id WHERE f.path='album/a.jpg'`).Scan(&refs); err != nil || refs != 0 {
		t.Fatalf("private references: %d %v", refs, err)
	}
	if err = l.ClearFaces(ctx); err != nil {
		t.Fatal(err)
	}
	m, err := l.MediaContext(ctx, "album/a.jpg")
	if err != nil || len(m.Faces) != 1 {
		t.Fatalf("XMP removed: %+v %v", m, err)
	}
}
func TestFacesSourceReplacementAndRetry(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	j := finishFace(t, l, 0)
	if err := os.Remove(filepath.Join(l.Root(), "a.jpg")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE path='a.jpg'`).Scan(&n)
	if n != 0 {
		t.Fatal("deleted photo retained")
	}
	if err := l.CommitFaceResult(ctx, j, facerec.Result{Model: j.Model, Faces: []facerec.Detection{faceDetection(0)}}); err == nil {
		t.Fatal("stale result accepted")
	}
	if err := l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	j, err := l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		j.Attempts = attempt
		if err = l.FailFaceJob(ctx, j, "invalid"); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := l.FaceStatus(ctx)
	if s.Failed != 1 {
		t.Fatalf("retry limit: %+v", s)
	}
	if err = l.RetryFaceJobs(ctx); err != nil {
		t.Fatal(err)
	}
	j, err = l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.CommitFaceResult(ctx, j, facerec.Result{Model: j.Model}); err != nil {
		t.Fatal(err)
	}
	if err = l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	if _, err = l.NextFaceJob(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("no-face image repeated")
	}
}
func TestFacesAtomicEditsAndReferenceBound(t *testing.T) {
	ctx := context.Background()
	paths := []string{}
	for i := 0; i < 8; i++ {
		paths = append(paths, fmt.Sprintf("%d.jpg", i))
	}
	l := faceLibrary(t, paths...)
	for range paths {
		finishFace(t, l, 0)
	}
	var n int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references`).Scan(&n); err != nil || n != 5 {
		t.Fatalf("reference cap %d %v", n, err)
	}
	f, _ := l.AutomaticFaces(ctx, paths[0])
	if err := l.EditFaces(ctx, []int64{f[0].ID, 999999}, 0, true, ""); err == nil {
		t.Fatal("invalid batch accepted")
	}
	f, _ = l.AutomaticFaces(ctx, paths[0])
	if len(f) != 1 {
		t.Fatal("partial mutation")
	}
	oldMod := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(filepath.Join(l.Root(), paths[0]), oldMod, oldMod); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	f, _ = l.AutomaticFaces(ctx, paths[0])
	if len(f) != 0 {
		t.Fatal("replaced photo retained")
	}
}
func TestFaceImageLimitsAndCancellation(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.FaceImage(ctx, "a.jpg"); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, err := l.FaceImage(context.Background(), "../outside.jpg"); err == nil {
		t.Fatal("traversal accepted")
	}
	b, err := l.FaceImage(context.Background(), "a.jpg")
	if err != nil || len(b) == 0 {
		t.Fatalf("prepare image %v", err)
	}
}

func TestFacesRestartMigrationAndQueuePrivacy(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg", "b.jpg")
	finishFace(t, l, 0)
	f, _ := l.AutomaticFaces(ctx, "album/a.jpg")
	if err := l.RenamePerson(ctx, f[0].PersonID, "Jürgen"); err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{f[0].ID}, f[0].PersonID, false, ""); err != nil {
		t.Fatal(err)
	}
	// Upgrade backfills XMP search metadata without rewriting recognized faces.
	writeXMPFace(t, filepath.Join(l.Root(), "b.jpg"), "XMP Name", .35, .35, .3, .3)
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`DELETE FROM photo_xmp_people; UPDATE schema_migrations SET version=17 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	f, err = restarted.AutomaticFaces(ctx, "album/a.jpg")
	if err != nil || len(f) != 1 || f[0].Name != "Jürgen" || !f[0].Manual {
		t.Fatalf("restart lost override %+v %v", f, err)
	}
	list, err := restarted.List(ctx, ListOptions{Query: `person:"XMP Name"`, PageSize: 60})
	if err != nil || list.Total != 1 {
		t.Fatalf("migration backfill %d %v", list.Total, err)
	}
	if err = restarted.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	status, err := restarted.FaceStatus(ctx)
	if err != nil || status.Queued != 0 || status.Done != 0 || status.Faces != 0 {
		t.Fatalf("private job counts leaked %+v %v", status, err)
	}
}
func TestFaceImageEXIFOrientation(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	path := filepath.Join(l.Root(), "a.jpg")
	for _, orientation := range []int{1, 2, 3, 4, 5, 6, 7, 8} {
		writeSizedJPEG(t, path, 80, 40, color.White)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// One little-endian EXIF orientation entry; the input pixel raster stays 80x40.
		exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, byte(orientation), 0, 0, 0, 0, 0, 0, 0}
		segment := []byte{0xff, 0xe1, 0, byte(len(exif) + 2)}
		segment = append(segment, exif...)
		input := append(append(append([]byte{}, raw[:2]...), segment...), raw[2:]...)
		if err = os.WriteFile(path, input, 0600); err != nil {
			t.Fatal(err)
		}
		img, err := l.faceImage(context.Background(), "a.jpg")
		if err != nil {
			t.Fatal(err)
		}
		w, h := 80, 40
		if orientation >= 5 {
			w, h = h, w
		}
		if img.Bounds().Dx() != w || img.Bounds().Dy() != h {
			t.Fatalf("orientation %d bounds %v", orientation, img.Bounds())
		}
	}
}

func TestFacesXMPNameChangeAndLateSeed(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	writeXMPFace(t, filepath.Join(l.Root(), "a.jpg"), "First Name", .35, .35, .3, .3)
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	finishFace(t, l, 0)
	p, err := l.People(ctx, 0, 1, "")
	if err != nil || len(p.People) != 1 || p.People[0].Name != "First Name" {
		t.Fatalf("late seed fragmented group %+v %v", p, err)
	}
	writeXMPFace(t, filepath.Join(l.Root(), "a.jpg"), "Updated Name", .35, .35, .3, .3)
	if _, err = l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	f, err := l.AutomaticFaces(ctx, "b.jpg")
	if err != nil || f[0].Name != "" {
		t.Fatalf("obsolete XMP label retained %+v %v", f, err)
	}
	finishFace(t, l, 0)
	p, err = l.People(ctx, 0, 1, "")
	if err != nil || len(p.People) != 1 || p.People[0].Name != "Updated Name" {
		t.Fatalf("XMP rename fragmented group %+v %v", p, err)
	}
}

func TestFacePreviewRejectsUnindexedReplacement(t *testing.T) {
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(context.Background(), "a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	writeSizedJPEG(t, filepath.Join(l.Root(), "a.jpg"), 160, 90, color.Black)
	if _, err = l.FaceThumbnail(context.Background(), faces[0].ID); err == nil {
		t.Fatal("cropped replacement using stale face region")
	}
}

func TestFaceNamedGroupSurvivesModelChange(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	f, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := f[0].PersonID
	if err := l.RenamePerson(ctx, id, "Manual Name"); err != nil {
		t.Fatal(err)
	}
	if err := l.PrepareFaceQueue(ctx, "next-model"); err != nil {
		t.Fatal(err)
	}
	job, err := l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = l.CommitFaceResult(ctx, job, facerec.Result{Model: job.Model, Faces: []facerec.Detection{faceDetection(0)}}); err != nil {
		t.Fatal(err)
	}
	f, err = l.AutomaticFaces(ctx, "a.jpg")
	if err != nil || len(f) != 1 || f[0].PersonID != id || f[0].Name != "Manual Name" {
		t.Fatalf("manual name lost %+v %v", f, err)
	}
}
func TestFaceAmbiguousXMPNameStaysUnassigned(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	for _, path := range []string{"a.jpg", "b.jpg"} {
		f, _ := l.AutomaticFaces(ctx, path)
		if err := l.RenamePerson(ctx, f[0].PersonID, "Alex"); err != nil {
			t.Fatal(err)
		}
	}
	writeXMPFace(t, filepath.Join(l.Root(), "c.jpg"), "Alex", .35, .35, .3, .3)
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	finishFace(t, l, 0)
	f, err := l.AutomaticFaces(ctx, "c.jpg")
	if err != nil || len(f) != 1 || f[0].Name != "" {
		t.Fatalf("ambiguous XMP chose an identity %+v %v", f, err)
	}
}

func TestFacesConcurrentIndexAndAnalysis(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg", "d.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("seed: %+v, %v", faces, err)
	}
	id := faces[0].PersonID
	if err = l.RenamePerson(ctx, id, "Manuell benannt"); err != nil {
		t.Fatal(err)
	}
	indexed := make(chan error, 1)
	go func() {
		for n := 0; n < 8; n++ {
			if _, err := l.RebuildIndex(ctx); err != nil {
				indexed <- err
				return
			}
		}
		indexed <- nil
	}()
	for n := 0; n < 3; n++ {
		job, err := l.NextFaceJob(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// A concurrent index write may invalidate the transaction snapshot. The
		// persistent job must remain available and succeed on a delayed retry.
		for attempt := 0; attempt < 10; attempt++ {
			err = l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(0)}})
			if err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = <-indexed; err != nil {
		t.Fatal(err)
	}
	page, err := l.People(ctx, id, 1, "")
	if err != nil || len(page.Faces) != 4 || page.Name != "Manuell benannt" {
		t.Fatalf("concurrent indexing changed assignments: %+v, %v", page, err)
	}
	if _, err = l.NextFaceJob(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("completed photos were requeued: %v", err)
	}
}

func TestFacesConservativeMatching(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg", "d.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	a, _ := l.AutomaticFaces(ctx, "a.jpg")
	b, _ := l.AutomaticFaces(ctx, "b.jpg")
	for _, tc := range []struct {
		path   string
		vector []float32
	}{
		// Both existing people exceed the similarity threshold, but neither has
		// enough separation to justify an automatic identity assignment.
		{"c.jpg", []float32{0.70710677, 0.70710677}},
		// A single nearest person still must clear the absolute threshold.
		{"d.jpg", []float32{0.54, 0, 0.841665}},
	} {
		job, err := l.NextFaceJob(ctx)
		if err != nil || job.Path != tc.path {
			t.Fatalf("job: %+v, %v", job, err)
		}
		detection := faceDetection(0)
		clear(detection.Embedding)
		copy(detection.Embedding, tc.vector)
		if err = l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{detection}}); err != nil {
			t.Fatal(err)
		}
		faces, err := l.AutomaticFaces(ctx, tc.path)
		if err != nil || len(faces) != 1 {
			t.Fatalf("faces: %+v, %v", faces, err)
		}
		if faces[0].PersonID == a[0].PersonID || faces[0].PersonID == b[0].PersonID || faces[0].Name != "" {
			t.Fatalf("uncertain match assigned to existing person: %+v", faces[0])
		}
	}
	page, err := l.People(ctx, 0, 1, "")
	if err != nil || len(page.People) != 4 {
		t.Fatalf("groups must remain separate: %+v, %v", page, err)
	}
}
