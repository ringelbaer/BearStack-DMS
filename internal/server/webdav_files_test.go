package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"bearstack/internal/document"
)

func TestWebDAVFileListingPreservesDuplicateNamesAndHTTPMetadata(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()
	s := &Server{repo: repo, store: store, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	date := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		content := fmt.Sprintf("duplicate %d", i)
		stored := writeWebDAVStoredFile(t, store, fmt.Sprintf("files/%d.pdf", i), []byte(content))
		if _, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: "scan.pdf", StoredPath: stored, Title: "Jürgen 100% + Rechnung",
			DocumentDate: &date, UploadedAt: date, UpdatedAt: date, SizeBytes: int64(len(content)),
			MIMEType: "application/pdf", SHA256: fmt.Sprint(i), Tags: []string{"outer", "inner"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{Name: "Bills", Tags: []string{"outer", "inner"}}); err != nil {
		t.Fatal(err)
	}
	full, err := repo.ListDocuments(ctx, document.ListFilter{Tags: []string{"outer", "inner"}, Sort: document.ListSortDate, Direction: document.ListDirectionDescending})
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range [][]string{{"outer", "inner"}, {searchFavoritesFolderName, "Bills"}} {
		parent, err := s.webDAVResolver().Resolve(ctx, prefix)
		if err != nil {
			t.Fatal(err)
		}
		children, err := s.webDAVResolver().Children(ctx, parent)
		if err != nil || len(children) != 2 {
			t.Fatalf("%v: children=%d %v", prefix, len(children), err)
		}
		for i, doc := range full {
			wantName := fmt.Sprintf("2026-05-07 - Jürgen 100%% + Rechnung (ID %d).pdf", doc.ID)
			got := children[i]
			if got.Name != wantName || got.Document.ID != doc.ID || got.Document.CustomValues != nil {
				t.Fatalf("file[%d]: %+v", i, got)
			}
			want := got
			want.Document = doc
			if !reflect.DeepEqual(webDAVResponseFor("/webdav", got), webDAVResponseFor("/webdav", want)) {
				t.Fatal("PROPFIND metadata changed")
			}
			resolved, err := s.webDAVResolver().Resolve(ctx, append(append([]string(nil), prefix...), wantName))
			if err != nil || resolved.Document.ID != doc.ID {
				t.Fatalf("duplicate resolution: %+v %v", resolved, err)
			}
			target := "/webdav"
			for _, segment := range append(append([]string(nil), prefix...), wantName) {
				target += "/" + url.PathEscape(segment)
			}
			r := newLoopbackRequest("GET", target, nil)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 200 || w.Header().Get("ETag") != webDAVETag(doc) || w.Header().Get("Last-Modified") != date.Format("Mon, 02 Jan 2006 15:04:05 GMT") {
				t.Fatalf("GET %s: %d %v", target, w.Code, w.Header())
			}
			r = newLoopbackRequest("GET", target, nil)
			r.Header.Set("If-None-Match", webDAVETag(doc))
			w = httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 304 || w.Body.Len() != 0 {
				t.Fatalf("conditional GET: %d %s", w.Code, w.Body.String())
			}
		}
	}
}
