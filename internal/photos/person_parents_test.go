package photos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func parentFixture(t *testing.T) (*Library, []int64) {
	t.Helper()
	ctx := context.Background()
	l := faceLibrary(t, "a/a.jpg", "b/b.jpg", "c/c.jpg", "d/d.jpg", "e/e.jpg")
	ids := []int64{}
	for i, path := range []string{"a/a.jpg", "b/b.jpg", "c/c.jpg", "d/d.jpg", "e/e.jpg"} {
		finishFace(t, l, i)
		faces, err := l.AutomaticFaces(ctx, path)
		if err != nil || len(faces) != 1 {
			t.Fatalf("faces: %v %v", faces, err)
		}
		id := faces[0].PersonID
		ids = append(ids, id)
		if err := l.RenamePerson(ctx, id, fmt.Sprintf("Person %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	return l, ids
}

func TestPersonParentsConcurrentCycle(t *testing.T) {
	l, ids := parentFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, pair := range [][2]int64{{ids[0], ids[1]}, {ids[1], ids[0]}} {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- l.SetPersonParents(context.Background(), pair[0], pair[1], 0) }()
	}
	wg.Wait()
	close(errs)
	saved, rejected := 0, 0
	for err := range errs {
		if err == nil {
			saved++
		} else if errors.Is(err, ErrPersonParents) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if saved != 1 || rejected != 1 {
		t.Fatalf("saved=%d rejected=%d", saved, rejected)
	}
}

func TestPersonParentsValidationAndVisibility(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	child, mother, father := ids[0], ids[1], ids[2]
	if err := l.SetPersonParents(ctx, child, mother, father); err != nil {
		t.Fatal(err)
	}
	for _, v := range [][3]int64{{child, child, 0}, {child, mother, mother}, {mother, child, 0}, {child, 999999, 0}, {child, -1, 0}} {
		if err := l.SetPersonParents(ctx, v[0], v[1], v[2]); !errors.Is(err, ErrPersonParents) {
			t.Fatalf("accepted %v: %v", v, err)
		}
	}
	if err := l.SetPersonParents(ctx, mother, ids[3], 0); err != nil {
		t.Fatal(err)
	}
	if err := l.SetPersonParents(ctx, ids[3], child, 0); !errors.Is(err, ErrPersonParents) {
		t.Fatalf("cycle: %v", err)
	}
	if err := l.RenamePerson(ctx, father, "Vater umbenannt"); err != nil {
		t.Fatal(err)
	}
	p, err := l.PersonParents(ctx, child)
	if err != nil || p.Mother.ID != mother || p.Father.Name != "Vater umbenannt" {
		t.Fatalf("parents: %+v %v", p, err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "b/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	p, err = l.PersonParents(ctx, child)
	if err != nil || p.Mother != nil || p.Father == nil {
		t.Fatalf("private parent leaked: %+v %v", p, err)
	}
	if err := l.SetPersonParents(ctx, child, mother, 0); !errors.Is(err, ErrPersonParents) {
		t.Fatalf("private accepted: %v", err)
	}
	if err := l.SetPersonParents(ctx, child, 0, 0); err != nil {
		t.Fatal(err)
	}
	p, err = l.PersonParents(ctx, child)
	if err != nil || p.Mother != nil || p.Father != nil {
		t.Fatalf("clear: %+v %v", p, err)
	}
	if err := l.RenamePerson(ctx, ids[4], ""); err != nil {
		t.Fatal(err)
	}
	if err := l.SetPersonParents(ctx, child, ids[4], 0); !errors.Is(err, ErrPersonParents) {
		t.Fatalf("unnamed accepted: %v", err)
	}
}

func TestPersonParentsMergeAndRollback(t *testing.T) {
	ctx := context.Background()
	l, ids := parentFixture(t)
	if err := l.SetPersonParents(ctx, ids[0], ids[1], ids[2]); err != nil {
		t.Fatal(err)
	}
	if err := l.SetPersonParents(ctx, ids[1], ids[3], 0); err != nil {
		t.Fatal(err)
	}
	if err := l.MergePeople(ctx, ids[1], ids[4]); err != nil {
		t.Fatal(err)
	}
	p, err := l.PersonParents(ctx, ids[0])
	if err != nil || p.Mother.ID != ids[4] {
		t.Fatalf("parent reference not moved: %+v %v", p, err)
	}
	p, err = l.PersonParents(ctx, ids[4])
	if err != nil || p.Mother.ID != ids[3] {
		t.Fatalf("parent metadata not moved: %+v %v", p, err)
	}
	if err := l.MergePeople(ctx, ids[3], ids[0]); !errors.Is(err, ErrParentMerge) {
		t.Fatalf("merge cycle accepted: %v", err)
	}
	if err := l.MergePeople(ctx, ids[2], ids[4]); !errors.Is(err, ErrParentMerge) {
		t.Fatalf("duplicate parent merge accepted: %v", err)
	}
	p, err = l.PersonParents(ctx, ids[0])
	if err != nil || p.Mother.ID != ids[4] || p.Father.ID != ids[2] {
		t.Fatalf("partial merge: %+v %v", p, err)
	}
	if _, err := l.index.db.Exec(`DELETE FROM photo_people WHERE id=?`, ids[2]); err != nil {
		t.Fatal(err)
	}
	p, err = l.PersonParents(ctx, ids[0])
	if err != nil || p.Father != nil {
		t.Fatalf("orphan: %+v %v", p, err)
	}
}
