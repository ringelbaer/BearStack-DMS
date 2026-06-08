package photopath

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanRejectsTraversal(t *testing.T) {
	if got, err := Clean("family/2024"); err != nil || got != "family/2024" {
		t.Fatalf("Clean() = %q, %v; want family/2024, nil", got, err)
	}
	if _, err := Clean("../secrets"); !errors.Is(err, ErrEscapesRoot) {
		t.Fatalf("Clean traversal error = %v, want ErrEscapesRoot", err)
	}
	if _, err := Clean("/etc/passwd"); !errors.Is(err, ErrEscapesRoot) {
		t.Fatalf("Clean absolute error = %v, want ErrEscapesRoot", err)
	}
}

func TestResolveRejectsSymlinkComponents(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "album", "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Resolve(root, "album/link/photo.jpg"); !errors.Is(err, ErrEscapesRoot) {
		t.Fatalf("Resolve symlink error = %v, want ErrEscapesRoot", err)
	}
}
