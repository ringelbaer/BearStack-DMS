package server

import (
	"bytes"
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"bearstack/internal/document"
	"bearstack/internal/photos"
)

// database/sql consults Done when acquiring a connection. A warmed fragment
// renderer must not acquire one for unrelated navigation or page-shell data.
type renderReadContext struct {
	context.Context
	reads atomic.Int32
}

func (c *renderReadContext) Done() <-chan struct{} {
	c.reads.Add(1)
	return c.Context.Done()
}

func TestPartialRenderingPreservesContentWithoutNavigationReads(t *testing.T) {
	s := transferTestServer(t)
	s.settingsService().CacheRenderSettings(renderSettingsSnapshot{TagDisplayMode: tagDisplayModeUpper})
	data := PageData{
		Documents:       []document.Document{{ID: 1, OriginalName: "test.pdf", Tags: []string{"steuer"}}},
		DocumentColumns: []DocumentColumn{{Key: "tags", Label: "Tags"}},
		PersonFolders: photos.PersonFolderPage{PersonID: 1, Name: "Anna", Page: 1,
			Folders: []photos.PersonFolder{{Directory: "2026/20260101-Familie_Fest", DisplayPath: "2026 / Familie Fest", Count: 3}}},
	}
	for _, user := range []string{"admin", "reader"} {
		principal, ok := s.authenticateBasic(user, "secret")
		if !ok {
			t.Fatal("authentication failed")
		}
		ctx := &renderReadContext{Context: context.Background()}
		r := withAuthPrincipal(httptest.NewRequest("GET", "/", nil).WithContext(ctx), principal)
		full := s.withRenderSettings(r, data)
		if ctx.reads.Load() == 0 {
			t.Fatal("positive control did not observe navigation database reads")
		}
		for _, name := range []string{"document_table", "person_folder_content"} {
			var expected bytes.Buffer
			if err := s.templates.ExecuteTemplate(&expected, name, full); err != nil {
				t.Fatal(err)
			}
			ctx.reads.Store(0)
			w := httptest.NewRecorder()
			s.renderPartial(w, r, name, data)
			if w.Code != 200 || w.Body.String() != expected.String() {
				t.Fatalf("%s/%s changed partial output: %d\n%s\nwant:\n%s", user, name, w.Code, w.Body, expected.String())
			}
			if ctx.reads.Load() != 0 {
				t.Fatalf("%s queried page-shell data: %d context checks", name, ctx.reads.Load())
			}
		}
	}
}

func TestPartialTagModeDefaultsAndExplicitOverride(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "/", nil)
	for _, tc := range []struct{ input, want string }{{"", tagDisplayModeLower}, {"strtoupper", tagDisplayModeUpper}, {"invalid", tagDisplayModeLower}} {
		data := s.withPartialRenderSettings(r, PageData{TagDisplayMode: tc.input})
		if data.TagDisplayMode != tc.want {
			t.Fatalf("mode %q = %q, want %q", tc.input, data.TagDisplayMode, tc.want)
		}
	}
}
