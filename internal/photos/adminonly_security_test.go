package photos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAdminOnlyMarkerSymlinksFailClosed(t *testing.T) {
	for _, target := range []string{"missing", AdminOnlyMarkerName, "."} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			album := filepath.Join(root, "album")
			if err := os.MkdirAll(filepath.Join(album, "child"), 0o750); err != nil {
				t.Fatal(err)
			}
			l := newTestLibrary(t, root)
			defer l.Close()
			if err := os.Symlink(target, filepath.Join(album, AdminOnlyMarkerName)); err != nil {
				t.Fatal(err)
			}
			if private, err := l.FolderAdminOnly("album/child"); err != nil || !private {
				t.Errorf("folder privacy = %v, %v", private, err)
			}
			if private, err := l.MediaAdminOnly("album/child/photo.jpg"); err != nil || !private {
				t.Errorf("media privacy = %v, %v", private, err)
			}
			if private, err := l.MediaAdminOnlyBatch([]string{"album/child/a.jpg", "album/child/b.jpg"}); err != nil || !private["album/child/a.jpg"] || !private["album/child/b.jpg"] {
				t.Errorf("batch privacy = %v, %v", private, err)
			}
			if private, err := newFaceDirectoryVisibility(root).check("album/child"); err != nil || !private {
				t.Errorf("face privacy = %v, %v", private, err)
			}
		})
	}
}

func TestAdminOnlyMarkerReadFailurePreservesFaceData(t *testing.T) {
	l := faceLibrary(t, "album/a.jpg")
	finishFace(t, l, 0)
	album := filepath.Join(l.Root(), "album")
	if err := os.Chmod(album, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(album, 0o750)
	if _, err := os.Lstat(filepath.Join(album, AdminOnlyMarkerName)); !errors.Is(err, os.ErrPermission) {
		t.Skip("filesystem does not enforce directory permission checks")
	}
	if !adminOnlyMarkerExists(album) || !l.directoryAdminOnly("album") {
		t.Error("boolean visibility checks did not fail closed")
	}
	if _, err := l.MediaAdminOnly("album/a.jpg"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("media permission error = %v", err)
	}
	if _, err := l.MediaAdminOnlyBatch([]string{"album/a.jpg"}); !errors.Is(err, os.ErrPermission) {
		t.Errorf("batch permission error = %v", err)
	}
	if err := l.refreshAdminOnlyIndexFlags(context.Background()); !errors.Is(err, os.ErrPermission) {
		t.Errorf("startup visibility error = %v", err)
	}
	var count int
	if err := l.index.db.QueryRow(`SELECT count(*) FROM photo_faces`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("permission failure destroyed face data: %d, %v", count, err)
	}
}

func TestAdminOnlyRefreshRejectsUnresolvableDirectoriesWithoutPublishingThem(t *testing.T) {
	l := adminRefreshFixture(t)
	defer l.Close()
	album := filepath.Join(l.Root(), "album")
	if err := os.WriteFile(filepath.Join(album, AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.refreshAdminOnlyIndexFlags(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(album, filepath.Join(t.TempDir(), "offline")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), album); err != nil {
		t.Fatal(err)
	}
	if err := l.refreshAdminOnlyIndexFlags(context.Background()); err == nil {
		t.Error("visibility refresh accepted a symlink instead of a directory")
	}
	for _, table := range []string{"media_index", "blog_index", "folder_index"} {
		var public int
		if err := l.index.db.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE (path='album' OR path LIKE 'album/%') AND admin_only=0`).Scan(&public); err != nil {
			t.Fatal(err)
		}
		if public != 0 {
			t.Errorf("%s: published %d private entries", table, public)
		}
	}
}
