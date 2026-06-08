package photos

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryListsTagsFromPhotoDatabase(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album", "photo.jpg"), color.RGBA{R: 200, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album", "story.md"), []byte("# Reise\n\nBlogtext"), 0o600); err != nil {
		t.Fatal(err)
	}

	lib, err := New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	if _, err := lib.List(context.Background(), ListOptions{Path: "album"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetMediaTags("album/photo.jpg", []string{" Reise ", "", "Reise", "Sommer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.SetFolderTags("album", []string{"Reise"}); err != nil {
		t.Fatal(err)
	}
	post, err := lib.blogFromPath("album/story.md")
	if err != nil {
		t.Fatal(err)
	}
	post.Tags = []string{"Reise"}
	lib.saveBlog(post)
	if _, err := lib.SaveTag(context.Background(), "Leer", "#cc3300"); err != nil {
		t.Fatal(err)
	}

	tags, err := lib.ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("tags = %#v", tags)
	}
	if tags[0].Name != "leer" || tags[0].Color != "#cc3300" || tags[0].Count != 0 {
		t.Fatalf("first tag = %#v", tags[0])
	}
	if tags[1].Name != "reise" || tags[1].Color != "#176b87" || tags[1].Count != 3 {
		t.Fatalf("second tag = %#v", tags[1])
	}
	if tags[2].Name != "sommer" || tags[2].Count != 1 {
		t.Fatalf("third tag = %#v", tags[2])
	}

	renamed, err := lib.RenameTag(context.Background(), "Reise", "Ausflug", "#2f855a")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "ausflug" || renamed.Color != "#2f855a" || renamed.Count != 3 {
		t.Fatalf("renamed tag = %#v", renamed)
	}
	media, err := lib.Media("album/photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Tags) != 2 || media.Tags[0] != "ausflug" || media.Tags[1] != "sommer" {
		t.Fatalf("media tags = %#v", media.Tags)
	}
	blog, err := lib.blogFromPath("album/story.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(blog.Tags) != 1 || blog.Tags[0] != "ausflug" {
		t.Fatalf("blog tags = %#v", blog.Tags)
	}
	results, err := lib.List(context.Background(), ListOptions{Query: "tag:ausflug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Media) != 1 || len(results.Folders) != 1 || len(results.Blogs) != 1 {
		t.Fatalf("renamed tag results = media %#v folders %#v blogs %#v", results.Media, results.Folders, results.Blogs)
	}

	deleted, err := lib.DeleteTag(context.Background(), "Ausflug")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Name != "ausflug" || deleted.Count != 3 {
		t.Fatalf("deleted tag = %#v", deleted)
	}
	media, err = lib.Media("album/photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Tags) != 1 || media.Tags[0] != "sommer" {
		t.Fatalf("media tags after delete = %#v", media.Tags)
	}
	blog, err = lib.blogFromPath("album/story.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(blog.Tags) != 0 {
		t.Fatalf("blog tags after delete = %#v", blog.Tags)
	}
	results, err = lib.List(context.Background(), ListOptions{Query: "tag:ausflug"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Media) != 0 || len(results.Folders) != 0 || len(results.Blogs) != 0 {
		t.Fatalf("deleted tag results = media %#v folders %#v blogs %#v", results.Media, results.Folders, results.Blogs)
	}
}
