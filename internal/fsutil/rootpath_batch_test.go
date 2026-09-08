package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkRootPathTraversal(b *testing.B) {
	root := b.TempDir()
	paths := make([]string, 500)
	for i := range paths {
		paths[i] = fmt.Sprintf("archive/photos/year/album-%03d", i)
		if err := os.MkdirAll(filepath.Join(root, paths[i]), 0700); err != nil {
			b.Fatal(err)
		}
	}
	escape := errors.New("escape")
	for _, mode := range []string{"independent", "batch"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				resolve := func(rel string) error {
					_, _, err := ResolveWithinRoot(root, rel, true, escape)
					return err
				}
				if mode == "batch" {
					batch := NewRootPathBatch(root)
					resolve = func(rel string) error {
						_, _, err := batch.Resolve(rel, true, escape)
						return err
					}
				}
				for _, rel := range paths {
					if err := resolve(rel); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func TestRootPathBatchSharesAncestorsAndPreservesResolution(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"parent/a", "parent/b"} {
		if err := os.MkdirAll(filepath.Join(root, rel), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	batch := NewRootPathBatch(root)
	calls := map[string]int{}
	batch.lstat = func(path string) (os.FileInfo, error) {
		calls[path]++
		return os.Lstat(path)
	}
	escape := errors.New("escape")
	for _, rel := range []string{"parent/a", "parent/b", "parent/a", "missing/a", "missing/b", "link/a", "link/b", "file/child", "../escape", "", `parent\b`, "/absolute"} {
		wantClean, wantAbs, wantErr := ResolveWithinRoot(root, rel, true, escape)
		clean, abs, err := batch.Resolve(rel, true, escape)
		if clean != wantClean || abs != wantAbs || errorText(err) != errorText(wantErr) {
			t.Errorf("%q: (%q,%q,%v), want (%q,%q,%v)", rel, clean, abs, err, wantClean, wantAbs, wantErr)
		}
	}
	for path, count := range calls {
		if count != 1 {
			t.Errorf("%s checked %d times", path, count)
		}
	}
	if calls[filepath.Join(root, "parent")] != 1 || calls[filepath.Join(root, "missing/a")] != 0 {
		t.Fatalf("ancestor/missing checks: %v", calls)
	}
	// A new traversal and ordinary file access must observe replacements.
	if err := os.RemoveAll(filepath.Join(root, "parent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "parent")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewRootPathBatch(root).Resolve("parent/a", true, escape); !errors.Is(err, escape) {
		t.Fatalf("fresh traversal accepted symlink: %v", err)
	}
	if _, _, err := ResolveWithinRoot(root, "parent/b", true, escape); !errors.Is(err, escape) {
		t.Fatalf("normal access reused traversal cache: %v", err)
	}
}

func TestRootPathBatchPreservesFilesystemErrors(t *testing.T) {
	batch := NewRootPathBatch(t.TempDir())
	calls := 0
	batch.lstat = func(path string) (os.FileInfo, error) {
		calls++
		return nil, &os.PathError{Op: "lstat", Path: path, Err: os.ErrPermission}
	}
	for _, rel := range []string{"parent/a", "parent/b"} {
		if _, _, err := batch.Resolve(rel, true, errors.New("escape")); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("permission error lost: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("failed ancestor checked %d times", calls)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
