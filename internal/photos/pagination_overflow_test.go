package photos

import (
	"context"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestGalleryExtremePageNumbers(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0700); err != nil {
		t.Fatal(err)
	}
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{R: 255, A: 255})
	writeJPEG(t, filepath.Join(root, "album", "other.jpg"), color.RGBA{G: 255, A: 255})
	if err := os.WriteFile(filepath.Join(root, "story.md"), []byte("# Story"), 0600); err != nil {
		t.Fatal(err)
	}
	lib := newTestLibrary(t, root)
	defer lib.Close()
	if _, err := lib.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, filesystem := range []bool{true, false} {
		for _, query := range []string{"", "type:image", "-tag:missing", "file_name:photo or file_name:other"} {
			for _, page := range []int{math.MaxInt, math.MaxInt/120 + 2} {
				t.Run(fmt.Sprintf("filesystem=%v/query=%s/page=%d", filesystem, query, page), func(t *testing.T) {
					got, err := lib.List(context.Background(), ListOptions{FullFilesystem: filesystem, Query: query, Page: page, PageSize: 120, FolderPageSize: 24, BlogPageSize: 20, Sort: "ascending_name"})
					if err != nil {
						t.Fatal(err)
					}
					if len(got.Media) != 0 || len(got.Folders) != 0 || len(got.Blogs) != 0 || got.HasNext || got.FolderHasNext || got.BlogHasNext {
						t.Fatalf("out-of-range listing: %+v", got)
					}
					if !got.HasPrev {
						t.Fatal("previous page should remain available")
					}
				})
			}
		}
	}
}

func TestPaginationLastPageAndMaximumSize(t *testing.T) {
	items := []Media{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	for _, tc := range []struct {
		page, size int
		count      int
		next       bool
	}{
		{1, 2, 2, true}, {2, 2, 1, false}, {3, 2, 0, false}, {math.MaxInt, 2, 0, false}, {1, math.MaxInt, 3, false}, {2, math.MaxInt, 0, false},
	} {
		listing := Listing{Media: items}
		paginateListingMedia(&listing, ListOptions{Page: tc.page, PageSize: tc.size})
		if len(listing.Media) != tc.count || listing.HasNext != tc.next || pageHasNext(tc.page, tc.size, len(items)) != tc.next {
			t.Fatalf("%+v: %+v", tc, listing)
		}
	}
	if got := postFilterCandidateLimit(indexMediaOptions{Offset: math.MaxInt, Limit: 120}); got != indexPostFilterCandidateMax {
		t.Fatalf("wrapped candidate limit %d", got)
	}
}

func TestUnindexedFastFallbackExtremePage(t *testing.T) {
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "photo.jpg"), color.RGBA{R: 255, A: 255})
	lib := newTestLibrary(t, root)
	defer lib.Close()
	got, err := lib.List(context.Background(), ListOptions{Page: math.MaxInt, PageSize: 120})
	if err != nil || len(got.Media) != 0 || got.HasNext {
		t.Fatalf("fast fallback: %+v %v", got, err)
	}
}
