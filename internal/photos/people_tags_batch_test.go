package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAddPeopleTagsAtomicAdditiveAndVisible(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "secret/c.jpg")
	for axis := 0; axis < 3; axis++ {
		finishFace(t, l, axis)
	}
	ids := []int64{}
	for _, path := range []string{"a.jpg", "b.jpg", "secret/c.jpg"} {
		faces, err := l.AutomaticFaces(ctx, path)
		if err != nil || len(faces) != 1 {
			t.Fatalf("faces: %v %v", faces, err)
		}
		ids = append(ids, faces[0].PersonID)
	}
	if _, err := l.SetPersonTags(ctx, ids[0], []string{"existing"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		count, err := l.AddPeopleTags(ctx, []int64{ids[0], ids[1], ids[0]}, []string{" Family ", "family", "Travel"})
		if err != nil || count != 2 {
			t.Fatalf("batch: %d %v", count, err)
		}
	}
	assertTags := func(id int64, want []string) {
		t.Helper()
		got, err := l.personTags(ctx, id)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("tags for %d: %v %v; want %v", id, got, err, want)
		}
	}
	assertTags(ids[0], []string{"existing", "family", "travel"})
	assertTags(ids[1], []string{"family", "travel"})
	assertTags(ids[2], []string{})
	media, err := l.MediaContext(ctx, "a.jpg")
	if err != nil || len(media.Tags) != 0 {
		t.Fatalf("photo tags changed: %v %v", media.Tags, err)
	}
	if _, err := l.AddPeopleTags(ctx, []int64{ids[0], 999999}, []string{"orphan"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing person: %v", err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "secret/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddPeopleTags(ctx, []int64{ids[0], ids[2]}, []string{"orphan"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("private person: %v", err)
	}
	if _, err := l.GetTag(ctx, "orphan"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("orphan tag created: %v", err)
	}
	assertTags(ids[0], []string{"existing", "family", "travel"})
	// A write failure must roll back both newly created tags and earlier assignments.
	if _, err := l.index.db.Exec(`CREATE TRIGGER fail_batch BEFORE INSERT ON person_tag_index WHEN NEW.tag='rollback-z' BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddPeopleTags(ctx, ids[:2], []string{"rollback-a", "rollback-z"}); err == nil {
		t.Fatal("write failure accepted")
	}
	assertTags(ids[1], []string{"family", "travel"})
	if _, err := l.GetTag(ctx, "rollback-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("failed batch created tag: %v", err)
	}
}

func TestAddPeopleTagsBounds(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg")
	finishFace(t, l, 0)
	faces, _ := l.AutomaticFaces(ctx, "a.jpg")
	id := faces[0].PersonID
	for _, tc := range []struct {
		ids  []int64
		tags []string
	}{
		{nil, []string{"tag"}}, {[]int64{0}, []string{"tag"}}, {[]int64{id}, nil},
		{[]int64{id}, []string{"  "}}, {make([]int64, 501), []string{"tag"}},
		{[]int64{id}, make([]string, 101)}, {[]int64{id}, []string{strings.Repeat("x", 4097)}},
	} {
		if _, err := l.AddPeopleTags(ctx, tc.ids, tc.tags); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	tags := make([]string, 100)
	for i := range tags {
		tags[i] = fmt.Sprintf("tag-%03d", i)
	}
	if _, err := l.AddPeopleTags(ctx, []int64{id}, tags); err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddPeopleTags(ctx, []int64{id}, []string{tags[0]}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.AddPeopleTags(ctx, []int64{id}, []string{"overflow"}); err == nil {
		t.Fatal("union limit exceeded")
	}
	if _, err := l.GetTag(ctx, "overflow"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("overflow tag created: %v", err)
	}
}
