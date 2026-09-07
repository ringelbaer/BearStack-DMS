package photos

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type failingInfoEntry struct {
	os.DirEntry
	err error
}

func (e failingInfoEntry) Info() (os.FileInfo, error) { return nil, e.err }

func TestIndexScanPreservesEntriesOnStatFailure(t *testing.T) {
	for _, name := range []string{"photo.jpg", "story.md"} {
		for _, failure := range []error{os.ErrPermission, os.ErrNotExist} {
			t.Run(name+"/"+failure.Error(), func(t *testing.T) {
				l := photoTagFixture(t)
				ctx := context.Background()
				before := photoPruneState(t, l)
				cache, err := l.loadIndexRunCache(ctx)
				if err != nil {
					t.Fatal(err)
				}
				abs := filepath.Join(l.Root(), "album")
				info, err := os.Stat(abs)
				if err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(abs)
				if err != nil {
					t.Fatal(err)
				}
				for i, entry := range entries {
					if entry.Name() == name {
						entries[i] = failingInfoEntry{entry, failure}
					}
				}
				_, err = l.indexDirectoryEntries(ctx, indexQueueItem{Path: "album"}, cache, abs, info, entries)
				if !errors.Is(err, failure) {
					t.Fatalf("scan error = %v; want %v", err, failure)
				}
				if after := photoPruneState(t, l); !reflect.DeepEqual(before, after) {
					t.Fatalf("failed scan changed index:\nbefore %v\nafter %v", before, after)
				}
				// A later complete scan must still detect real deletions.
				if err := os.Remove(filepath.Join(abs, name)); err != nil {
					t.Fatal(err)
				}
				if _, err := l.RebuildIndex(ctx); err != nil {
					t.Fatal(err)
				}
				table := "media_index"
				if name == "story.md" {
					table = "blog_index"
				}
				var count int
				if err := l.index.db.QueryRow(`SELECT count(*) FROM `+table+` WHERE path=?`, "album/"+name).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("complete scan did not prune deleted file")
				}
			})
		}
	}
}

func TestIndexScanPreservesEntriesOnBlogReadFailure(t *testing.T) {
	l := photoTagFixture(t)
	ctx := context.Background()
	before := photoPruneState(t, l)
	path := filepath.Join(l.Root(), "album/story.md")
	// Force a cache miss, then remove the file after directory metadata was read.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range entries {
		fileInfo, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		entries[i] = fs.FileInfoToDirEntry(fileInfo)
	}
	cache, err := l.loadIndexRunCache(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, err = l.indexDirectoryEntries(ctx, indexQueueItem{Path: "album"}, cache, filepath.Dir(path), info, entries)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read error = %v", err)
	}
	if after := photoPruneState(t, l); !reflect.DeepEqual(before, after) {
		t.Fatal("failed blog read changed index")
	}
	if err := os.WriteFile(path, []byte("# Recovered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	var tags, text string
	if err := l.index.db.QueryRow(`SELECT tags, text FROM blog_index WHERE path='album/story.md'`).Scan(&tags, &text); err != nil {
		t.Fatal(err)
	}
	if tags != `["original"]` || text != "Recovered" {
		t.Fatalf("recovered blog = %s %q", tags, text)
	}
}

func photoPruneState(t *testing.T, l *Library) []string {
	t.Helper()
	state := photoTagState(t, l)
	for _, table := range []string{"folder_preview_index", "photo_thumbnail_index", "photo_folder_scan", "photo_faces", "photo_face_jobs"} {
		var count int
		if err := l.index.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		state = append(state, fmt.Sprintf("%s:%d", table, count))
	}
	rows, err := l.index.db.Query(`SELECT path, mod_time_unix_nano, quick_signature_unix_nano FROM photo_folder_scan ORDER BY path`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		var full, quick int64
		if err := rows.Scan(&path, &full, &quick); err != nil {
			t.Fatal(err)
		}
		state = append(state, fmt.Sprintf("%s:%d:%d", path, full, quick))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestIndexPruningRollsBackRelatedTables(t *testing.T) {
	for _, operation := range []struct {
		name   string
		table  string
		remove func(context.Context, *photoIndexStore) error
	}{
		{"media", "media_index", func(ctx context.Context, s *photoIndexStore) error {
			return s.deleteMediaIndexPaths(ctx, []string{"album/photo.jpg", "album/second.jpg"})
		}},
		{"blog", "blog_index", func(ctx context.Context, s *photoIndexStore) error {
			return s.deleteBlogIndexPaths(ctx, []string{"album/story.md"})
		}},
		{"subtree", "photo_folder_scan", func(ctx context.Context, s *photoIndexStore) error { return s.deleteFolderIndexSubtree(ctx, "album") }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			l := photoTagFixture(t)
			finishFace(t, l, 0)
			before := photoPruneState(t, l)
			// Fail after related rows (including cascading face deletions) were removed.
			if _, err := l.index.db.Exec(`CREATE TRIGGER fail_prune BEFORE DELETE ON ` + operation.table + ` BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := operation.remove(ctx, l.index); err == nil {
				t.Fatal("expected injected failure")
			}
			if after := photoPruneState(t, l); !reflect.DeepEqual(before, after) {
				t.Fatalf("partial pruning:\nbefore %v\nafter %v", before, after)
			}
			if _, err := l.index.db.Exec(`DROP TRIGGER fail_prune`); err != nil {
				t.Fatal(err)
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if err := operation.remove(cancelled, l.index); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled prune = %v", err)
			}
			if after := photoPruneState(t, l); !reflect.DeepEqual(before, after) {
				t.Fatal("cancelled pruning changed index")
			}
			if err := operation.remove(ctx, l.index); err != nil {
				t.Fatal(err)
			}
			if after := photoPruneState(t, l); reflect.DeepEqual(before, after) {
				t.Fatal("retry did not remove entries")
			}
		})
	}
}
