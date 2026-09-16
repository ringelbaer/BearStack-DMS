package photos

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPeopleFolderPhotoCountSortingBeforePagination(t *testing.T) {
	ctx := context.Background()
	paths := []string{"elsewhere/a.jpg", "elsewhere/b.jpg", "trip/a.jpg", "trip/aa.jpg", "trip/b.jpg", "trip/c.jpg", "trip/ignored.jpg", "trip/private/a.jpg", "trip/u.jpg"}
	l := faceLibrary(t, paths...)
	axes := []int{0, 0, 1, 0, 1, 2, 2, 2, 3}
	for _, axis := range axes {
		finishFace(t, l, axis)
	}
	faces := map[string]RecognizedFace{}
	for _, path := range paths {
		found, err := l.AutomaticFaces(ctx, path)
		if err != nil || len(found) != 1 {
			t.Fatalf("%s: %v %v", path, found, err)
		}
		faces[path] = found[0]
	}
	for path, name := range map[string]string{"elsewhere/a.jpg": "Ada", "trip/a.jpg": "Zoe", "trip/c.jpg": "Bea"} {
		id := faces[path].PersonID
		if err := l.RenamePerson(ctx, id, name); err != nil {
			t.Fatal(err)
		}
		if _, err := l.SetPersonTags(ctx, id, []string{"Family"}); err != nil {
			t.Fatal(err)
		}
	}
	// Same person detected twice in a photo still contributes just one image.
	if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, faces["trip/c.jpg"].ID); err != nil {
		t.Fatal(err)
	}
	if err := l.EditFaces(ctx, []int64{faces["trip/ignored.jpg"].ID}, 0, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "trip/private/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, sort, query string
		names             []string
		counts            []int
	}{
		{".people/all", "ascending_count", "", []string{"Bea", "Zoe", "Ada"}, []int{1, 2, 3}},
		{".people/all", "descending_count", "", []string{"Ada", "Zoe", "Bea"}, []int{3, 2, 1}},
		{personTagPath("family"), "descending_count", "", []string{"Ada", "Zoe", "Bea"}, []int{3, 2, 1}},
		{personTagPath("family"), "ascending_count", "a", []string{"Bea", "Ada"}, []int{1, 3}},
		{DirectoryPeoplePath("trip"), "ascending_count", "", []string{"Ada", "Bea", "Zoe"}, []int{1, 1, 2}},
		{DirectoryPeoplePath("trip"), "descending_count", "", []string{"Zoe", "Ada", "Bea"}, []int{2, 1, 1}},
		{DirectoryPeoplePath("trip"), "descending_count", "a", []string{"Ada", "Bea"}, []int{1, 1}},
	} {
		for page := 1; page <= len(tc.names)+1; page++ {
			got, err := l.List(ctx, ListOptions{Path: tc.path, Sort: tc.sort, Query: tc.query, Page: page, FolderPageSize: 1})
			if err != nil {
				t.Fatalf("%s %s page %d: %v", tc.path, tc.sort, page, err)
			}
			if got.FolderTotal != len(tc.names) || got.FolderHasNext != (page < len(tc.names)) {
				t.Fatalf("pagination: %+v", got)
			}
			if page > len(tc.names) {
				if len(got.Folders) != 0 {
					t.Fatalf("extra page: %+v", got)
				}
			} else if len(got.Folders) != 1 || got.Folders[0].Name != tc.names[page-1] || got.Folders[0].MediaCount != tc.counts[page-1] {
				t.Fatalf("%s %s q=%s page %d: %+v", tc.path, tc.sort, tc.query, page, got.Folders)
			}
		}
	}
}
