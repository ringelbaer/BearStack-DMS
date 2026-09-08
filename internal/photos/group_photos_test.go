package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/facerec"
)

func finishGroupPhoto(t *testing.T, l *Library, count, offset int) (FaceJob, GroupPhoto) {
	t.Helper()
	ctx := context.Background()
	if err := l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	faces := make([]facerec.Detection, count)
	for i := range faces {
		faces[i] = faceDetection((offset + i) % 128)
		faces[i].X = .05 + float64(i%4)*.23
		faces[i].Y = .05 + float64(i/4)*.25
		faces[i].Width = .18
		faces[i].Height = .2
	}
	if err := l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: faces}); err != nil {
		t.Fatal(err)
	}
	photo, err := l.GroupPhoto(ctx, job.Path)
	if err != nil {
		t.Fatal(err)
	}
	return job, photo
}

func TestGroupPhotosThresholdCursorAndCurrentPhoto(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a/five.jpg", "b/six.jpg", "c/seven.jpg")
	finishGroupPhoto(t, l, 5, 0)
	_, six := finishGroupPhoto(t, l, 6, 10)
	_, seven := finishGroupPhoto(t, l, 7, 20)
	next, err := l.NextGroupPhoto(ctx, "", 5)
	if err != nil || next == nil || next.Path != six.Path {
		t.Fatalf("strict threshold: %+v %v", next, err)
	}
	if len(next.Faces) != 6 || next.Remaining != 6 {
		t.Fatalf("faces %+v", next)
	}
	next, err = l.NextGroupPhoto(ctx, six.Path, 5)
	if err != nil || next == nil || next.Path != seven.Path {
		t.Fatalf("skip: %+v %v", next, err)
	}
	next, err = l.NextGroupPhoto(ctx, seven.Path, 5)
	if err != nil || next != nil {
		t.Fatalf("end: %+v %v", next, err)
	}
	next, err = l.NextGroupPhoto(ctx, "", 5)
	if err != nil || next == nil || next.Path != six.Path {
		t.Fatal("skipping changed stored faces")
	}
	if err := l.RenamePerson(ctx, six.Faces[0].PersonID, "Ada"); err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{six.Faces[1].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	current, err := l.GroupPhoto(ctx, six.Path)
	if err != nil || current.Remaining != 4 || len(current.Faces) != 6 || current.Revision == six.Revision {
		t.Fatalf("current below threshold: %+v %v", current, err)
	}
	if !current.Faces[1].Ignored || current.Faces[0].Name != "Ada" {
		t.Fatal("all recognized faces must remain visible")
	}
	next, err = l.NextGroupPhoto(ctx, "", 5)
	if err != nil || next == nil || next.Path != seven.Path {
		t.Fatalf("processed faces counted: %+v %v", next, err)
	}
	if next, err := l.NextGroupPhoto(ctx, "", 7); err != nil || next != nil {
		t.Fatalf("equal threshold accepted %+v %v", next, err)
	}
	for _, minimum := range []int{-1, 256} {
		if _, err := l.NextGroupPhoto(ctx, "", minimum); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("minimum %d: %v", minimum, err)
		}
	}
	for _, path := range []string{"../a.jpg", "/a.jpg", "a//b.jpg"} {
		if _, err := l.NextGroupPhoto(ctx, path, 5); !errors.Is(err, ErrLabelInvalid) {
			t.Fatalf("path %q: %v", path, err)
		}
	}
}

func TestGroupPhotoIgnoreProtectsNamedAndOtherPhotos(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	_, a := finishGroupPhoto(t, l, 6, 0)
	_, b := finishGroupPhoto(t, l, 6, 0)
	if a.Faces[0].PersonID != b.Faces[0].PersonID {
		t.Fatal("fixture must share person groups")
	}
	if err := l.RenamePerson(ctx, a.Faces[0].PersonID, "Ada"); err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{a.Faces[2].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetFaceFavorite(ctx, a.Faces[1].ID, a.Faces[1].PersonID, true); err != nil {
		t.Fatal(err)
	}
	a, err := l.GroupPhoto(ctx, a.Path)
	if err != nil {
		t.Fatal(err)
	}
	count, err := l.IgnoreGroupPhoto(ctx, a.Path, a.Revision)
	if err != nil || count != 4 {
		t.Fatalf("ignore %d %v", count, err)
	}
	current, err := l.GroupPhoto(ctx, a.Path)
	if err != nil || current.Remaining != 0 || len(current.Faces) != 6 {
		t.Fatalf("ignored photo %+v %v", current, err)
	}
	if current.Faces[0].Ignored || current.Faces[0].Name != "Ada" {
		t.Fatal("named face changed")
	}
	if !current.Faces[1].Ignored || !current.Faces[1].Favorite {
		t.Fatal("favorite/ignore semantics changed")
	}
	active, err := l.AutomaticFaces(ctx, b.Path)
	if err != nil || len(active) != 6 || active[0].Name != "Ada" {
		t.Fatalf("other photo changed %+v %v", active, err)
	}
	for _, id := range selectedReferences(t, l) {
		if id == a.Faces[1].ID {
			t.Fatal("ignored favorite remains a reference")
		}
	}
	if _, err := l.IgnoreGroupPhoto(ctx, a.Path, a.Revision); !errors.Is(err, ErrGroupPhotoChanged) {
		t.Fatalf("stale repeat: %v", err)
	}
}

func TestGroupPhotoIgnoreConflictAndRollback(t *testing.T) {
	for _, scenario := range []string{"named", "reassigned", "reanalysis", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a.jpg")
			job, photo := finishGroupPhoto(t, l, 6, 0)
			switch scenario {
			case "named":
				if err := l.RenamePerson(ctx, photo.Faces[0].PersonID, "Concurrent"); err != nil {
					t.Fatal(err)
				}
			case "reassigned":
				if err := l.EditFaces(ctx, []int64{photo.Faces[0].ID}, 0, false, ""); err != nil {
					t.Fatal(err)
				}
			case "reanalysis":
				if err := l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{faceDetection(0)}}); err != nil {
					t.Fatal(err)
				}
			case "rollback":
				if _, err := l.index.db.Exec(`CREATE TRIGGER fail_group_ignore BEFORE UPDATE OF ignored ON photo_faces WHEN new.id=` + fmt.Sprint(photo.Faces[3].ID) + ` BEGIN SELECT RAISE(ABORT,'injected write failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			_, err := l.IgnoreGroupPhoto(ctx, photo.Path, photo.Revision)
			if scenario == "rollback" {
				if err == nil {
					t.Fatal("expected database failure")
				}
			} else if !errors.Is(err, ErrGroupPhotoChanged) {
				t.Fatalf("conflict: %v", err)
			}
			var ignored int
			if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces WHERE ignored=1`).Scan(&ignored); err != nil || ignored != 0 {
				t.Fatalf("partial mutation %d %v", ignored, err)
			}
		})
	}
}

