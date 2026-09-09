package photos

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// Broad substring search must retain the complete folder result and page its
// previews even at deep offsets. Construction is outside the measurement.
func BenchmarkNativeFolderSearch(b *testing.B) {
	l := routeTestLibrary(b)
	if _, err := l.index.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM seq WHERE n<10000)
	INSERT INTO folder_index(path,parent,name,media_count,public_media_count,recursive_media_count,public_recursive_media_count,public_recursive_blog_count,indexed_at)
	SELECT printf('album-%05d',n),'',printf('album-%05d',n),1,1,1,1,0,'2026-09-09' FROM seq`); err != nil {
		b.Fatal(err)
	}
	for _, page := range []int{1, 417} {
		b.Run(fmt.Sprintf("page-%d", page), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				listing, err := l.List(context.Background(), ListOptions{Query: "al", SkipMedia: true, SkipBlogs: true, FolderPageSize: 24, FolderPreviewSize: 2, Page: page})
				want := 24
				if page == 417 {
					want = 16
				}
				if err != nil || listing.FolderTotal != 10000 || len(listing.Folders) != want {
					b.Fatalf("search: %d/%d %v", len(listing.Folders), listing.FolderTotal, err)
				}
			}
		})
	}
}

// Exercise the native request sizes against real SQLite indexes, including deep
// offsets and folder previews. Fixture construction is outside each measurement.
func BenchmarkNativeGallery(b *testing.B) {
	lib := newBenchmarkIndexedLibrary(b, 5000, 300000)
	defer lib.Close()
	for _, tc := range []struct {
		name      string
		opts      ListOptions
		wantBroad bool
	}{
		{"folders-first", ListOptions{Page: 1, SkipMedia: true, SkipBlogs: true}, false},
		{"folders-last", ListOptions{Page: 208, SkipMedia: true, SkipBlogs: true}, false},
		{"media-first", ListOptions{Page: 1, Recursive: true, SkipFolders: true, SkipBlogs: true}, false},
		{"media-deep", ListOptions{Page: 3000, Recursive: true, SkipFolders: true, SkipBlogs: true}, false},
		{"search-exact", ListOptions{Query: "IMG_0004242", SkipFolders: true, SkipBlogs: true}, false},
		{"search-broad-postfilter", ListOptions{Query: "IM", SkipFolders: true, SkipBlogs: true}, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			opts := tc.opts
			opts.PageSize, opts.FolderPageSize, opts.FolderPreviewSize = 96, 24, 2
			opts.LeanMetadata = true
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				listing, err := lib.List(context.Background(), opts)
				if tc.wantBroad {
					if !errors.Is(err, ErrSearchTooBroad()) {
						b.Fatalf("broad search: %v", err)
					}
					continue
				}
				if err != nil {
					b.Fatal(err)
				}
				if len(listing.Media) > 96 || len(listing.Folders) > 24 {
					b.Fatal("unbounded native response")
				}
				if !opts.SkipMedia && len(listing.Media) == 0 {
					b.Fatal("missing media")
				}
				if !opts.SkipFolders && (len(listing.Folders) != 24 || listing.FolderTotal != 5000) {
					b.Fatalf("folders: %d / %d", len(listing.Folders), listing.FolderTotal)
				}
			}
		})
	}
}
