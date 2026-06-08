package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bearstack/internal/config"
	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

func TestWebDAVWellKnownRedirect(t *testing.T) {
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	tests := []struct {
		name     string
		method   string
		target   string
		location string
	}{
		{
			name:     "root",
			method:   "PROPFIND",
			target:   "/.well-known/webdav",
			location: "/webdav/",
		},
		{
			name:     "subpath",
			method:   "PROPFIND",
			target:   "/.well-known/webdav/steuer",
			location: "/webdav/steuer",
		},
		{
			name:     "escaped subpath",
			method:   "PROPFIND",
			target:   "/.well-known/webdav/Steuer%202026",
			location: "/webdav/Steuer%202026",
		},
		{
			name:     "query",
			method:   http.MethodGet,
			target:   "/.well-known/webdav/?depth=1",
			location: "/webdav/?depth=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			rec := httptest.NewRecorder()

			server.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusPermanentRedirect {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Location"); got != tt.location {
				t.Fatalf("location = %q, want %q", got, tt.location)
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != "" {
				t.Fatalf("unexpected auth challenge = %q", got)
			}
		})
	}
}

func TestWebDAVConfiguredPathStillUsesBasicAuthChallengeWithoutSession(t *testing.T) {
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/webdav", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatalf("missing auth challenge")
	}
}

func TestWebDAVConfiguredPathRoutesRedirectsAndHrefs(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer.pdf",
		StoredPath:   "steuer.pdf",
		Title:        "Steuer",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "webdav-custom-path",
	}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg: config.Config{
			WebDAV: config.WebDAVConfig{Path: "/dav"},
		},
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	handler := server.Handler()

	req := httptest.NewRequest("PROPFIND", "/dav/", nil)
	req.Header.Set("Depth", "1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/dav/steuer/") {
		t.Fatalf("custom webdav href missing: %s", body)
	}
	if strings.Contains(body, "/webdav/steuer/") {
		t.Fatalf("default webdav href leaked into custom response: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/.well-known/webdav/steuer", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusPermanentRedirect {
		t.Fatalf("well-known status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/dav/steuer" {
		t.Fatalf("well-known location = %q", got)
	}

	req = httptest.NewRequest("PROPFIND", "/webdav/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("default route status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestWebDAVRootPropfindListsSearchFavoritesFirst(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer.pdf",
		StoredPath:   "steuer.pdf",
		Title:        "Steuer",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "webdav-root-steuer",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name: "Steuer Favorit",
		Tags: []string{"steuer"},
	}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest("PROPFIND", "/webdav/", nil)
	req.Header.Set("Depth", "1")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	favoritesIndex := strings.Index(body, "/webdav/00%20Suchfavoriten/")
	tagIndex := strings.Index(body, "/webdav/steuer/")
	if favoritesIndex < 0 || tagIndex < 0 || favoritesIndex > tagIndex {
		t.Fatalf("unexpected root listing order: %s", body)
	}
	if strings.Contains(body, "Steuer.pdf") {
		t.Fatalf("root listing contains document file: %s", body)
	}
}

func TestWebDAVRootPropfindFiltersTagsByConfiguredMinimum(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	for i, doc := range []document.Document{
		{OriginalName: "haeufig-selten.pdf", StoredPath: "haeufig-selten.pdf", Title: "Haeufig Selten", Tags: []string{"haeufig", "selten"}, MIMEType: "application/pdf", SizeBytes: 1},
		{OriginalName: "haeufig.pdf", StoredPath: "haeufig.pdf", Title: "Haeufig", Tags: []string{"haeufig"}, MIMEType: "application/pdf", SizeBytes: 1},
	} {
		doc.SHA256 = fmt.Sprintf("webdav-min-root-%d", i)
		if _, err := repo.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SaveSetting(ctx, folderTagMinDocumentsSettingKey, "2"); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest("PROPFIND", "/webdav/", nil)
	req.Header.Set("Depth", "1")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("root status = %d body = %s", rec.Code, rec.Body.String())
	}
	rootBody := rec.Body.String()
	if !strings.Contains(rootBody, "/webdav/haeufig/") {
		t.Fatalf("root tag with enough documents missing: %s", rootBody)
	}
	if strings.Contains(rootBody, "/webdav/selten/") {
		t.Fatalf("root tag below minimum rendered: %s", rootBody)
	}

	req = httptest.NewRequest("PROPFIND", "/webdav/haeufig/", nil)
	req.Header.Set("Depth", "1")
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("child status = %d body = %s", rec.Code, rec.Body.String())
	}
	childBody := rec.Body.String()
	if !strings.Contains(childBody, "/webdav/haeufig/selten/") {
		t.Fatalf("child tag below root minimum should stay visible below first level: %s", childBody)
	}
}

func TestWebDAVTagPropfindGetAndHeadDocument(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()

	content := []byte("%PDF webdav")
	storedPath := writeWebDAVStoredFile(t, store, "2026/05/rechnung.pdf", content)
	docDate := time.Date(2026, time.May, 7, 0, 0, 0, 0, time.UTC)
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "scan.pdf",
		StoredPath:   storedPath,
		Title:        "Rechnung",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    int64(len(content)),
		SHA256:       "webdav-get-document",
		DocumentDate: &docDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest("PROPFIND", "/webdav/steuer/", nil)
	req.Header.Set("Depth", "1")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	name := documentDisplayName(doc)
	if !strings.Contains(rec.Body.String(), url.PathEscape(name)) {
		t.Fatalf("propfind body missing %q: %s", name, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<getcontenttype>application/pdf</getcontenttype>") {
		t.Fatalf("propfind body missing PDF content type: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), fmt.Sprintf("<getcontentlength>%d</getcontentlength>", len(content))) {
		t.Fatalf("propfind body missing content length: %s", rec.Body.String())
	}

	target := "/webdav/steuer/" + url.PathEscape(name)
	getReq := httptest.NewRequest(http.MethodGet, target, nil)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body = %s", getRec.Code, getRec.Body.String())
	}
	if !bytes.Equal(getRec.Body.Bytes(), content) {
		t.Fatalf("GET body = %q", getRec.Body.Bytes())
	}
	if got := getRec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/pdf") {
		t.Fatalf("content type = %q", got)
	}
	if got := getRec.Header().Get("ETag"); got == "" {
		t.Fatal("missing ETag")
	}

	rangeReq := httptest.NewRequest(http.MethodGet, target, nil)
	rangeReq.Header.Set("Range", "bytes=0-3")
	rangeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rangeRec, rangeReq)
	if rangeRec.Code != http.StatusPartialContent {
		t.Fatalf("Range GET status = %d body = %s", rangeRec.Code, rangeRec.Body.String())
	}
	if got := rangeRec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("accept ranges = %q", got)
	}
	if got := rangeRec.Header().Get("Content-Range"); got != fmt.Sprintf("bytes 0-3/%d", len(content)) {
		t.Fatalf("content range = %q", got)
	}
	if !bytes.Equal(rangeRec.Body.Bytes(), content[:4]) {
		t.Fatalf("Range GET body = %q", rangeRec.Body.Bytes())
	}

	headReq := httptest.NewRequest(http.MethodHead, target, nil)
	headRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(headRec, headReq)
	if headRec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d body = %s", headRec.Code, headRec.Body.String())
	}
	if headRec.Body.Len() != 0 {
		t.Fatalf("HEAD returned body %q", headRec.Body.String())
	}
	if got := headRec.Header().Get("Content-Length"); got != fmt.Sprint(len(content)) {
		t.Fatalf("content length = %q", got)
	}
}