func TestGroupPhotoVisibilityAndSourceChanges(t *testing.T) {
	for _, scenario := range []string{"private", "deleted", "replaced", "symlink", "imported-name"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "a/one.jpg", "b/two.jpg")
			_, a := finishGroupPhoto(t, l, 6, 0)
			_, b := finishGroupPhoto(t, l, 6, 20)
			switch scenario {
			case "private":
				if err := os.WriteFile(filepath.Join(l.Root(), "a/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := os.Remove(filepath.Join(l.Root(), a.Path)); err != nil {
					t.Fatal(err)
				}
			case "replaced":
				f, err := os.OpenFile(filepath.Join(l.Root(), a.Path), os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.Write([]byte("changed")); err != nil {
					t.Fatal(err)
				}
				f.Close()
			case "symlink":
				original := filepath.Join(l.Root(), "a")
				moved := filepath.Join(t.TempDir(), "moved")
				if err := os.Rename(original, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, original); err != nil {
					t.Fatal(err)
				}
			case "imported-name":
				if _, err := l.index.db.Exec(`UPDATE photo_people SET name='Imported',name_fold='imported',name_source=? WHERE id IN (SELECT person_id FROM photo_faces WHERE path=?)`, b.Path, a.Path); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(l.Root(), "b/.adminonly"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			next, err := l.NextGroupPhoto(ctx, "", 5)
			want := b.Path
			if scenario == "imported-name" {
				want = a.Path
			}
			if err != nil || next == nil || next.Path != want {
				t.Fatalf("next %+v %v want %s", next, err, want)
			}
			if scenario == "imported-name" {
				for _, f := range next.Faces {
					if f.Name != "" {
						t.Fatal("private imported name exposed")
					}
				}
				return
			}
			if _, err := l.IgnoreGroupPhoto(ctx, a.Path, a.Revision); err == nil {
				t.Fatal("unsafe source mutation succeeded")
			}
		})
	}
}

func TestGroupPhotosMigrationAndQueryPlan(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	_, photo := finishGroupPhoto(t, l, 6, 0)
	if _, err := l.index.db.Exec(`DROP INDEX idx_face_group_candidates; UPDATE schema_migrations SET version=22 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	root, cache, db := l.Root(), l.CacheDir(), l.DBPath()
	l.Close()
	l, err := New(root, cache, db, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	next, err := l.NextGroupPhoto(ctx, "", 5)
	if err != nil || next == nil || next.Revision != photo.Revision {
		t.Fatalf("migration lost state %+v %v", next, err)
	}
	if _, err := l.NextFaceJob(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("index migration requeued inference")
	}
	rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN SELECT f.path FROM photo_faces f INDEXED BY idx_face_group_candidates CROSS JOIN photo_people p ON p.id=f.person_id CROSS JOIN media_index m ON m.path=f.path WHERE f.path>? AND f.ignored=0 AND (p.name='' OR p.name_source<>'') AND m.admin_only=0 AND m.type='image' GROUP BY f.path HAVING count(*)>? ORDER BY f.path LIMIT 1`, "", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan string
	for rows.Next() {
		var a, b, c int
		var detail string
		if err := rows.Scan(&a, &b, &c, &detail); err != nil {
			t.Fatal(err)
		}
		plan += detail + "\n"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "idx_face_group_candidates (path>?)") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatalf("unbounded candidate plan: %s", plan)
	}
}

func TestGroupPhotoConcurrentIgnore(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	_, photo := finishGroupPhoto(t, l, 6, 0)
	type outcome struct {
		count int
		err   error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			count, err := l.IgnoreGroupPhoto(ctx, photo.Path, photo.Revision)
			results <- outcome{count, err}
		}()
	}
	saved, conflicts := 0, 0
	for range 2 {
		result := <-results
		if result.err == nil && result.count == 6 {
			saved++
		} else if errors.Is(result.err, ErrGroupPhotoChanged) {
			conflicts++
		} else {
			t.Fatalf("concurrent result: %+v", result)
		}
	}
	if saved != 1 || conflicts != 1 {
		t.Fatalf("saved=%d conflicts=%d", saved, conflicts)
	}
	current, err := l.GroupPhoto(ctx, photo.Path)
	if err != nil || current.Remaining != 0 {
		t.Fatalf("result %+v %v", current, err)
	}
}
