package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanRelativePathNormalizesAndRejectsEscapes(t *testing.T) {
	escapeErr := errors.New("escape")
	got, err := CleanRelativePath(`  a\b/../c  `, false, escapeErr)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a/c" {
		t.Fatalf("clean path = %q", got)
	}
	if _, err := CleanRelativePath("../secret", false, escapeErr); !errors.Is(err, escapeErr) {
		t.Fatalf("escape err = %v", err)
	}
	if got, err := CleanRelativePath(".", true, escapeErr); err != nil || got != "" {
		t.Fatalf("empty path = %q err=%v", got, err)
	}
}

func TestResolveWithinRootRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	escapeErr := errors.New("escape")
	if _, _, err := ResolveWithinRoot(root, "link/file.txt", false, escapeErr); !errors.Is(err, escapeErr) {
		t.Fatalf("symlink escape err = %v", err)
	}
}

func TestEnsureDirWithinRootCreatesNestedDirsAndRejectsFiles(t *testing.T) {
	root := t.TempDir()
	escapeErr := errors.New("escape")
	dir, err := EnsureDirWithinRoot(root, "a/b", 0o750, escapeErr)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(root, "a", "b") {
		t.Fatalf("dir = %q", dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("created dir info = %#v err=%v", info, err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureDirWithinRoot(root, "file/child", 0o750, escapeErr); err == nil {
		t.Fatal("expected file path to be rejected")
	}
}