func TestWebDAVTagFolderListsConfiguredCustomFieldValueFolders(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kunde", false, document.CustomFieldValueFolderAlways); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Projekt", false, 5); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var kundeID, projektID int64
	for _, field := range fields {
		switch field.Label {
		case "Kunde":
			kundeID = field.ID
		case "Projekt":
			projektID = field.ID
		}
	}

	acmePath := writeWebDAVStoredFile(t, store, "2026/05/acme.pdf", []byte("%PDF acme"))
	acmeID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "acme.pdf",
		StoredPath:   acmePath,
		Title:        "ACME",
		Tags:         []string{"steuer", "privat"},
		MIMEType:     "application/pdf",
		SizeBytes:    9,
		SHA256:       "webdav-field-acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, acmeID, "ACME", "", nil, []string{"steuer", "privat"}, map[int64]string{kundeID: "ACME", projektID: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	betaPath := writeWebDAVStoredFile(t, store, "2026/05/beta.pdf", []byte("%PDF beta"))
	betaID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "beta.pdf",
		StoredPath:   betaPath,
		Title:        "Beta",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    9,
		SHA256:       "webdav-field-beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, betaID, "Beta", "", nil, []string{"steuer"}, map[int64]string{kundeID: "Beta", projektID: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	acmeDoc, err := repo.GetDocument(ctx, acmeID)
	if err != nil {
		t.Fatal(err)
	}
	betaDoc, err := repo.GetDocument(ctx, betaID)
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest("PROPFIND", "/webdav/steuer/", nil)
	req.Header.Set("Depth", "1")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<displayname>ACME</displayname>") || !strings.Contains(body, "<displayname>Beta</displayname>") {
		t.Fatalf("tag folder missing custom field folders: %s", body)
	}
	if strings.Contains(body, "Kunde: ACME") || strings.Contains(body, "Kunde: Beta") {
		t.Fatalf("webdav field value folders include field labels: %s", body)
	}
	if strings.Contains(body, "Projekt: Alpha") || strings.Contains(body, "<displayname>Alpha</displayname>") {
		t.Fatalf("thresholded field value folder leaked: %s", body)
	}

	target := "/webdav/steuer/" + url.PathEscape("ACME") + "/"
	req = httptest.NewRequest("PROPFIND", target, nil)
	req.Header.Set("Depth", "1")
	rec = httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("field folder status = %d body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, "/webdav/steuer/ACME/privat/") {
		t.Fatalf("field folder missing next tag folder: %s", body)
	}
	if !strings.Contains(body, url.PathEscape(documentDisplayName(acmeDoc))) || strings.Contains(body, url.PathEscape(documentDisplayName(betaDoc))) {
		t.Fatalf("field folder documents = %s", body)
	}
}

