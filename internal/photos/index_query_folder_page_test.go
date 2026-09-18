package photos

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestIndexedFolderPagesMatchUnpagedListing(t *testing.T) {
	l := routeTestLibrary(t)
	// Date prefixes, Unicode/case ties, underscores, nesting and hidden-only containers.
	names := []string{"2026-09-18_Sommer_Urlaub", "2025-01-01_Winter", "alpha", "Alpha", "Äpfel", "äpfel", "Öl", "zebra", "empty", "private", "container", "blog", "unknown", "inner"}
	for i, name := range names {
		parent := ""
		path := name
		if name == "inner" {
			parent = "alpha"
			path = "alpha/2026-01-01_Nested_Folder"
			name = "2026-01-01_Nested_Folder"
		}
		private := 0
		if name == "private" {
			private = 1
		}
		public, total, blogs := 1, 1, 0
		if name == "container" {
			public = 0
		}
		if name == "empty" {
			public = 0
			total = 0
		}
		if name == "blog" {
			public = 0
			total = 0
			blogs = 1
		}
		_, err := l.index.db.Exec(`INSERT INTO folder_index(path,parent,name,media_count,public_media_count,recursive_media_count,public_recursive_media_count,recursive_blog_count,public_recursive_blog_count,dir_count,mod_time_unix_nano,tags,admin_only,indexed_at)
   VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, path, parent, name, total, public, total, public, blogs, blogs, 1, i, `["tag"]`, private, "2026-09-18")
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, admin := range []bool{false, true} {
		for _, people := range []bool{false, true} {
			for _, order := range []string{"ascending_name", "descending_name", "ascending_date", "descending_date", "random"} {
				t.Run(fmt.Sprintf("%v/%v/%s", admin, people, order), func(t *testing.T) {
					opts := ListOptions{Sort: order, SkipMedia: true, SkipBlogs: true, IncludeAdminOnly: admin, IncludePeopleFolders: people}
					all, err := l.List(context.Background(), opts)
					if err != nil {
						t.Fatal(err)
					}
					for _, size := range []int{1, 4, 24} {
						for page := 1; page <= (len(all.Folders)+size-1)/size+1; page++ {
							opts.FolderPageSize, opts.Page = size, page
							got, err := l.List(context.Background(), opts)
							if err != nil {
								t.Fatal(err)
							}
							want, next := listingPage(all.Folders, page, size)
							if len(got.Folders) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got.Folders, want)) || got.FolderHasNext != next || got.FolderTotal != len(all.Folders) {
								t.Fatalf("size=%d page=%d: got=%+v want=%+v total=%d next=%v/%v", size, page, got.Folders, want, got.FolderTotal, got.FolderHasNext, next)
							}
							if got.foldersPaged != (order == "ascending_name" || order == "descending_name") {
								t.Fatal("wrong pagination path")
							}
						}
					}
				})
			}
		}
	}
	if _, err := l.index.db.Exec(`INSERT INTO photo_folder_scan(path,mod_time_unix_nano,order_mode,scanned_at) VALUES('alpha',0,'ascending_name','2026-09-18')`); err != nil {
		t.Fatal(err)
	}
	for _, order := range []string{"ascending_name", "descending_name"} {
		opts := ListOptions{Path: "alpha", Sort: order, SkipMedia: true, SkipBlogs: true, IncludePeopleFolders: true}
		all, err := l.List(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		opts.FolderPageSize = 1
		got, err := l.List(context.Background(), opts)
		if err != nil {
			t.Fatal(err)
		}
		if !got.foldersPaged || !reflect.DeepEqual(got.Folders, all.Folders) || got.FolderTotal != 1 || got.Folders[0].DisplayName != "Nested Folder" {
			t.Fatalf("nested folder page: %+v / %+v", got, all)
		}
	}
	// Old/migrating visibility counters must use the existing repair/filter path.
	if _, err := l.index.db.Exec(`UPDATE folder_index SET public_recursive_media_count=-1 WHERE path='unknown'`); err != nil {
		t.Fatal(err)
	}
	opts := ListOptions{Sort: "ascending_name", SkipMedia: true, SkipBlogs: true}
	all, err := l.List(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`UPDATE folder_index SET public_recursive_media_count=-1 WHERE path='unknown'`); err != nil {
		t.Fatal(err)
	}
	opts.FolderPageSize = 4
	got, err := l.List(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.foldersPaged || got.FolderTotal != len(all.Folders) || !reflect.DeepEqual(got.Folders, all.Folders[:4]) {
		t.Fatal("incomplete visibility did not preserve fallback")
	}
}

func BenchmarkIndexedFolderPage(b *testing.B) {
	l := routeTestLibrary(b)
	if _, err := l.index.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10000)
 INSERT INTO folder_index(path,parent,name,media_count,public_media_count,recursive_media_count,public_recursive_media_count,public_recursive_blog_count,indexed_at)
 SELECT printf('album-%05d',x),'',printf('album-%05d',x),1,1,1,1,0,'2026-09-18' FROM n`); err != nil {
		b.Fatal(err)
	}
	for _, page := range []int{1, 417} {
		for _, paged := range []bool{false, true} {
			b.Run(fmt.Sprintf("page-%d/sql-%v", page, paged), func(b *testing.B) {
				opts := ListOptions{Sort: "ascending_name", SkipMedia: true, SkipBlogs: true, Page: page, FolderPageSize: 24}
				b.ReportAllocs()
				for b.Loop() {
					listing := newListing("", opts)
					var err error
					if paged {
						listing.foldersPaged, err = l.indexFolderPage(context.Background(), "", opts, &listing)
					} else {
						listing.Folders, err = l.indexFolders(context.Background(), "", false)
					}
					if err != nil {
						b.Fatal(err)
					}
					if err = l.finishListing(context.Background(), opts, &listing, indexedListingSource); err != nil {
						b.Fatal(err)
					}
					want := 24
					if page == 417 {
						want = 17
					}
					if listing.FolderTotal != 10001 || len(listing.Folders) != want {
						b.Fatalf("wrong folder page: %d/%d", len(listing.Folders), listing.FolderTotal)
					}
				}
			})
		}
	}
}
