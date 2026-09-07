package photos

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func setBlogTagsForTest(t *testing.T, lib *Library, path string, tags []string) {
	t.Helper()
	ctx := context.Background()
	tx, err := lib.index.beginTagWrite(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE blog_index SET tags = ? WHERE path = ?`, tagsJSONString(tags), path); err != nil {
		t.Fatal(err)
	}
	if err := syncTagIndexTx(ctx, tx, "blog_tag_index", "blog_path", path, tags); err != nil {
		t.Fatal(err)
	}
	if err := refreshBlogSearchTx(ctx, tx, path); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func photoTagFixture(t *testing.T) *Library {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0750); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "album/photo.jpg"), color.RGBA{R: 80, A: 255})
	writeJPEG(t, filepath.Join(root, "album/second.jpg"), color.RGBA{G: 80, A: 255})
	if err := os.WriteFile(filepath.Join(root, "album/story.md"), []byte("# Journal\nText"), 0600); err != nil {
		t.Fatal(err)
	}
	l := newTestLibrary(t, root)
	t.Cleanup(func() { _ = l.Close() })
	ctx := context.Background()
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"album/photo.jpg", "album/second.jpg"} {
		if _, err := l.SetMediaTagsContext(ctx, path, []string{"original"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.SetFolderTagsContext(ctx, "album", []string{"original"}); err != nil {
		t.Fatal(err)
	}
	setBlogTagsForTest(t, l, "album/story.md", []string{"original"})
	return l
}

// Include the FTS row contents and catalog, not only the public tag response.
func photoTagState(t *testing.T, l *Library) []string {
	t.Helper()
	var result []string
	for _, query := range []string{
		`SELECT name || ':' || color FROM photo_tags ORDER BY name`,
		`SELECT path || ':' || tags FROM media_index ORDER BY path`,
		`SELECT path || ':' || tags FROM folder_index ORDER BY path`,
		`SELECT path || ':' || tags FROM blog_index ORDER BY path`,
		`SELECT media_path || ':' || tag FROM media_tag_index ORDER BY media_path, tag`,
		`SELECT folder_path || ':' || tag FROM folder_tag_index ORDER BY folder_path, tag`,
		`SELECT blog_path || ':' || tag FROM blog_tag_index ORDER BY blog_path, tag`,
		`SELECT path || ':' || search_text FROM media_search ORDER BY path`,
		`SELECT path || ':' || search_text FROM folder_search ORDER BY path`,
		`SELECT path || ':' || search_text FROM blog_search ORDER BY path`,
	} {
		rows, err := l.index.db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			result = append(result, query+":"+value)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func TestPhotoTagMutationsRollbackOnIndexFailure(t *testing.T) {
	ctx := context.Background()
	for _, operation := range []string{"media", "folder", "rename", "delete", "bulk"} {
		t.Run(operation, func(t *testing.T) {
			l := photoTagFixture(t)
			before := photoTagState(t, l)
			// FTS writes its content into the backing table. Rejecting INSERT
			// exercises rollback after its previous row has already been deleted.
			search := "media_search_content"
			if operation == "folder" || operation == "rename" {
				search = "folder_search_content"
			}
			if operation == "delete" {
				search = "blog_search_content"
			}
			if _, err := l.index.db.Exec(`CREATE TRIGGER fail_search BEFORE INSERT ON ` + search + ` BEGIN SELECT RAISE(ABORT, 'forced search failure'); END`); err != nil {
				t.Fatal(err)
			}
			var err error
			switch operation {
			case "media":
				_, err = l.SetMediaTagsContext(ctx, "album/photo.jpg", []string{"changed"})
			case "folder":
				_, err = l.SetFolderTagsContext(ctx, "album", []string{"changed"})
			case "rename":
				_, err = l.RenameTag(ctx, "original", "changed")
			case "delete":
				_, err = l.DeleteTag(ctx, "original")
			case "bulk":
				_, err = l.UpdateMediaTagsContext(ctx, []string{"album/second.jpg", "album/photo.jpg"}, []string{"changed"}, true)
			}
			if err == nil {
				t.Fatal("expected search failure")
			}
			if after := photoTagState(t, l); !reflect.DeepEqual(before, after) {
				t.Fatalf("partial mutation after failure\nbefore: %v\nafter: %v", before, after)
			}
		})
	}
	t.Run("tag association", func(t *testing.T) {
		l := photoTagFixture(t)
		before := photoTagState(t, l)
		if _, err := l.index.db.Exec(`CREATE TRIGGER fail_tag BEFORE INSERT ON media_tag_index BEGIN SELECT RAISE(ABORT, 'forced tag failure'); END`); err != nil {
			t.Fatal(err)
		}
		if _, err := l.SetMediaTagsContext(ctx, "album/photo.jpg", []string{"changed"}); err == nil {
			t.Fatal("expected tag failure")
		}
		if !reflect.DeepEqual(before, photoTagState(t, l)) {
			t.Fatal("tag association failure committed partial state")
		}
	})
}

func TestPhotoTagBulkConcurrentChanges(t *testing.T) {
	l := photoTagFixture(t)
	// A separate pool proves that correctness comes from SQLite transactions,
	// rather than a mutex that only covers a single Library instance.
	other, _, err := openPhotoIndexStore(l.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer other.close()
	ctx := context.Background()
	paths := []string{"album/photo.jpg", "album/second.jpg"}
	for _, add := range []bool{true, false} {
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				store := l.index
				if i%2 == 0 {
					store = other
				}
				if n, err := store.updateMediaTags(ctx, paths, []string{fmt.Sprintf("tag%02d", i)}, add); err != nil || n != len(paths) {
					t.Errorf("add=%t count=%d error=%v", add, n, err)
				}
			}(i)
		}
		close(start)
		wg.Wait()
		for _, path := range paths {
			m, err := l.MediaContext(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if add {
				want = 13
			}
			if len(m.Tags) != want {
				t.Fatalf("add=%t tags=%v", add, m.Tags)
			}
		}
	}
}

func TestPhotoTagBulkRollbackAndNoop(t *testing.T) {
	l := photoTagFixture(t)
	ctx := context.Background()
	before := photoTagState(t, l)
	if n, err := l.index.updateMediaTags(ctx, []string{"album/photo.jpg", "missing.jpg"}, []string{"changed"}, true); err == nil || n != 0 {
		t.Fatalf("count=%d error=%v", n, err)
	}
	if !reflect.DeepEqual(before, photoTagState(t, l)) {
		t.Fatal("bulk failure changed preceding photo")
	}
	if n, err := l.UpdateMediaTagsContext(ctx, []string{"album/photo.jpg", "album/photo.jpg"}, []string{" Original "}, true); err != nil || n != 0 {
		t.Fatalf("noop count=%d error=%v", n, err)
	}
	if n, err := l.UpdateMediaTagsContext(ctx, []string{"album/photo.jpg", "album/photo.jpg"}, []string{"new"}, true); err != nil || n != 1 {
		t.Fatalf("deduplicated count=%d error=%v", n, err)
	}
	before = photoTagState(t, l)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.index.updateMediaTags(canceled, []string{"album/photo.jpg"}, []string{"canceled"}, true); err == nil {
		t.Fatal("expected cancellation")
	}
	if !reflect.DeepEqual(before, photoTagState(t, l)) {
		t.Fatal("cancellation changed tags")
	}
}

func TestPhotoTagScansPreserveCurrentTags(t *testing.T) {
	l := photoTagFixture(t)
	ctx := context.Background()
	media, err := l.MediaContext(ctx, "album/photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	post, err := l.blogFromPath("album/story.md")
	if err != nil {
		t.Fatal(err)
	}
	staleFolder := Folder{Path: "album", Name: "album", Tags: []string{"original"}, ModTime: time.Now()}
	if _, err := l.DeleteTag(ctx, "original"); err != nil {
		t.Fatal(err)
	}
	if err := l.saveMediaContext(ctx, media); err != nil {
		t.Fatal(err)
	}
	if err := l.saveBlogBatch(ctx, []BlogPost{post}); err != nil {
		t.Fatal(err)
	}
	if err := l.saveFolder(staleFolder); err != nil {
		t.Fatal(err)
	}
	if err := l.saveScannedFolder(ctx, "album", time.Now(), 2, 1, 0, false, ""); err != nil {
		t.Fatal(err)
	}
	for _, state := range photoTagState(t, l) {
		if strings.Contains(state, "original") {
			t.Fatalf("stale scan restored removed tag: %s", state)
		}
	}
}

func TestPhotoTagChangesUseTagIndex(t *testing.T) {
	l := photoTagFixture(t)
	for _, kind := range []string{"media", "folder", "blog"} {
		query := photoTagValuesQuery(kind+"_index", kind+"_tag_index", kind+"_path")
		rows, err := l.index.db.Query("EXPLAIN QUERY PLAN "+query, "original")
		if err != nil {
			t.Fatal(err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		joined := strings.Join(plan, "\n")
		if !strings.Contains(joined, "idx_"+kind+"_tag_index_tag") || strings.Contains(joined, "SCAN "+kind+"_index") {
			t.Fatalf("unbounded %s lookup: %s", kind, joined)
		}
	}
}