func TestWebDAVSearchFavoritePropfindListsMatchingActiveDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	matchingDate := time.Date(2026, time.May, 7, 0, 0, 0, 0, time.UTC)
	matchingID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "matching.pdf",
		StoredPath:   "matching.pdf",
		Title:        "Treffer",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "webdav-favorite-match",
		DocumentDate: &matchingDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "other.pdf",
		StoredPath:   "other.pdf",
		Title:        "Andere",
		Tags:         []string{"privat"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "webdav-favorite-other",
	}); err != nil {
		t.Fatal(err)
	}
	trashedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "trash.pdf",
		StoredPath:   "trash.pdf",
		Title:        "Papierkorb",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "webdav-favorite-trash",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, trashedID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name: "Steuer 2026",
		Tags: []string{"steuer"},
	}); err != nil {
		t.Fatal(err)
	}
	matching, err := repo.GetDocument(ctx, matchingID)
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	target := "/webdav/" + url.PathEscape(searchFavoritesFolderName) + "/" + url.PathEscape(escapePathComponent("Steuer 2026")) + "/"
	req := httptest.NewRequest("PROPFIND", target, nil)
	req.Header.Set("Depth", "1")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, url.PathEscape(documentDisplayName(matching))) {
		t.Fatalf("favorite listing missing match: %s", body)
	}
	if strings.Contains(body, "Andere") || strings.Contains(body, "Papierkorb") {
		t.Fatalf("favorite listing contains excluded document: %s", body)
	}
}

func TestWebDAVPutCreatesDocumentsAndInheritsTagsFromHierarchy(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()

	if _, err := repo.SaveTag(ctx, "steuer", "", "#176b87", false, false); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Kunde", false, document.CustomFieldValueFolderAlways); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var kundeID int64
	for _, field := range fields {
		if field.Label == "Kunde" {
			kundeID = field.ID
		}
	}
	if kundeID == 0 {
		t.Fatal("missing custom field Kunde")
	}

	seedPath := writeWebDAVStoredFile(t, store, "2026/05/seed.pdf", []byte("%PDF seed"))
	seedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "seed.pdf",
		StoredPath:   seedPath,
		Title:        "Seed",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    int64(len("%PDF seed")),
		SHA256:       "webdav-put-seed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, seedID, "Seed", "", nil, []string{"steuer"}, map[int64]string{kundeID: "ACME"}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	tests := []struct {
		target string
		body   []byte
	}{
		{target: "/webdav/root-upload.pdf", body: []byte("%PDF root")},
		{target: "/webdav/steuer/tag-upload.pdf", body: []byte("%PDF tag")},
		{target: "/webdav/steuer/ACME/folder-upload.pdf", body: []byte("%PDF folder")},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodPut, tt.target, bytes.NewReader(tt.body))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("PUT %s status = %d body = %s", tt.target, rec.Code, rec.Body.String())
		}
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	byOriginalName := map[string]document.Document{}
	for _, doc := range docs {
		byOriginalName[doc.OriginalName] = doc
	}

	assertDoc := func(name string, wantTags []string) {
		t.Helper()
		doc, ok := byOriginalName[name]
		if !ok {
			t.Fatalf("missing uploaded document %q", name)
		}
		if doc.UploadWay != document.UploadWayWebDAV {
			t.Fatalf("%s upload way = %q", name, doc.UploadWay)
		}
		if len(doc.Tags) != len(wantTags) {
			t.Fatalf("%s tags = %#v, want %#v", name, doc.Tags, wantTags)
		}
		for i := range wantTags {
			if doc.Tags[i] != wantTags[i] {
				t.Fatalf("%s tags = %#v, want %#v", name, doc.Tags, wantTags)
			}
		}
	}

	assertDoc("root-upload.pdf", []string{})
	assertDoc("tag-upload.pdf", []string{"steuer"})
	assertDoc("folder-upload.pdf", []string{"steuer"})
}

