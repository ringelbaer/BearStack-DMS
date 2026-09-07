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

func TestRootOperationsRejectTraversalAndSymlinksWithoutCreatingFiles(t *testing.T) {
	for _, target := range []string{"outside", "inside", "dangling"} {
		t.Run(target, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			destination := outside
			if target == "inside" {
				destination = filepath.Join(root, "real")
				if err := os.Mkdir(destination, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if target == "dangling" {
				destination = filepath.Join(outside, "missing")
			}
			if err := os.Symlink(destination, filepath.Join(root, "link")); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			escapeErr := errors.New("escape")
			for _, rel := range []string{"../child", `..\child`, "/absolute/child", "link", "link/child"} {
				if _, _, err := ResolveWithinRoot(root, rel, false, escapeErr); !errors.Is(err, escapeErr) {
					t.Errorf("ResolveWithinRoot(%q) = %v, want escape error", rel, err)
				}
				if _, err := EnsureDirWithinRoot(root, rel, 0o700, escapeErr); !errors.Is(err, escapeErr) {
					t.Errorf("EnsureDirWithinRoot(%q) = %v, want escape error", rel, err)
				}
			}
			if _, err := os.Stat(filepath.Join(destination, "child")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("child created through symlink: %v", err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("outside directory modified: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestRootOperationsAllowRootAndExistingDirectories(t *testing.T) {
	root := t.TempDir()
	escapeErr := errors.New("escape")
	for _, rel := range []string{"", ".", "/", "a/.."} {
		clean, abs, err := ResolveWithinRoot(root, rel, true, escapeErr)
		if err != nil || clean != "" || abs != root {
			t.Errorf("ResolveWithinRoot(%q) = %q, %q, %v", rel, clean, abs, err)
		}
		if _, _, err := ResolveWithinRoot(root, rel, false, escapeErr); err == nil {
			t.Errorf("empty file path %q accepted", rel)
		}
		if dir, err := EnsureDirWithinRoot(root, rel, 0o700, escapeErr); err != nil || dir != root {
			t.Errorf("EnsureDirWithinRoot(%q) = %q, %v", rel, dir, err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "existing"), 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		dir, err := EnsureDirWithinRoot(root, "existing/child", 0o700, escapeErr)
		if err != nil || dir != filepath.Join(root, "existing", "child") {
			t.Fatalf("repeated creation = %q, %v", dir, err)
		}
	}
}
