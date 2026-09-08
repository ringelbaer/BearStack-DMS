package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestPersonVisibilityIsScopedAndChecksNewMarkers(t *testing.T) {
	for _, operation := range []string{"detail", "people", "search", "suggestions", "picker", "rename"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			l := faceLibrary(t, "one/a.jpg", "other/b.jpg")
			finishFace(t, l, 0)
			finishFace(t, l, 1)
			faces, err := l.AutomaticFaces(ctx, "one/a.jpg")
			if err != nil || len(faces) != 1 {
				t.Fatalf("faces: %v %v", faces, err)
			}
			id := faces[0].PersonID
			if err := l.RenamePerson(ctx, id, "Alice"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(l.Root(), "other/.adminonly"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			trace := NewListTrace()
			ctx = ContextWithListTrace(ctx, trace)
			switch operation {
			case "detail":
				_, err = l.LabelPerson(ctx, id, 0)
			case "people":
				_, err = l.People(ctx, id, 1, "", false, false)
			case "search":
				_, err = l.People(ctx, 0, 1, "Alice", true, false)
			case "suggestions":
				_, err = l.LabelSuggestions(ctx, "Alice", false)
			case "picker":
				_, err = l.SuggestPeople(ctx, "Alice")
			case "rename":
				err = l.RenamePerson(ctx, id, "Alice")
			}
			if err != nil {
				t.Fatal(err)
			}
			var unrelated bool
			if err := l.index.db.QueryRow(`SELECT admin_only FROM media_index WHERE path='other/b.jpg'`).Scan(&unrelated); err != nil {
				t.Fatal(err)
			}
			if unrelated {
				t.Fatal("scanned an unrelated person's directory")
			}
			for _, step := range trace.Snapshot().Steps {
				if step.Name == "photos.faces.visibility" {
					for _, field := range step.Fields {
						if field.Key == "directories" && field.Value != "1" {
							t.Fatalf("checked %s directories", field.Value)
						}
					}
				}
			}
			// No TTL: a marker created after the successful request is immediate.
			if err := os.WriteFile(filepath.Join(l.Root(), "one/.adminonly"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := l.LabelPerson(ctx, id, 0); err == nil {
				t.Fatal("new private marker ignored")
			}
		})
	}
}

func TestPersonVisibilityIncludesNameSourceWithoutFaces(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "one/a.jpg", "source/z.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "one/a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces: %v %v", faces, err)
	}
	id := faces[0].PersonID
	if _, err := l.index.db.Exec(`UPDATE photo_people SET name='Imported',name_fold='imported',manual_name=0,name_source='source/z.jpg' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "source/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	p, err := l.LabelPerson(ctx, id, 0)
	if err != nil || p.Name != "" || p.Count != 1 {
		t.Fatalf("name provenance leaked: %+v %v", p, err)
	}
}

func TestFaceDirectoryVisibilityChecksAncestorsAndFailsClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "parent/child"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "parent/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	v := newFaceDirectoryVisibility(root)
	for _, dir := range []string{"parent/child", "missing", "link", "../outside"} {
		if !v.private(dir) {
			t.Errorf("exposed %s", dir)
		}
	}
	if v.private("") {
		t.Fatal("public root became private")
	}
}

func TestFaceVisibilitySharesOnlyRunningChecks(t *testing.T) {
	var state faceVisibilityState
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- state.check(context.Background(), "same", func() error { calls.Add(1); close(started); <-release; return nil })
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := state.check(ctx, "same", func() error { calls.Add(1); return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waiter cancellation: %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("overlapping calls repeated scan")
	}
	if err := state.check(context.Background(), "same", func() error { calls.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("completed result was cached")
	}
}

type observedVisibilityWait struct {
	context.Context
	waiting chan struct{}
}

func TestFaceVisibilityOfflineDirectoryPreservesFaces(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "album/a.jpg")
	finishFace(t, l, 0)
	faces, err := l.AutomaticFaces(ctx, "album/a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces %v %v", faces, err)
	}
	offline := filepath.Join(t.TempDir(), "offline")
	if err := os.Rename(filepath.Join(l.Root(), "album"), offline); err != nil {
		t.Fatal(err)
	}
	if _, err := l.LabelPerson(ctx, faces[0].PersonID, 0); err == nil {
		t.Fatal("offline directory must block disclosure")
	}
	var count int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("offline directory destroyed faces: %d %v", count, err)
	}
	if err := os.Rename(offline, filepath.Join(l.Root(), "album")); err != nil {
		t.Fatal(err)
	}
	if p, err := l.LabelPerson(ctx, faces[0].PersonID, 0); err != nil || p.Count != 1 {
		t.Fatalf("restored directory: %+v %v", p, err)
	}
}

func (c observedVisibilityWait) Done() <-chan struct{} {
	c.waiting <- struct{}{}
	return c.Context.Done()
}

func TestFaceVisibilityWaitersShareFreshScan(t *testing.T) {
	var state faceVisibilityState
	var calls atomic.Int32
	firstStarted, firstRelease := make(chan struct{}), make(chan struct{})
	secondStarted, secondRelease := make(chan struct{}), make(chan struct{})
	results := make(chan error, 3)
	go func() {
		results <- state.check(context.Background(), "same", func() error {
			calls.Add(1)
			close(firstStarted)
			<-firstRelease
			return nil
		})
	}()
	<-firstStarted
	waiting := make(chan struct{}, 4)
	for range 2 {
		go func() {
			results <- state.check(observedVisibilityWait{Context: context.Background(), waiting: waiting}, "same", func() error {
				calls.Add(1)
				close(secondStarted)
				<-secondRelease
				return nil
			})
		}()
	}
	<-waiting
	<-waiting
	close(firstRelease)
	<-secondStarted // Neither new caller may reuse the older scan.
	<-waiting       // The other caller is now waiting for the fresh scan.
	close(secondRelease)
	for range 3 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("%d scans, want old scan plus one shared fresh scan", calls.Load())
	}
}