func TestWebDAVPutRejectsExistingTargetsAndDuplicates(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()

	existingContent := []byte("%PDF existing")
	checksum := sha256.Sum256(existingContent)
	storedPath := writeWebDAVStoredFile(t, store, "2026/05/rechnung.pdf", []byte("%PDF existing"))
	docDate := time.Date(2026, time.May, 7, 0, 0, 0, 0, time.UTC)
	existingID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung.pdf",
		StoredPath:   storedPath,
		Title:        "Rechnung",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    int64(len(existingContent)),
		SHA256:       hex.EncodeToString(checksum[:]),
		DocumentDate: &docDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	existingDoc, err := repo.GetDocument(ctx, existingID)
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	before, err := repo.CountDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}

	existingTarget := "/webdav/steuer/" + url.PathEscape(documentDisplayName(existingDoc))
	req := httptest.NewRequest(http.MethodPut, existingTarget, bytes.NewReader([]byte("%PDF overwrite")))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("existing target status = %d body = %s", rec.Code, rec.Body.String())
	}

	duplicateReq := httptest.NewRequest(http.MethodPut, "/webdav/steuer/new-name.pdf", bytes.NewReader(existingContent))
	duplicateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(duplicateRec, duplicateReq)
	if duplicateRec.Code != http.StatusConflict {
		t.Fatalf("duplicate target status = %d body = %s", duplicateRec.Code, duplicateRec.Body.String())
	}

	after, err := repo.CountDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("document count changed from %d to %d", before, after)
	}
}

