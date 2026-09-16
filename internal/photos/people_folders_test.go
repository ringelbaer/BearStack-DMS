package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"bearstack/internal/facerec"
)

func TestPeopleFoldersTagsPortraitsAndGallery(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg", "c.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	a, _ := l.AutomaticFaces(ctx, "a.jpg")
	b, _ := l.AutomaticFaces(ctx, "b.jpg")
	c, _ := l.AutomaticFaces(ctx, "c.jpg")
	id := a[0].PersonID
	if err := l.RenamePerson(ctx, id, "Zoe"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetPersonTags(ctx, id, []string{"Familie", " familie ", "Reise / 2026"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetPersonTags(ctx, c[0].PersonID, []string{"Familie"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SaveTag(ctx, "ohne person"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetFaceFavorite(ctx, b[0].ID, id, true); err != nil {
		t.Fatal(err)
	}
	root, err := l.List(ctx, ListOptions{IncludePeopleFolders: true, FolderPreviewSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	f := root.Folders[0]
	if f.Path != PeopleFolderPath || !f.Virtual || f.DirCount != 2 || len(f.Previews) != 2 || f.Previews[0].FaceID != b[0].ID {
		t.Fatalf("root: %+v", f)
	}
	directory, err := l.List(ctx, ListOptions{Path: PeopleFolderPath, FolderPreviewSize: 1})
	if err != nil || len(directory.Folders) != 3 || directory.Folders[0].Name != "Alle" || directory.Folders[0].DirCount != 1 || len(directory.Folders[0].Previews) != 1 || directory.Folders[0].Previews[0].FaceID != b[0].ID {
		t.Fatalf("tags: %+v %v", directory, err)
	}
	for _, folder := range directory.Folders {
		if folder.Name == "ohne person" {
			t.Fatal("unused tag leaked")
		}
	}
	tagged, err := l.List(ctx, ListOptions{Path: personTagPath("reise / 2026")})
	if err != nil || len(tagged.Folders) != 1 || tagged.Folders[0].Name != "Zoe" || tagged.Folders[0].MediaCount != 2 {
		t.Fatalf("people: %+v %v", tagged, err)
	}
	// Duplicate detections in one image must not duplicate gallery entries.
	if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, a[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`UPDATE media_index SET captured_at=? WHERE path='a.jpg'`, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`UPDATE media_index SET captured_at=? WHERE path='b.jpg'`, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sort, path string }{{"ascending_date", "a.jpg"}, {"descending_date", "b.jpg"}, {"ascending_name", "a.jpg"}, {"descending_name", "b.jpg"}} {
		page, err := l.List(ctx, ListOptions{Path: tagged.Folders[0].Path, Sort: tc.sort, PageSize: 1})
		if err != nil || page.Total != 2 || len(page.Media) != 1 || page.Media[0].Path != tc.path || !page.HasNext || page.ParentPath != personTagPath("reise / 2026") {
			t.Fatalf("%s: %+v %v", tc.sort, page, err)
		}
	}
	for _, q := range []string{"file_name:a.jpg", "file_name:a.jpg OR file_name:c.jpg", "-tag:missing"} {
		page, err := l.List(ctx, ListOptions{Path: PersonFolderPath(id), Query: q})
		want := 1
		if q == "-tag:missing" {
			want = 2
		}
		if err != nil || page.Total != want {
			t.Fatalf("scoped search %q: %+v %v", q, page, err)
		}
	}
	media, err := l.MediaContext(ctx, "a.jpg")
	if err != nil || len(media.Tags) != 0 {
		t.Fatalf("person tags modified photo: %+v %v", media, err)
	}
}

func TestPersonTagsMergeRenameDeleteAndMigration(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "a.jpg", "b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	a, _ := l.AutomaticFaces(ctx, "a.jpg")
	b, _ := l.AutomaticFaces(ctx, "b.jpg")
	for i, id := range []int64{a[0].PersonID, b[0].PersonID} {
		if _, err := l.SetPersonTags(ctx, id, []string{"shared", fmt.Sprintf("tag%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.MergePeople(ctx, a[0].PersonID, b[0].PersonID); err != nil {
		t.Fatal(err)
	}
	tags, err := l.personTags(ctx, b[0].PersonID)
	if err != nil || !reflect.DeepEqual(tags, []string{"shared", "tag0", "tag1"}) {
		t.Fatalf("merged: %v %v", tags, err)
	}
	if tags, err := l.personTags(ctx, a[0].PersonID); err != nil || len(tags) != 0 {
		t.Fatalf("orphaned tags: %v %v", tags, err)
	}
	if _, err := l.RenameTag(ctx, "tag0", "tag1"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.DeleteTag(ctx, "shared"); err != nil {
		t.Fatal(err)
	}
	page, err := l.People(ctx, b[0].PersonID, 1, "", false, false)
	if err != nil || !reflect.DeepEqual(page.Tags, []string{"tag1"}) {
		t.Fatalf("renamed/deleted: %v %v", page.Tags, err)
	}
	if _, err := l.SetPersonTags(ctx, b[0].PersonID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetPersonTags(ctx, 999999, []string{"orphan"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("invalid person: %v", err)
	}
	if _, err := l.GetTag(ctx, "orphan"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("non-atomic tags: %v", err)
	}
	// A v31 database acquires the new index without reindexing media or faces.
	if _, err := l.index.db.Exec(`DROP TRIGGER photo_person_tags_delete; DROP TABLE person_tag_index; UPDATE schema_migrations SET version=31 WHERE component='photos'`); err != nil {
		t.Fatal(err)
	}
	if err := runPhotoSchemaMigrations(ctx, l.index.db); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetPersonTags(ctx, b[0].PersonID, []string{"after upgrade"}); err != nil {
		t.Fatal(err)
	}
}

func TestPeopleFoldersVisibilityAndIgnored(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "public/a.jpg", "secret/b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 1)
	a, _ := l.AutomaticFaces(ctx, "public/a.jpg")
	b, _ := l.AutomaticFaces(ctx, "secret/b.jpg")
	for _, id := range []int64{a[0].PersonID, b[0].PersonID} {
		if err := l.RenamePerson(ctx, id, "Person"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.SetPersonTags(ctx, b[0].PersonID, []string{"secret-tag"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "secret/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := l.List(ctx, ListOptions{Path: PeopleFolderPath, IncludeAdminOnly: true})
	if err != nil || len(root.Folders) != 1 || root.Folders[0].DirCount != 1 {
		t.Fatalf("private group/tag leaked: %+v %v", root, err)
	}
	if _, err := l.List(ctx, ListOptions{Path: PersonFolderPath(b[0].PersonID), IncludeAdminOnly: true}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("private gallery: %v", err)
	}
	if _, err := l.SetPersonTags(ctx, b[0].PersonID, []string{"changed"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("private tags: %v", err)
	}
	options, err := l.ListTags(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range options {
		if tag.Name == "secret-tag" {
			t.Fatal("private tag option leaked")
		}
	}
	if err := l.EditFaces(ctx, []int64{a[0].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	root, err = l.List(ctx, ListOptions{Path: PeopleFolderPath})
	if err != nil || root.Folders[0].DirCount != 0 || len(root.Folders[0].Previews) != 0 {
		t.Fatalf("ignored faces leaked: %+v %v", root, err)
	}
}

func TestPeopleFoldersPaginationAndPreviewLimits(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "group.jpg")
	if err := l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := l.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	detections := make([]facerec.Detection, 70)
	for i := range detections {
		detections[i] = faceDetection(i)
	}
	if err := l.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: detections}); err != nil {
		t.Fatal(err)
	}
	faces, err := l.AutomaticFaces(ctx, "group.jpg")
	if err != nil || len(faces) != 70 {
		t.Fatalf("faces: %d %v", len(faces), err)
	}
	for i, face := range faces[:65] {
		if err := l.RenamePerson(ctx, face.PersonID, fmt.Sprintf("Person %02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for limit := 1; limit <= 4; limit++ {
		root, err := l.List(ctx, ListOptions{IncludePeopleFolders: true, FolderPreviewSize: limit})
		if err != nil || len(root.Folders[0].Previews) != 2*limit {
			t.Fatalf("limit %d: %+v %v", limit, root, err)
		}
	}
	seen := map[string]bool{}
	for page := 1; page <= 4; page++ {
		out, err := l.List(ctx, ListOptions{Path: PeopleFolderPath + "/all", Page: page, FolderPageSize: 24})
		if err != nil || out.FolderTotal != 65 || out.FolderHasNext != (page < 3) {
			t.Fatalf("page %d: %+v %v", page, out, err)
		}
		for _, folder := range out.Folders {
			if folder.Name == "Unbenannt" {
				t.Fatal("unnamed person in all folder")
			}
			if seen[folder.Path] {
				t.Fatal("duplicate person")
			}
			seen[folder.Path] = true
		}
	}
	if len(seen) != 65 {
		t.Fatalf("missing people: %d", len(seen))
	}
	for _, tc := range []struct {
		sort, query, name string
		total             int
	}{
		{"ascending_name", "", "Person 00", 65},
		{"descending_name", "", "Person 64", 65},
		{"ascending_name", "Person 06", "Person 06", 1},
		{"ascending_name", "Unbenannt", "", 0},
	} {
		out, err := l.List(ctx, ListOptions{Path: PeopleFolderPath + "/all", PageSize: 1, Sort: tc.sort, Query: tc.query})
		if err != nil || out.Total != tc.total || out.FolderTotal != tc.total || out.HasNext != (tc.total > 1) || out.FolderHasNext != out.HasNext {
			t.Fatalf("browser %s %q: %+v %v", tc.sort, tc.query, out, err)
		}
		if tc.total == 0 {
			if len(out.Folders) != 0 {
				t.Fatalf("unexpected folders: %+v", out.Folders)
			}
		} else if len(out.Folders) != 1 || out.Folders[0].Name != tc.name {
			t.Fatalf("browser order/search: %+v", out.Folders)
		}
	}
	for _, path := range []string{".people/nope", ".people/t-!", ".people/t-YQ/0", ".people/all/-1", ".people/all/1/extra"} {
		if _, err := l.List(ctx, ListOptions{Path: path}); err == nil {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
}

func TestPeopleFoldersPrivateFavoriteWithinVisiblePerson(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "public/a.jpg", "secret/b.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	a, _ := l.AutomaticFaces(ctx, "public/a.jpg")
	b, _ := l.AutomaticFaces(ctx, "secret/b.jpg")
	id := a[0].PersonID
	if _, err := l.SetPersonTags(ctx, id, []string{"family"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.SetFaceFavorite(ctx, b[0].ID, id, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "secret/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	people, err := l.List(ctx, ListOptions{Path: personTagPath("family")})
	if err != nil || len(people.Folders) != 1 || people.Folders[0].MediaCount != 1 || people.Folders[0].Previews[0].FaceID != a[0].ID {
		t.Fatalf("private favorite/count leaked: %+v %v", people, err)
	}
	gallery, err := l.List(ctx, ListOptions{Path: PersonFolderPath(id), IncludeAdminOnly: true})
	if err != nil || gallery.Total != 1 || gallery.Media[0].Path != "public/a.jpg" {
		t.Fatalf("private photo leaked: %+v %v", gallery, err)
	}
}
