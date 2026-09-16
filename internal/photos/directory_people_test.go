package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryPeopleScopePortraitsAndGallery(t *testing.T) {
	ctx := context.Background()
	paths := []string{"elsewhere/d.jpg", "trip-other/c.jpg", "trip/a.jpg", "trip/new.jpg", "trip/private/e.jpg", "trip/sub/b.jpg"}
	l := faceLibrary(t, paths...)
	for i := range paths {
		embedding := 0
		if i == 3 {
			embedding = 1
		}
		if i == 4 {
			embedding = 2
		}
		finishFace(t, l, embedding)
	}
	faces := map[string][]RecognizedFace{}
	for _, path := range paths {
		var err error
		faces[path], err = l.AutomaticFaces(ctx, path)
		if err != nil || len(faces[path]) != 1 {
			t.Fatalf("%s: %v %v", path, faces[path], err)
		}
	}
	id := faces["trip/a.jpg"][0].PersonID
	if err := l.RenamePerson(ctx, id, "Ada"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"elsewhere/d.jpg", "trip/sub/b.jpg"} {
		if _, err := l.SetFaceFavorite(ctx, faces[path][0].ID, id, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(l.root, "trip/private/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT path,directory,person_id,x,y,width,height,confidence,embedding,model FROM photo_faces WHERE id=?`, faces["trip/a.jpg"][0].ID); err != nil {
		t.Fatal(err)
	}
	path := DirectoryPeoplePath("trip")
	out, err := l.List(ctx, ListOptions{Path: path, Query: "ada", IncludeAdminOnly: true})
	if err != nil || out.FolderTotal != 1 || len(out.Folders) != 1 || out.ParentPath != "trip" {
		t.Fatalf("overview: %+v %v", out, err)
	}
	folder := out.Folders[0]
	if folder.MediaCount != 2 || folder.Previews[0].FaceID != faces["trip/sub/b.jpg"][0].ID {
		t.Fatalf("counts/portrait escaped subtree: %+v", folder)
	}
	gallery, err := l.List(ctx, ListOptions{Path: folder.Path, Sort: "ascending_name"})
	if err != nil || gallery.Total != 2 || gallery.ParentPath != path {
		t.Fatalf("gallery: %+v %v", gallery, err)
	}
	for _, media := range gallery.Media {
		if media.Path != "trip/a.jpg" && media.Path != "trip/sub/b.jpg" {
			t.Fatalf("outside media: %s", media.Path)
		}
	}
	gallery, err = l.List(ctx, ListOptions{Path: folder.Path, Query: "file_name:b.jpg OR file_name:d.jpg"})
	if err != nil || gallery.Total != 1 || gallery.Media[0].Path != "trip/sub/b.jpg" {
		t.Fatalf("scoped OR: %+v %v", gallery, err)
	}
	for page := 1; page <= 3; page++ {
		wantFolders := 0
		if page == 1 {
			wantFolders = 1
		}
		out, err = l.List(ctx, ListOptions{Path: path, Page: page, FolderPageSize: 1})
		if err != nil || out.FolderTotal != 1 || out.FolderHasNext || len(out.Folders) != wantFolders {
			t.Fatalf("page %d: %+v %v", page, out, err)
		}
	}
	if _, err := l.List(ctx, ListOptions{Path: DirectoryPeoplePath("trip/private"), IncludeAdminOnly: true}); !errors.Is(err, ErrAdminOnly()) {
		t.Fatalf("private directory: %v", err)
	}
}

func TestDirectoryPeoplePathsAndEmptyDirectories(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "space %_ &/a.jpg")
	finishFace(t, l, 0)
	out, err := l.List(ctx, ListOptions{Path: DirectoryPeoplePath("")})
	if err != nil || out.FolderTotal != 0 || len(out.Folders) != 0 {
		t.Fatalf("unnamed directory: %+v %v", out, err)
	}
	faces, err := l.AutomaticFaces(ctx, "space %_ &/a.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces: %+v %v", faces, err)
	}
	if err := l.RenamePerson(ctx, faces[0].PersonID, "Ada"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"space %_ &", ""} {
		out, err := l.List(ctx, ListOptions{Path: DirectoryPeoplePath(dir)})
		if err != nil || out.FolderTotal != 1 || out.ParentPath != dir {
			t.Fatalf("directory %q: %+v %v", dir, out, err)
		}
	}
	if err := os.Mkdir(filepath.Join(l.root, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	out, err = l.List(ctx, ListOptions{Path: DirectoryPeoplePath("empty")})
	if err != nil || out.FolderTotal != 0 {
		t.Fatalf("empty: %+v %v", out, err)
	}
	for _, path := range []string{DirectoryPeoplePath("../"), DirectoryPeoplePath(".people"), DirectoryPeoplePath("missing"), DirectoryPeoplePath("space %_ &/a.jpg"), ".people/f-!", DirectoryPeoplePath("") + "/0"} {
		if _, err := l.List(ctx, ListOptions{Path: path}); err == nil {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
}