func TestWebDAVPutAllowsSameOriginalNameWhenTargetPathDoesNotExist(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()

	existingContent := []byte("%PDF existing display-name")
	storedPath := writeWebDAVStoredFile(t, store, "2026/05/existing-name.pdf", existingContent)
	docDate := time.Date(2026, time.May, 7, 0, 0, 0, 0, time.UTC)
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "202311-KK000051929807_Bescheinigung_03_2023-11-28.pdf",
		StoredPath:   storedPath,
		Title:        "Bescheinigung",
		Tags:         []string{"gesundheit"},
		MIMEType:     "application/pdf",
		SizeBytes:    int64(len(existingContent)),
		SHA256:       "webdav-put-existing-original-name",
		DocumentDate: &docDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	existingDoc, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	displayName := documentDisplayName(existingDoc)
	if displayName == existingDoc.OriginalName {
		t.Fatalf("expected display name to differ from original name, got %q", displayName)
	}

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	target := "/webdav/gesundheit/202311-KK000051929807_Bescheinigung_03_2023-11-28.pdf"

	probeReq := httptest.NewRequest("PROPFIND", target, nil)
	probeReq.Header.Set("Depth", "0")
	probeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(probeRec, probeReq)
	if probeRec.Code != http.StatusNotFound {
		t.Fatalf("probe status = %d body = %s", probeRec.Code, probeRec.Body.String())
	}

	putReq := httptest.NewRequest(http.MethodPut, target, bytes.NewReader([]byte("%PDF new content")))
	putRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusCreated {
		t.Fatalf("PUT status = %d body = %s", putRec.Code, putRec.Body.String())
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	matches := 0
	for _, doc := range docs {
		if doc.OriginalName != "202311-KK000051929807_Bescheinigung_03_2023-11-28.pdf" {
			continue
		}
		matches++
	}
	if matches != 2 {
		t.Fatalf("documents with same original name = %d, want 2", matches)
	}
}

func TestWebDAVPutRejectsSearchFavoritesPaths(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()
	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	target := "/webdav/" + url.PathEscape(searchFavoritesFolderName) + "/" + url.PathEscape("Steuer Favorit") + "/neu.pdf"
	req := httptest.NewRequest(http.MethodPut, target, bytes.NewReader([]byte("%PDF blocked")))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestWebDAVSpaceTagPathsAcceptClientEncodingVariants(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()

	if _, err := repo.SaveTag(ctx, "katha arbeit", "", "#176b87", false, false); err != nil {
		t.Fatal(err)
	}

	seedContent := []byte("%PDF seed tag space")
	seedPath := writeWebDAVStoredFile(t, store, "2026/05/seed-space.pdf", seedContent)
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "seed-space.pdf",
		StoredPath:   seedPath,
		Title:        "Seed Space",
		Tags:         []string{"katha arbeit"},
		MIMEType:     "application/pdf",
		SizeBytes:    int64(len(seedContent)),
		SHA256:       "webdav-space-tag-seed",
	}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	type pathVariant struct {
		name      string
		folderURL string
		filename  string
		content   []byte
	}

	variants := []pathVariant{
		{
			name:      "escaped-space",
			folderURL: "/webdav/katha%20arbeit/",
			filename:  "upload-escaped.pdf",
			content:   []byte("%PDF escaped"),
		},
		{
			name:      "double-escaped-space",
			folderURL: "/webdav/katha%2520arbeit/",
			filename:  "upload-double-escaped.pdf",
			content:   []byte("%PDF double escaped"),
		},
		{
			name:      "plus-space",
			folderURL: "/webdav/katha+arbeit/",
			filename:  "upload-plus.pdf",
			content:   []byte("%PDF plus"),
		},
	}

	for _, variant := range variants {
		t.Run(variant.name+"-propfind", func(t *testing.T) {
			req := httptest.NewRequest("PROPFIND", variant.folderURL, nil)
			req.Header.Set("Depth", "0")
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusMultiStatus {
				t.Fatalf("PROPFIND %s status = %d body = %s", variant.folderURL, rec.Code, rec.Body.String())
			}
		})

		t.Run(variant.name+"-put", func(t *testing.T) {
			target := variant.folderURL + variant.filename
			req := httptest.NewRequest(http.MethodPut, target, bytes.NewReader(variant.content))
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("PUT %s status = %d body = %s", target, rec.Code, rec.Body.String())
			}
		})
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, uploaded := range []string{"upload-escaped.pdf", "upload-double-escaped.pdf", "upload-plus.pdf"} {
		found := false
		for _, doc := range docs {
			if doc.OriginalName != uploaded {
				continue
			}
			found = true
			if doc.UploadWay != document.UploadWayWebDAV {
				t.Fatalf("%s upload way = %q", uploaded, doc.UploadWay)
			}
			if len(doc.Tags) != 1 || doc.Tags[0] != "katha arbeit" {
				t.Fatalf("%s tags = %#v", uploaded, doc.Tags)
			}
			break
		}
		if !found {
			t.Fatalf("missing uploaded document %q", uploaded)
		}
	}
}

func TestWebDAVPutRequiresUploadCapability(t *testing.T) {
	ctx := context.Background()
	repo, store := newWebDAVTestRepoAndStore(t, ctx)
	defer repo.Close()
	if _, err := repo.SaveTag(ctx, "steuer", "", "#176b87", false, false); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg: config.Config{
			MaxUploadBytes: 1 << 20,
			Auth: config.AuthConfig{
				Credentials: []config.AuthCredential{
					{Username: "reader", Password: "secret", Role: "documents_read"},
					{Username: "editor", Password: "secret", Role: "documents_editor"},
				},
			},
		},
		repo:    repo,
		store:   store,
		authKey: []byte("01234567890123456789012345678901"),
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	readerReq := httptest.NewRequest(http.MethodPut, "/webdav/steuer/reader.pdf", bytes.NewReader([]byte("%PDF reader")))
	readerReq.SetBasicAuth("reader", "secret")
	readerReq.Header.Set("Accept", "application/json")
	readerRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(readerRec, readerReq)
	if readerRec.Code != http.StatusForbidden {
		t.Fatalf("reader status = %d body = %s", readerRec.Code, readerRec.Body.String())
	}

	editorReq := httptest.NewRequest(http.MethodPut, "/webdav/steuer/editor.pdf", bytes.NewReader([]byte("%PDF editor")))
	editorReq.SetBasicAuth("editor", "secret")
	editorRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(editorRec, editorReq)
	if editorRec.Code != http.StatusCreated {
		t.Fatalf("editor status = %d body = %s", editorRec.Code, editorRec.Body.String())
	}
}

func TestWebDAVNonPutWriteMethodsAreReadOnly(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	before, err := repo.CountDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{http.MethodDelete, "MKCOL", "MOVE", "COPY", "PROPPATCH", "LOCK", "UNLOCK"} {
		req := httptest.NewRequest(method, "/webdav/neu.pdf", strings.NewReader("content"))
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d body = %s", method, rec.Code, rec.Body.String())
		}
	}
	after, err := repo.CountDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("document count changed from %d to %d", before, after)
	}
}

