package server

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"bearstack/internal/photos"
	"bearstack/internal/transfers"
)

func transferPhotoLibrary(t *testing.T) *photos.Library {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"2026/20260613-Geburtstag_Thomas/a.jpg", "2026/20260613-Geburtstag_Thomas/Unter_Ordner/b.mp4", "root.jpg", "private/hidden.jpg"} {
		full := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(full), 0750)
		if err := os.WriteFile(full, []byte("original"), 0444); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "private/.adminonly"), nil, 0444)
	l, err := photos.New(root, t.TempDir(), filepath.Join(t.TempDir(), "photos.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	if _, err = l.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	return l
}
func TestTransferSourcePathsNamesSelectionAndChanges(t *testing.T) {
	l := transferPhotoLibrary(t)
	s := photoTransferSource{l}
	ctx := context.Background()
	folder := "2026/20260613-Geburtstag_Thomas"
	selection := transfers.Selection{Path: folder}
	info, err := s.Describe(ctx, selection)
	if err != nil || info.Target != "20260613-Geburtstag_Thomas" || info.Name != photos.MediaFolderName(folder+"/a.jpg") {
		t.Fatalf("description %+v %v", info, err)
	}
	var items []transfers.SourceItem
	if err = s.Walk(ctx, selection, func(i transfers.SourceItem) error { items = append(items, i); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("recursive selection %v", items)
	}
	for _, item := range items {
		if item.DisplayPath != photos.MediaDisplayPath(item.Path) || strings.HasPrefix(strings.Join(item.Relative, "/"), "2026/") {
			t.Fatalf("path formatting %+v", item)
		}
		file, err := s.Open(ctx, selection, item)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(file)
		file.Close()
		if err != nil || string(data) != "original" {
			t.Fatalf("read %q %v", data, err)
		}
	}
	selected := transfers.Selection{Paths: []string{"root.jpg", folder + "/a.jpg", "root.jpg", "private/hidden.jpg"}}
	items = nil
	if err = s.Walk(ctx, selected, func(i transfers.SourceItem) error { items = append(items, i); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || strings.Join(items[1].Relative, "/") != folder+"/a.jpg" {
		t.Fatalf("selected paths %+v", items)
	}
	if info, err = s.Describe(ctx, selected); err != nil || !info.Virtual || info.Target != "Auswahl" {
		t.Fatalf("selection description %+v %v", info, err)
	}
	for _, sel := range []transfers.Selection{{Path: ""}, {Path: "2026"}, {Path: ".people"}, {Paths: []string{}}, {Paths: []string{"../outside.jpg"}}} {
		if _, err = s.Describe(ctx, sel); err == nil {
			t.Fatalf("accepted invalid selection %+v", sel)
		}
	}
	if err = os.Chmod(filepath.Join(l.Root(), "root.jpg"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(l.Root(), "root.jpg"), []byte("changed original"), 0444); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Open(ctx, selected, items[0]); transfers.Kind(err) != transfers.Conflict {
		t.Fatalf("changed source accepted %v", err)
	}
	os.Remove(filepath.Join(l.Root(), "root.jpg"))
	os.Symlink(filepath.Join(l.Root(), folder, "a.jpg"), filepath.Join(l.Root(), "root.jpg"))
	if _, err = s.Open(ctx, selected, items[0]); err == nil {
		t.Fatal("symlink accepted")
	}
}
func TestTransferSourceOnReadOnlyMount(t *testing.T) {
	root := os.Getenv("BEARSTACK_READONLY_TEST_ROOT")
	if root == "" {
		t.Skip("requires disposable read-only mount")
	}
	probe, err := os.OpenFile(filepath.Join(root, "transfer-write-probe"), os.O_CREATE|os.O_WRONLY, 0600)
	if err == nil {
		probe.Close()
		os.Remove(probe.Name())
		t.Fatal("fixture is writable")
	}
	if !errors.Is(err, syscall.EROFS) {
		t.Fatalf("not a read-only mount: %v", err)
	}
	l, err := photos.New(root, t.TempDir(), filepath.Join(t.TempDir(), "photos.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err = l.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	listing, err := l.List(context.Background(), photos.ListOptions{Recursive: true, PageSize: 10})
	if err != nil || len(listing.Media) == 0 {
		t.Fatalf("no media %+v %v", listing, err)
	}
	sel := transfers.Selection{Paths: []string{listing.Media[0].Path}}
	source := photoTransferSource{l}
	count := 0
	err = source.Walk(context.Background(), sel, func(item transfers.SourceItem) error {
		file, err := source.Open(context.Background(), sel, item)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(io.Discard, file)
		count++
		return err
	})
	if err != nil || count != 1 {
		t.Fatalf("readonly export: %d %v", count, err)
	}
}
