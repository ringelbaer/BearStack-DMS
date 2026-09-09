package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/photos"
)

func TestPhotoCatalogFolderSearchContinuesBeyondFifty(t *testing.T) {
	s := faceTestServer(t)
	root := s.photos.Root()
	image, err := os.ReadFile(filepath.Join(root, "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 61; i++ {
		dir := filepath.Join(root, fmt.Sprintf("album-%03d", i))
		if err = os.Mkdir(dir, 0750); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"one.jpg", "two.jpg"} {
			if err = os.WriteFile(filepath.Join(dir, name), image, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if i == 60 {
			if err = os.WriteFile(filepath.Join(dir, ".adminonly"), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err = s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"album", "al", "album OR missing"} {
		for _, sort := range []string{"ascending_name", "descending_name"} {
			seen := map[string]bool{}
			for page := 1; page <= 4; page++ {
				w := labelRequest(s, "GET", fmt.Sprintf("/api/photos/v1/browse?section=folders&q=%s&sort=%s&page=%d", url.QueryEscape(query), sort, page), "reader", "")
				var out photoCatalogPage
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
					t.Fatalf("search: %d %s", w.Code, w.Body.String())
				}
				want := 24
				if page == 3 {
					want = 12
				}
				if page == 4 {
					want = 0
				}
				if out.FolderTotal != 60 || len(out.Folders) != want || out.FolderHasNext != (page < 3) {
					t.Fatalf("%q %s page %d: %d/%d next=%v", query, sort, page, len(out.Folders), out.FolderTotal, out.FolderHasNext)
				}
				for j, folder := range out.Folders {
					index := (page-1)*24 + j
					if sort == "descending_name" {
						index = 59 - index
					}
					if folder.Path != fmt.Sprintf("album-%03d", index) || seen[folder.Path] || len(folder.Previews) != 2 {
						t.Fatalf("lost/duplicate/unsorted folder: %+v", folder)
					}
					seen[folder.Path] = true
				}
			}
			if len(seen) != 60 {
				t.Fatalf("unreachable folders: %d", len(seen))
			}
		}
	}
}

func TestPhotoCatalogReaderAndValidation(t *testing.T) {
	s := faceTestServer(t)
	for _, user := range []string{"reader", "editor", "manager"} {
		w := labelRequest(s, "GET", "/api/photos/v1/session", user, "")
		var result struct {
			Protocol  int    `json:"protocol"`
			CanManage bool   `json:"can_manage_people"`
			Account   string `json:"account"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Protocol != 1 || result.Account == "" || result.CanManage != (user != "reader") {
			t.Fatalf("session %s: %d %s", user, w.Code, w.Body.String())
		}
	}
	for _, test := range []struct {
		path   string
		status int
	}{
		{"session", 200}, {"browse", 200}, {"browse?section=folders", 200}, {"browse?section=blogs", 200},
		{"browse?page=0", 400}, {"browse?page=-1", 400}, {"browse?page=1000001", 400}, {"browse?page=x", 400},
		{"browse?sort=sql", 400}, {"browse?type=sql", 400}, {"browse?section=sql", 400}, {"browse?recursive=yes", 400},
		{"browse?path=../outside", 400}, {"browse?q=" + strings.Repeat("x", 801), 400},
		{"media/info?path=one.jpg", 200}, {"media/info?path=../outside", 400}, {"blog?path=../outside", 400},
		{"blog?path=one.jpg", 404},
	} {
		w := labelRequest(s, "GET", "/api/photos/v1/"+test.path, "reader", "")
		if w.Code != test.status || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Errorf("%s: %d %s", test.path, w.Code, w.Body.String())
		}
		if w := labelRequest(s, "GET", "/api/photos/v1/"+test.path, "", ""); w.Code != 401 {
			t.Errorf("unauthenticated %s: %d", test.path, w.Code)
		}
	}
	if w := labelRequest(s, "POST", "/api/photos/v1/browse", "reader", "{}"); w.Code != 405 {
		t.Fatalf("write admitted: %d", w.Code)
	}
}

func TestPhotoCatalogPagingContentAndPrivatePaths(t *testing.T) {
	s := faceTestServer(t)
	root := s.photos.Root()
	image, err := os.ReadFile(filepath.Join(root, "one.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 27; i++ {
		for j := 0; j < 3; j++ {
			write(fmt.Sprintf("album-%02d/%d.jpg", i, j), image)
		}
	}
	for i := 0; i < 45; i++ {
		write(fmt.Sprintf("post-%02d.md", i), []byte("# Travel\n<script>bad()</script>"))
	}
	for i := 0; i < 101; i++ {
		write(fmt.Sprintf("image-%03d.jpg", i), image)
	}
	write("private/.adminonly", nil)
	write("private/secret.jpg", image)
	write("private/secret.md", []byte("Secret"))
	write("notes.txt", []byte("# Notes & details\n  keep indentation"))
	write(".hidden.md", []byte("Hidden content"))
	if _, err := s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	read := func(query string) photoCatalogPage {
		t.Helper()
		w := labelRequest(s, "GET", "/api/photos/v1/browse?"+query, "reader", "")
		var out photoCatalogPage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("browse: %d %s", w.Code, w.Body.String())
		}
		return out
	}
	first := read("sort=ascending_name")
	if len(first.Media) != 96 || !first.HasNext || first.Total != 102 || len(first.Folders) != 24 || !first.FolderHasNext || first.FolderTotal != 27 || len(first.Blogs) != 20 || !first.BlogHasNext {
		t.Fatalf("page lengths: media=%d/%d folders=%d/%d blogs=%d", len(first.Media), first.Total, len(first.Folders), first.FolderTotal, len(first.Blogs))
	}
	for _, folder := range first.Folders {
		if len(folder.Previews) != 2 {
			t.Fatalf("previews: %s %d", folder.Path, len(folder.Previews))
		}
	}
	for _, post := range first.Blogs {
		if post.Text != "" || post.HTML != "" {
			t.Fatal("body loaded for summary")
		}
	}
	second := read("sort=ascending_name&page=2")
	if len(second.Media) != 6 || second.HasNext || len(second.Folders) != 3 || second.FolderHasNext || len(second.Blogs) != 20 || !second.BlogHasNext {
		t.Fatal("incorrect second page")
	}
	third := read("section=blogs&page=3")
	if len(third.Blogs) != 6 || third.BlogHasNext || len(third.Media) != 0 || len(third.Folders) != 0 {
		t.Fatal("incorrect final blog page")
	}
	if out := read("section=media&q=image-100"); len(out.Media) != 1 || out.Media[0].Path != "image-100.jpg" {
		t.Fatal("search failed")
	}
	w := labelRequest(s, "GET", "/api/photos/v1/blog?path=post-00.md", "reader", "")
	var post struct {
		Blog photoCatalogBlog `json:"blog"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &post) != nil || !strings.Contains(post.Blog.HTML, "<h2>Travel</h2>") || strings.Contains(post.Blog.HTML, "<script>") {
		t.Fatalf("unsafe/lost markup: %s", w.Body.String())
	}
	w = labelRequest(s, "GET", "/api/photos/v1/blog?path=notes.txt", "reader", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &post) != nil || post.Blog.Text != "# Notes & details\n  keep indentation" || !strings.HasPrefix(post.Blog.HTML, "<pre># Notes &amp; details") {
		t.Fatalf("plain text changed: %s", w.Body.String())
	}
	if w := labelRequest(s, "GET", "/api/photos/v1/blog?path=.hidden.md", "reader", ""); w.Code != 404 {
		t.Fatalf("hidden text: %d", w.Code)
	}
	for _, path := range []string{"browse?path=private", "media/info?path=private/secret.jpg", "media?path=private/secret.jpg", "thumbnail?path=private/secret.jpg", "blog?path=private/secret.md"} {
		if w := labelRequest(s, "GET", "/api/photos/v1/"+path, "reader", ""); w.Code != 403 {
			t.Errorf("private %s: %d", path, w.Code)
		}
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"blog?path=link/secret.md", "media/info?path=link/secret.jpg", "browse?path=link"} {
		if w := labelRequest(s, "GET", "/api/photos/v1/"+path, "reader", ""); w.Code != 400 {
			t.Errorf("symlink %s: %d", path, w.Code)
		}
	}
	// Original delivery shares the browser's streaming/range implementation.
	r := httptest.NewRequest("GET", "/api/photos/v1/media?path="+url.QueryEscape("one.jpg"), nil)
	r.SetBasicAuth("reader", "secret")
	r.Header.Set("Range", "bytes=0-9")
	rw := httptest.NewRecorder()
	s.Handler().ServeHTTP(rw, r)
	if rw.Code != 206 || rw.Body.Len() != 10 {
		t.Fatalf("range: %d %d", rw.Code, rw.Body.Len())
	}
}

func TestPhotoCatalogLibraryPaginationFallback(t *testing.T) {
	s := faceTestServer(t)
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(filepath.Join(s.photos.Root(), fmt.Sprintf("text-%d.md", i)), []byte("hello"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.photos.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		listing, err := s.photos.List(context.Background(), photos.ListOptions{Page: 2, PageSize: 2, BlogPageSize: 2, BlogSummaries: true, FullFilesystem: full})
		if err != nil || len(listing.Blogs) != 2 || !listing.BlogHasNext {
			t.Fatalf("fallback=%v blogs=%d next=%v err=%v", full, len(listing.Blogs), listing.BlogHasNext, err)
		}
	}
}