func TestWebDAVRejectsUnsupportedDepthAndEmptyPathSegment(t *testing.T) {
	server := &Server{
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	depthReq := httptest.NewRequest("PROPFIND", "/webdav/", nil)
	depthReq.Header.Set("Depth", "infinity")
	depthRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(depthRec, depthReq)
	if depthRec.Code != http.StatusForbidden {
		t.Fatalf("depth status = %d body = %s", depthRec.Code, depthRec.Body.String())
	}

	pathReq := httptest.NewRequest("PROPFIND", "/webdav/%20%20", nil)
	pathReq.Header.Set("Depth", "1")
	pathRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(pathRec, pathReq)
	if pathRec.Code != http.StatusBadRequest {
		t.Fatalf("path status = %d body = %s", pathRec.Code, pathRec.Body.String())
	}
}

func TestWebDAVOptionsAllowIncludesPut(t *testing.T) {
	server := &Server{
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodOptions, "/webdav/", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Allow"); got != "OPTIONS, PROPFIND, GET, HEAD, PUT" {
		t.Fatalf("allow = %q", got)
	}
}

func TestAPIFoldersSearchFavoriteVirtualPath(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer.pdf",
		StoredPath:   "steuer.pdf",
		Title:        "Steuer",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "api-folder-favorite",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name: "Steuer Favorit",
		Tags: []string{"steuer"},
	}); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	rootReq := httptest.NewRequest(http.MethodGet, "/api/folders?page_size=25", nil)
	rootRec := httptest.NewRecorder()
	server.handleAPIFolders(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("root status = %d body = %s", rootRec.Code, rootRec.Body.String())
	}
	var rootPayload struct {
		Folders []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"folders"`
	}
	if err := json.NewDecoder(rootRec.Body).Decode(&rootPayload); err != nil {
		t.Fatal(err)
	}
	if len(rootPayload.Folders) < 2 || rootPayload.Folders[0].Name != searchFavoritesFolderName || rootPayload.Folders[0].Kind != "search_favorites" {
		t.Fatalf("root folders = %#v", rootPayload.Folders)
	}

	favReq := httptest.NewRequest(http.MethodGet, "/api/folders?tags="+url.QueryEscape(searchFavoritesFolderName)+"&tags="+url.QueryEscape("Steuer Favorit")+"&page_size=25", nil)
	favRec := httptest.NewRecorder()
	server.handleAPIFolders(favRec, favReq)
	if favRec.Code != http.StatusOK {
		t.Fatalf("favorite status = %d body = %s", favRec.Code, favRec.Body.String())
	}
	var favPayload struct {
		Documents []struct {
			ID int64 `json:"id"`
		} `json:"documents"`
	}
	if err := json.NewDecoder(favRec.Body).Decode(&favPayload); err != nil {
		t.Fatal(err)
	}
	if len(favPayload.Documents) != 1 || favPayload.Documents[0].ID != docID {
		t.Fatalf("favorite documents = %#v", favPayload.Documents)
	}
}

func TestFoldersPageRendersSearchFavoritesRootTile(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer.pdf",
		StoredPath:   "steuer.pdf",
		Title:        "Steuer",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "folders-ui-favorite",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name: "Steuer Favorit",
		Tags: []string{"steuer"},
	}); err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/folders", nil)
	rec := httptest.NewRecorder()

	server.handleFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, ">"+searchFavoritesFolderName+"<") {
		t.Fatalf("folders page renders technical search favorites name: %s", body)
	}
	if !strings.Contains(body, ">Suchfavoriten<") || !strings.Contains(body, `1 Suchfavorit`) || !strings.Contains(body, `folder-icon-search-favorites`) || !strings.Contains(body, "/folders?tags=00&#43;Suchfavoriten") {
		t.Fatalf("folders page missing search favorites tile: %s", body)
	}
	if strings.Contains(body, `folder-tile-search-favorites folder-tile-redundant`) {
		t.Fatalf("search favorites tile was marked redundant: %s", body)
	}
}

func TestFoldersPageMarksRedundantRootFolders(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	for i, doc := range []document.Document{
		{OriginalName: "alle-teil.pdf", StoredPath: "alle-teil.pdf", Title: "Alle Teil", Tags: []string{"alle", "teil"}, MIMEType: "application/pdf", SizeBytes: 1},
		{OriginalName: "alle.pdf", StoredPath: "alle.pdf", Title: "Alle", Tags: []string{"alle"}, MIMEType: "application/pdf", SizeBytes: 1},
	} {
		doc.SHA256 = fmt.Sprintf("folders-ui-redundant-root-%d", i)
		if _, err := repo.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/folders", nil)
	rec := httptest.NewRecorder()

	server.handleFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="folder-tile folder-tile-redundant" href="/folders?tags=alle"`) {
		t.Fatalf("root folder with full document count was not marked redundant: %s", body)
	}
	if strings.Contains(body, `class="folder-tile folder-tile-redundant" href="/folders?tags=teil"`) {
		t.Fatalf("root folder with smaller document count was marked redundant: %s", body)
	}
}

