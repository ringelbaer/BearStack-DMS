package photos

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestOpenOriginalRejectsLinksAndPinsLibraryRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "photos")
	if err := os.MkdirAll(filepath.Join(root, "album"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "album", "a.jpg"), []byte("original"), 0400); err != nil {
		t.Fatal(err)
	}
	l := newTestLibrary(t, root)
	defer l.Close()
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "a.jpg"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{out, "outside"}, {"album", "inside"}, {"album/a.jpg", "link.jpg"}} {
		if err := os.Symlink(pair[0], filepath.Join(root, pair[1])); err != nil {
			t.Fatal(err)
		}
		name := pair[1]
		if filepath.Ext(name) != ".jpg" {
			name += "/a.jpg"
		}
		if f, err := l.OpenOriginal(name, true); err == nil {
			f.Close()
			t.Fatalf("accepted symlink %s", name)
		}
	}
	for _, name := range []string{"../a.jpg", "/etc/passwd", "album"} {
		if f, err := l.OpenOriginal(name, true); err == nil {
			f.Close()
			t.Fatalf("accepted %s", name)
		}
	}
	if err := os.Rename(root, root+"-old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, root); err != nil {
		t.Fatal(err)
	}
	f, err := l.OpenOriginal("album/a.jpg", true)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || string(data) != "original" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if _, err := f.Write([]byte("changed")); err == nil {
		t.Fatal("original descriptor is writable")
	}
}

func TestOpenOriginalRechecksPrivacyAndRejectsConcurrentSymlinkSwap(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.Mkdir(album, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(album, "a.jpg"), []byte("original"), 0400); err != nil {
		t.Fatal(err)
	}
	l := newTestLibrary(t, root)
	defer l.Close()
	marker := filepath.Join(album, AdminOnlyMarkerName)
	if err := os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if f, err := l.OpenOriginal("album/a.jpg", false); err != ErrAdminOnly() {
		if f != nil {
			f.Close()
		}
		t.Fatalf("private open: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "a.jpg"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if os.Rename(album, album+"-held") != nil {
				continue
			}
			_ = os.Symlink(out, album)
			_ = os.Remove(album)
			_ = os.Rename(album+"-held", album)
		}
	})
	defer func() { close(stop); wg.Wait() }()
	for range 500 {
		f, err := l.OpenOriginal("album/a.jpg", false)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil || string(data) != "original" {
			t.Fatalf("escaped during swap: %q, %v", data, err)
		}
	}
}