func TestFoldersPageFiltersRootTagsByConfiguredMinimum(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	for i, doc := range []document.Document{
		{OriginalName: "haeufig-selten.pdf", StoredPath: "haeufig-selten.pdf", Title: "Haeufig Selten", Tags: []string{"haeufig", "selten"}, MIMEType: "application/pdf", SizeBytes: 1},
		{OriginalName: "haeufig.pdf", StoredPath: "haeufig.pdf", Title: "Haeufig", Tags: []string{"haeufig"}, MIMEType: "application/pdf", SizeBytes: 1},
	} {
		doc.SHA256 = fmt.Sprintf("folders-ui-min-root-%d", i)
		if _, err := repo.CreateDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SaveSetting(ctx, folderTagMinDocumentsSettingKey, "2"); err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/folders", nil)
	rec := httptest.NewRecorder()
	server.handleFolders(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("root status = %d body = %s", rec.Code, rec.Body.String())
	}
	rootBody := rec.Body.String()
	if !strings.Contains(rootBody, `href="/folders?tags=haeufig"`) {
		t.Fatalf("root folder with enough documents missing: %s", rootBody)
	}
	if strings.Contains(rootBody, `href="/folders?tags=selten"`) {
		t.Fatalf("root folder below minimum rendered: %s", rootBody)
	}

	req = httptest.NewRequest(http.MethodGet, "/folders?tags=haeufig", nil)
	rec = httptest.NewRecorder()
	server.handleFolders(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("child status = %d body = %s", rec.Code, rec.Body.String())
	}
	childBody := rec.Body.String()
	if !strings.Contains(childBody, `href="/folders?tags=haeufig&amp;tags=selten"`) {
		t.Fatalf("child folder below root minimum should stay visible below first level: %s", childBody)
	}
}

func TestFoldersPageRendersConfiguredCustomFieldValueFolders(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", false, document.CustomFieldValueFolderAlways); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Projekt", false, 5); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var kundeID, projektID int64
	for _, field := range fields {
		switch field.Label {
		case "Kunde":
			kundeID = field.ID
		case "Projekt":
			projektID = field.ID
		}
	}
	for i, item := range []struct {
		name   string
		tags   []string
		values map[int64]string
	}{
		{"acme.pdf", []string{"steuer"}, map[int64]string{kundeID: "ACME", projektID: "Alpha"}},
		{"beta.pdf", []string{"steuer"}, map[int64]string{kundeID: "Beta", projektID: "Alpha"}},
	} {
		id, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: item.name,
			StoredPath:   item.name,
			Title:        item.name,
			Tags:         item.tags,
			MIMEType:     "application/pdf",
			SizeBytes:    1,
			SHA256:       fmt.Sprintf("folders-ui-field-value-%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMetadata(ctx, id, item.name, "", nil, item.tags, item.values); err != nil {
			t.Fatal(err)
		}
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/folders?tags=steuer", nil)
	rec := httptest.NewRecorder()

	server.handleFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, ">ACME<") || !strings.Contains(body, ">Beta<") || !strings.Contains(body, `folder-tile-field-value`) || !strings.Contains(body, `<span class="folder-meta">Kunde</span>`) {
		t.Fatalf("folders page missing custom field value folders: %s", body)
	}
	if strings.Contains(body, ">Kunde: ACME<") || strings.Contains(body, ">Kunde: Beta<") {
		t.Fatalf("field labels are rendered in the main folder name: %s", body)
	}
	if strings.Contains(body, "Projekt: Alpha") || strings.Contains(body, ">Alpha<") {
		t.Fatalf("thresholded value folder leaked: %s", body)
	}
	if !strings.Contains(body, "path=tag%3Asteuer") || !strings.Contains(body, fmt.Sprintf("path=field%%3A%d%%3AACME", kundeID)) {
		t.Fatalf("custom value folder link missing typed path: %s", body)
	}
}

func TestFoldersPageMarksRedundantChildFolders(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", false, document.CustomFieldValueFolderAlways); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kundeID := fields[0].ID
	for i, item := range []struct {
		name string
		tags []string
	}{
		{"steuer-privat.pdf", []string{"steuer", "privat"}},
		{"steuer-bank.pdf", []string{"steuer", "bank"}},
	} {
		id, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: item.name,
			StoredPath:   item.name,
			Title:        item.name,
			Tags:         item.tags,
			MIMEType:     "application/pdf",
			SizeBytes:    1,
			SHA256:       fmt.Sprintf("folders-ui-redundant-child-%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMetadata(ctx, id, item.name, "", nil, item.tags, map[int64]string{kundeID: "ACME"}); err != nil {
			t.Fatal(err)
		}
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/folders?tags=steuer", nil)
	rec := httptest.NewRecorder()

	server.handleFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="folder-tile folder-tile-field-value folder-tile-has-meta folder-tile-redundant"`) || !strings.Contains(body, `>ACME<`) {
		t.Fatalf("child field value folder with full document count was not marked redundant: %s", body)
	}
	if strings.Contains(body, `class="folder-tile folder-tile-redundant" href="/folders?tags=steuer&amp;tags=bank"`) ||
		strings.Contains(body, `class="folder-tile folder-tile-redundant" href="/folders?tags=steuer&amp;tags=privat"`) {
		t.Fatalf("child tag folder with smaller document count was marked redundant: %s", body)
	}
}

func TestFoldersPageSearchFavoriteSelectionSkipsFolderEmptyState(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer.pdf",
		StoredPath:   "steuer.pdf",
		Title:        "Steuer",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "folders-ui-favorite-selected",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name: "Steuer Favorit",
		Tags: []string{"steuer"},
	}); err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/folders?tags="+url.QueryEscape(searchFavoritesFolderName)+"&tags="+url.QueryEscape("Steuer Favorit"), nil)
	rec := httptest.NewRecorder()

	server.handleFolders(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "Keine weiteren Ordner") || strings.Contains(body, "Für diese Tag-Auswahl") || strings.Contains(body, `class="folder-empty"`) {
		t.Fatalf("selected search favorite renders misleading folder empty state: %s", body)
	}
	if !strings.Contains(body, ">Dokumente<") || !strings.Contains(body, "steuer.pdf") {
		t.Fatalf("selected search favorite missing document list: %s", body)
	}
}

func TestFoldersPageShowsDateFilterOnlyBelowRoot(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	docDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer-2026.pdf",
		StoredPath:   "steuer-2026.pdf",
		Title:        "Steuer 2026",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    1,
		SHA256:       "folders-ui-date-filter",
		DocumentDate: &docDate,
	}); err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/folders", nil)
	rec := httptest.NewRecorder()
	server.handleFolders(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("root status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "folder-date-filter") {
		t.Fatalf("root folders page renders date filter: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/folders?tags=steuer", nil)
	rec = httptest.NewRecorder()
	server.handleFolders(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tag status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "folder-date-filter") || !strings.Contains(body, ">2026<") {
		t.Fatalf("tag folder page missing date filter: %s", body)
	}
}

func TestTrashMobileCSSAllowsNarrowActionsAndWrapping(t *testing.T) {
	var body strings.Builder
	for _, name := range []string{
		"static/app.css",
		"static/app-statistics.css",
		"static/app-documents.css",
		"static/app-management.css",
		"static/app-overlays.css",
		"static/app-responsive.css",
	} {
		css, err := webFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		body.Write(css)
	}
	for _, want := range []string{
		`.trash-table .action-links form`,
		`grid-template-columns: repeat(auto-fit, minmax(min(150px, 100%), 1fr));`,
		`overflow-wrap: anywhere;`,
		`white-space: normal;`,
	} {
		if !strings.Contains(body.String(), want) {
			t.Fatalf("mobile trash CSS missing %q", want)
		}
	}
}

func newWebDAVTestRepoAndStore(t *testing.T, ctx context.Context) (*repository.Repository, *storage.Store) {
	t.Helper()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		repo.Close()
		t.Fatal(err)
	}
	return repo, store
}

func writeWebDAVStoredFile(t *testing.T, store *storage.Store, storedPath string, content []byte) string {
	t.Helper()
	target := filepath.Join(store.Root(), filepath.FromSlash(storedPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, content, 0o640); err != nil {
		t.Fatal(err)
	}
	return storedPath
}
