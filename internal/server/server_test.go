package server

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"bearstack"
	"bearstack/internal/config"
	"bearstack/internal/document"
	"bearstack/internal/photos"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

func authenticatedTestRequest(server *Server, req *http.Request, username string) *http.Request {
	if server == nil || !server.authEnabled() {
		return req
	}
	snapshot := server.authSnapshot()
	if snapshot == nil {
		return req
	}
	credential := snapshot.byUsername[username]
	if credential == nil {
		return req
	}
	return withAuthPrincipal(req, credential.principal())
}

func TestTesseractLanguageAllowsOnlyConfiguredLanguages(t *testing.T) {
	code, label, err := tesseractLanguage("de")
	if err != nil {
		t.Fatal(err)
	}
	if code != "deu" || label != "de" {
		t.Fatalf("de = %q %q", code, label)
	}

	code, label, err = tesseractLanguage("eng")
	if err != nil {
		t.Fatal(err)
	}
	if code != "eng" || label != "eng" {
		t.Fatalf("eng = %q %q", code, label)
	}

	if _, _, err := tesseractLanguage("fra"); err == nil {
		t.Fatal("fra should not be accepted")
	}
}

func TestTemplatesParse(t *testing.T) {
	if _, err := parseTemplates(); err != nil {
		t.Fatal(err)
	}
}

func TestNewPersistsAuthSessionKey(t *testing.T) {
	dataDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		DataDir: dataDir,
		Auth: config.AuthConfig{
			Username: "admin",
			Password: "secret",
		},
	}
	server1, err := New(cfg, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	session, err := server1.signAuthSession(authSessionPayloadForCredential(
		server1.authSnapshot().byUsername["admin"], time.Now().Add(time.Hour),
	))
	if err != nil {
		t.Fatal(err)
	}

	server2, err := New(cfg, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(server1.authKey, server2.authKey) {
		t.Fatal("auth session key changed across server restart")
	}
	payload, ok := server2.verifyAuthSession(session)
	if !ok {
		t.Fatal("session cookie did not verify after restart")
	}
	if payload.User != "admin" {
		t.Fatalf("session user = %q", payload.User)
	}
	keyInfo, err := os.Stat(filepath.Join(dataDir, authSessionKeyFileName))
	if err != nil {
		t.Fatal(err)
	}
	if got := keyInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("auth session key mode = %o, want 600", got)
	}
}

func TestTemplateAssetsAreScopedByPage(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		tpl     string
		data    PageData
		want    []string
		notWant []string
	}{
		{
			name: "documents",
			tpl:  "index.html",
			data: PageData{Title: "Dokumente", Active: "documents", Pagination: PaginationData{DocumentList: true}},
			want: []string{
				"/static/app-documents.css",
				"/static/app-management.css",
				"/static/app-documents.js",
				"/static/app-preview.js",
				"/static/app-upload.js",
			},
			notWant: []string{"/static/app-photos.js", "/static/app-charts.js"},
		},
		{
			name:    "statistics",
			tpl:     "statistics.html",
			data:    PageData{Title: "Statistik", Active: "statistics"},
			want:    []string{"/static/app-statistics.css", "/static/app-charts.js"},
			notWant: []string{"/static/app-photos.js", "/static/app-documents.js", "/static/app-upload.js"},
		},
		{
			name:    "photos",
			tpl:     "photos.html",
			data:    PageData{Title: "Fotos", Active: "photos", PhotoPage: true},
			want:    []string{"/static/app-photos-map.js", "/static/app-photos-thumbnails.js", "/static/app-photos.js", "/static/app-photos-frame.js"},
			notWant: []string{"/static/app-charts.js", "/static/app-documents.js", "/static/app-preview.js", "/static/app-upload.js"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := templates.ExecuteTemplate(&out, tc.tpl, tc.data); err != nil {
				t.Fatal(err)
			}
			body := out.String()
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Fatalf("missing asset %s in %s", want, body)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(body, notWant) {
					t.Fatalf("unexpected asset %s in %s", notWant, body)
				}
			}
		})
	}
}

func TestSystemMenuSeparatorsOnlyBetweenVisibleGroups(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name           string
		auth           AuthPermissions
		wantSeparators int
		want           []string
		notWant        []string
	}{
		{
			name:           "anonymous",
			auth:           AuthPermissions{},
			wantSeparators: 0,
			notWant:        []string{`class="system-menu"`, `href="/help"`, `href="/api"`},
		},
		{
			name:           "api uploader",
			auth:           authPermissionsFromCapabilities(authCapDocumentsUpload, authPrincipal{Username: "uploader"}),
			wantSeparators: 0,
			want:           []string{`href="/help"`, `href="/api"`, `action="/logout"`, `>Logout<`},
		},
		{
			name:           "photos reader",
			auth:           authPermissionsFromCapabilities(authCapPhotosRead, authPrincipal{Username: "photos"}),
			wantSeparators: 1,
			want:           []string{`href="/tags"`, `href="/help"`, `href="/api"`, `action="/logout"`},
		},
		{
			name:           "audit only",
			auth:           authPermissionsFromCapabilities(authCapSystemAudit, authPrincipal{Username: "auditor"}),
			wantSeparators: 1,
			want:           []string{`href="/log"`, `href="/help"`, `action="/logout"`},
			notWant:        []string{`href="/api"`},
		},
		{
			name:           "documents reader",
			auth:           authPermissionsFromCapabilities(authCapDocumentsRead, authPrincipal{Username: "reader"}),
			wantSeparators: 2,
			want:           []string{`href="/tags"`, `href="/duplicates"`, `href="/statistics"`, `href="/help"`, `href="/api"`, `action="/logout"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := templates.ExecuteTemplate(&out, "help.html", PageData{Title: "Hilfe", Active: "help", Auth: tc.auth}); err != nil {
				t.Fatal(err)
			}
			body := out.String()
			if got := strings.Count(body, `system-menu-separator`); got != tc.wantSeparators {
				t.Fatalf("separator count = %d, want %d in\n%s", got, tc.wantSeparators, body)
			}
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Fatalf("missing %q in\n%s", want, body)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(body, notWant) {
					t.Fatalf("unexpected %q in\n%s", notWant, body)
				}
			}
		})
	}
}

func TestRenderIncludesAppVersionFooter(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()

	server.render(rec, req, "help.html", PageData{Title: "Hilfe", Active: "help"})

	body := rec.Body.String()
	if !strings.Contains(body, `class="app-footer"`) {
		t.Fatalf("rendered page missing app footer in\n%s", body)
	}
	want := "BearStack v" + bearstack.Version()
	if !strings.Contains(body, want) {
		t.Fatalf("rendered page missing version %q in\n%s", want, body)
	}
}

func TestPhotoFrameOmitsAppVersionFooter(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/photos/frame", nil)
	rec := httptest.NewRecorder()

	server.render(rec, req, "photo_frame.html", PageData{
		Title:      "Fotoframe",
		Active:     "photos",
		PhotoFrame: true,
		PhotoPage:  true,
	})

	body := rec.Body.String()
	if strings.Contains(body, `class="app-footer"`) {
		t.Fatalf("photo frame rendered app footer in\n%s", body)
	}
	for _, want := range []string{`data-photo-frame-bar`, `data-photo-frame-bar-close`, `Fotoframe-Leiste ausblenden`} {
		if !strings.Contains(body, want) {
			t.Fatalf("photo frame missing %q in\n%s", want, body)
		}
	}
}

func TestPhotoPageAssetsCanSkipTagScript(t *testing.T) {
	if photoPageAssets(false).Tags {
		t.Fatal("read-only photo assets should not include tag assets")
	}
	if !photoPageAssets(true).Tags {
		t.Fatal("editable photo assets should include tag assets")
	}
}

func TestPhotoFrameMediaAPIResponseUsesCleanTitle(t *testing.T) {
	media := []PhotoMediaView{
		{Media: photos.Media{Name: "2026-05-18_Sommer-Urlaub.jpg", Path: "2026-05-18_Sommer-Urlaub.jpg", Type: photos.MediaTypeImage}},
	}
	frameResponses := photoFrameMediaAPIResponsesFrom(media)
	if len(frameResponses) != 1 {
		t.Fatalf("responses length = %d, want 1", len(frameResponses))
	}
	if frameResponses[0].Title != "Sommer Urlaub" {
		t.Fatalf("frame title = %q, want Sommer Urlaub", frameResponses[0].Title)
	}

	regular := photoMediaAPIResponseFrom(media[0])
	if regular.Title != "2026-05-18_Sommer-Urlaub.jpg" {
		t.Fatalf("regular title = %q, want raw filename", regular.Title)
	}
}

func TestTemplatesHideReadOnlyActions(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	data := PageData{
		Title:           "Dokumente",
		Active:          "documents",
		Auth:            authPermissionsFromCapabilities(authCapDocumentsRead, authPrincipal{Username: "reader"}),
		Documents:       []document.Document{{ID: 1, OriginalName: "rechnung.pdf", Title: "Rechnung", MIMEType: "application/pdf"}},
		DocumentColumns: []DocumentColumn{{Key: "name", Label: "Name"}, {Key: "actions", Label: "Aktionen"}},
		ColumnOptions:   []DocumentColumn{{Key: "name", Label: "Name"}},
		VisibleColumns:  map[string]bool{"name": true, "actions": true},
		Pagination:      PaginationData{DocumentList: true},
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "index.html", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, notWant := range []string{`data-upload-form`, `data-columns-open`, `/settings/page-size`} {
		if strings.Contains(body, notWant) {
			t.Fatalf("read-only document template contains %q in\n%s", notWant, body)
		}
	}

	out.Reset()
	data.Auth = authPermissionsFromCapabilities(authCapDocumentsRead|authCapDocumentsUpload|authCapSystemManage, authPrincipal{Username: "admin"})
	if err := templates.ExecuteTemplate(&out, "index.html", data); err != nil {
		t.Fatal(err)
	}
	body = out.String()
	for _, want := range []string{`data-upload-form`, `data-columns-open`, `/settings/page-size`} {
		if !strings.Contains(body, want) {
			t.Fatalf("admin document template missing %q in\n%s", want, body)
		}
	}
}

func TestDocumentListTemplateRendersShareActionBetweenDownloadAndDetails(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	data := PageData{
		Title:  "Dokumente",
		Active: "documents",
		Auth:   authPermissionsFromCapabilities(authCapDocumentsRead, authPrincipal{Username: "reader"}),
		Documents: []document.Document{{
			ID:           1,
			OriginalName: "rechnung.pdf",
			Title:        "Rechnung",
			MIMEType:     "application/pdf",
		}},
		DocumentColumns: []DocumentColumn{{Key: "actions", Label: "Aktionen"}},
		VisibleColumns:  map[string]bool{"actions": true},
		Pagination:      PaginationData{DocumentList: true},
		ReturnURL:       "/",
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "index.html", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	downloadIndex := strings.Index(body, `href="/documents/1/download"`)
	shareIndex := strings.Index(body, `data-share-url="/documents/1/view"`)
	detailIndex := strings.Index(body, `href="/documents/1?return=%2F"`)
	if downloadIndex < 0 || shareIndex < 0 || detailIndex < 0 {
		t.Fatalf("document actions missing expected links in\n%s", body)
	}
	if !(downloadIndex < shareIndex && shareIndex < detailIndex) {
		t.Fatalf("share action order incorrect: download=%d share=%d detail=%d", downloadIndex, shareIndex, detailIndex)
	}
}

func TestPhotosTemplateHidesEditToggleForReadOnly(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	data := PageData{
		Title:              "Fotos",
		Active:             "photos",
		PhotoModuleEnabled: true,
		PhotoPage:          true,
		Auth:               authPermissionsFromCapabilities(authCapPhotosRead, authPrincipal{Username: "photos"}),
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "photos.html", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if strings.Contains(body, `data-photo-mode-toggle`) || strings.Contains(body, `data-update-url="/photos/tags`) {
		t.Fatalf("read-only photos template exposes edit controls in\n%s", body)
	}
}

func TestHandleDocumentViewForcesReadOnlyEvenWithEditPermissions(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "main.pdf",
		StoredPath:   "2026/05/main.pdf",
		Title:        "Main",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "detail-readonly-view",
	})
	if err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: templates,
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/documents/%d/view", docID), nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handleDocumentView(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, fmt.Sprintf(`data-share-url="/documents/%d/view"`, docID)) {
		t.Fatalf("share action missing in\n%s", body)
	}
	if !strings.Contains(body, fmt.Sprintf(`href="/documents/%d/download"`, docID)) {
		t.Fatalf("download action missing in\n%s", body)
	}
	for _, notWant := range []string{
		fmt.Sprintf(`action="/documents/%d/metadata"`, docID),
		fmt.Sprintf(`action="/documents/%d/ocr/de"`, docID),
		fmt.Sprintf(`action="/documents/%d/ocr/eng"`, docID),
		fmt.Sprintf(`action="/documents/%d/delete"`, docID),
		`data-metadata-form`,
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf("read-only view still contains %q in\n%s", notWant, body)
		}
	}
}

func TestPreviewScriptTreatsChartInitializersAsOptional(t *testing.T) {
	contents, err := webFS.ReadFile("static/app-preview.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(contents)
	for _, want := range []string{
		`typeof initializeDocumentDateCharts === "function"`,
		`typeof initializeTagTimelines === "function"`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("app-preview.js must guard optional chart initializer %q", want)
		}
	}
}

func TestStatisticsChartScriptInitializesCharts(t *testing.T) {
	contents, err := webFS.ReadFile("static/app-charts.js")
	if err != nil {
		t.Fatal(err)
	}
	script := string(contents)
	for _, want := range []string{
		`document.addEventListener("DOMContentLoaded"`,
		`initializeDocumentDateCharts();`,
		`initializeTagTimelines();`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("app-charts.js must initialize statistics charts with %q", want)
		}
	}
}

func TestClosedDialogsAreHiddenByGlobalCSS(t *testing.T) {
	contents, err := webFS.ReadFile("static/app-overlays.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "dialog:not([open])") {
		t.Fatal("global CSS must hide closed dialog elements")
	}
}

func TestStatisticsTemplateRendersPhotoIndexerTelemetry(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	data := PageData{
		Title:              "Statistik",
		PhotoModuleEnabled: true,
		PhotoSettings: PhotoSettings{
			FolderThumbnailSize:  320,
			ThumbnailSize:        420,
			PreviewSize:          1920,
			LargePreviewSize:     3840,
			ThumbnailConcurrency: 2,
		},
		PhotoStatistics: photos.Statistics{
			IndexAvailable:       true,
			Index:                photos.IndexStats{Media: 5, Folders: 1, Blogs: 1},
			MediaBytes:           123456,
			AverageMediaBytes:    30864,
			ImageCount:           3,
			VideoCount:           1,
			AudioCount:           1,
			GPSMediaCount:        2,
			GPSCoveragePercent:   50,
			MediaTagAssignments:  3,
			FolderTagAssignments: 1,
			BlogTagAssignments:   1,
			PhotoTagCount:        4,
			ThumbnailCacheFiles:  2,
			ThumbnailCacheBytes:  2048,
			ThumbnailBackends:    []photos.ThumbnailBackendStatus{{Name: "vipsthumbnail", Purpose: "Bilder, primär", Available: true, Version: "vipsthumbnail 8.14", Path: "/usr/bin/vipsthumbnail"}},
			IndexDatabasePath:    "/tmp/photos.db",
			RootPath:             "/photos",
			CachePath:            "/photos/.bearstack-cache",
		},
		PhotoIndexTelemetry: photos.IndexTelemetry{
			StartedAt:      started,
			FinishedAt:     started.Add(2 * time.Second),
			Duration:       2 * time.Second,
			ScannedFolders: 2,
			SkippedFolders: 3,
			Files:          4,
			FilesPerSecond: 2,
			DBWrites:       5,
			Stats:          photos.IndexStats{Media: 4, Folders: 1},
			LastErrors:     []string{"album: testfehler"},
		},
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "statistics.html", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{"Fotos", "Foto-Indexer", "3 Bilder", "1 Videos", "vipsthumbnail", "2 gescannt", "3 übersprungen", "2.0 Datei(en)/s", "album: testfehler"} {
		if !strings.Contains(body, want) {
			t.Fatalf("statistics template missing %q in\n%s", want, body)
		}
	}
}

func TestCachedDocumentStatisticsInvalidatesWithDocumentCountCache(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{repo: repo}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "one.pdf",
		StoredPath:    "2026/05/one.pdf",
		Title:         "One",
		MIMEType:      "application/pdf",
		SizeBytes:     1,
		SHA256:        "stats-cache-one",
		SearchVersion: document.CurrentSearchVersion,
	}); err != nil {
		t.Fatal(err)
	}
	stats, err := server.cachedDocumentStatistics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ActiveDocuments != 1 {
		t.Fatalf("active documents = %d", stats.ActiveDocuments)
	}

	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "two.pdf",
		StoredPath:    "2026/05/two.pdf",
		Title:         "Two",
		MIMEType:      "application/pdf",
		SizeBytes:     1,
		SHA256:        "stats-cache-two",
		SearchVersion: document.CurrentSearchVersion,
	}); err != nil {
		t.Fatal(err)
	}
	cached, err := server.cachedDocumentStatistics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cached.ActiveDocuments != 1 {
		t.Fatalf("cached active documents = %d", cached.ActiveDocuments)
	}

	server.invalidateDocumentCountCache()
	refreshed, err := server.cachedDocumentStatistics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ActiveDocuments != 2 {
		t.Fatalf("refreshed active documents = %d", refreshed.ActiveDocuments)
	}
}

func TestCachedPhotoStatisticsInvalidatesPhotoCache(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	lib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()

	thumbnailDir := filepath.Join(cacheDir, "thumbnails")
	if err := os.MkdirAll(thumbnailDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(thumbnailDir, "one.webp"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{photos: lib}
	stats, err := server.cachedPhotoStatistics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.ThumbnailCacheFiles != 1 {
		t.Fatalf("thumbnail cache files = %d", stats.ThumbnailCacheFiles)
	}

	if err := os.WriteFile(filepath.Join(thumbnailDir, "two.webp"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	cached, err := server.cachedPhotoStatistics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cached.ThumbnailCacheFiles != 1 {
		t.Fatalf("cached thumbnail cache files = %d", cached.ThumbnailCacheFiles)
	}

	server.invalidatePhotoStatisticsCache()
	refreshed, err := server.cachedPhotoStatistics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ThumbnailCacheFiles != 2 {
		t.Fatalf("refreshed thumbnail cache files = %d", refreshed.ThumbnailCacheFiles)
	}
}

func TestStaticAssetURLIncludesContentVersion(t *testing.T) {
	versions, err := staticAssetVersions()
	if err != nil {
		t.Fatal(err)
	}
	cssURL := string(staticAssetURL("/static/app.css", versions))
	if !strings.HasPrefix(cssURL, "/static/app.css?v=") {
		t.Fatalf("css asset URL = %q", cssURL)
	}
	if got := staticAssetURL("/static/missing.css", versions); got != "/static/missing.css" {
		t.Fatalf("missing asset URL = %q", got)
	}
}

func TestCacheStaticAssetsUsesImmutableForVersionedRequests(t *testing.T) {
	handler := cacheStaticAssets(fstest.MapFS{
		"app.css": &fstest.MapFile{Data: []byte("body { color: #111; }\n")},
	})

	req := httptest.NewRequest(http.MethodGet, "/app.css?v=abc123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("versioned cache-control = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/app.css", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("unversioned cache-control = %q", got)
	}
}

func TestCacheStaticAssetsServesPrecompressedGzip(t *testing.T) {
	const body = "body { color: #111; }\n"
	handler := cacheStaticAssets(fstest.MapFS{
		"app.css": &fstest.MapFile{Data: []byte(body)},
	})

	req := httptest.NewRequest(http.MethodGet, "/app.css?v=abc123", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("content-type = %q", got)
	}
	if got := rec.Header().Get("Vary"); !strings.Contains(strings.ToLower(got), "accept-encoding") {
		t.Fatalf("vary = %q", got)
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	decompressed, err := io.ReadAll(zr)
	if closeErr := zr.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if string(decompressed) != body {
		t.Fatalf("decompressed = %q", string(decompressed))
	}

	req = httptest.NewRequest(http.MethodHead, "/app.css?v=abc123", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("HEAD status/body = %d/%d", rec.Code, rec.Body.Len())
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("HEAD content-encoding = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/app.css?v=abc123", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-3")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("range content-encoding = %q", got)
	}
}

func TestRenderErrorSetsHTMLContentType(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	server.renderError(rec, req, http.StatusBadRequest, fmt.Errorf("kaputt"))

	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
}

func TestRenderErrorHidesInternalDetails(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/broken", nil)
	rec := httptest.NewRecorder()

	server.renderError(rec, req, http.StatusInternalServerError, fmt.Errorf("open /private/tmp/secret.db: permission denied"))

	body := rec.Body.String()
	if strings.Contains(body, "/private/tmp/secret.db") || strings.Contains(body, "permission denied") {
		t.Fatalf("internal detail leaked in body: %s", body)
	}
	if !strings.Contains(body, "Interner Serverfehler") {
		t.Fatalf("generic error missing: %s", body)
	}
}

func TestRenderJSONErrorHidesInternalDetails(t *testing.T) {
	server := &Server{log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	rec := httptest.NewRecorder()

	server.renderJSONError(rec, http.StatusInternalServerError, "open /private/tmp/secret.db: permission denied")

	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Result().Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error != "Interner Serverfehler" {
		t.Fatalf("error = %q", payload.Error)
	}
}

func TestDesktopPreviewModeSettingDefaultsAndNormalizes(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{repo: repo}
	mode, err := server.desktopPreviewMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mode != desktopPreviewModeModal {
		t.Fatalf("default mode = %q", mode)
	}
	if normalizeDesktopPreviewMode("inline") != desktopPreviewModeInline || normalizeDesktopPreviewMode("quatsch") != desktopPreviewModeModal {
		t.Fatal("preview mode normalization failed")
	}

	if err := repo.SaveSetting(ctx, desktopPreviewModeSettingKey, "invalid"); err != nil {
		t.Fatal(err)
	}
	mode, err = server.desktopPreviewMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mode != desktopPreviewModeModal {
		t.Fatalf("invalid mode = %q", mode)
	}
}

func TestTagDisplayModeSettingDefaultsAndNormalizes(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{repo: repo}
	mode, err := server.tagDisplayMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mode != tagDisplayModeLower {
		t.Fatalf("default mode = %q", mode)
	}
	if normalizeTagDisplayMode("strtoupper") != tagDisplayModeUpper ||
		normalizeTagDisplayMode("ucfirst") != tagDisplayModeFirst ||
		normalizeTagDisplayMode("quatsch") != tagDisplayModeLower {
		t.Fatal("tag display mode normalization failed")
	}
	if formatTag(tagDisplayModeUpper, "steuer") != "STEUER" ||
		formatTag(tagDisplayModeFirst, "steuer") != "Steuer" ||
		formatTag(tagDisplayModeLower, "Steuer") != "steuer" {
		t.Fatal("tag display formatting failed")
	}

	if err := repo.SaveSetting(ctx, tagDisplayModeSettingKey, "invalid"); err != nil {
		t.Fatal(err)
	}
	mode, err = server.tagDisplayMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mode != tagDisplayModeLower {
		t.Fatalf("invalid mode = %q", mode)
	}
}

func TestThemeModeSettingDefaultsAndNormalizes(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{repo: repo}
	mode, err := server.themeMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mode != themeModeDefault {
		t.Fatalf("default theme mode = %q", mode)
	}
	if normalizeThemeMode("design2") != themeModeDesign2 ||
		normalizeThemeMode(" DESIGN2 ") != themeModeDesign2 ||
		normalizeThemeMode("quatsch") != themeModeDefault {
		t.Fatal("theme mode normalization failed")
	}

	if err := repo.SaveSetting(ctx, themeModeSettingKey, "invalid"); err != nil {
		t.Fatal(err)
	}
	mode, err = server.themeMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mode != themeModeDefault {
		t.Fatalf("invalid theme mode = %q", mode)
	}

	if err := repo.SaveSetting(ctx, themeModeSettingKey, themeModeDesign2); err != nil {
		t.Fatal(err)
	}
	mode, err = server.themeMode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if mode != themeModeDesign2 {
		t.Fatalf("stored theme mode = %q", mode)
	}
}

func TestDocumentCloudEnabledSettingDefaultsAndNormalizes(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{repo: repo}
	enabled, err := server.documentCloudEnabled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("document cloud should be disabled by default")
	}
	if err := repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "yes"); err != nil {
		t.Fatal(err)
	}
	enabled, err = server.documentCloudEnabled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("yes should enable document cloud")
	}
	if err := repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "invalid"); err != nil {
		t.Fatal(err)
	}
	enabled, err = server.documentCloudEnabled(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("invalid document cloud setting should fall back to disabled")
	}
}

func TestTrashRetentionDaysSettingDefaultsAndNormalizes(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{repo: repo}
	days, err := server.trashRetentionDays(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if days != 0 {
		t.Fatalf("default retention days = %d", days)
	}
	if normalizeTrashRetentionDays("30") != 30 ||
		normalizeTrashRetentionDays("60") != 60 ||
		normalizeTrashRetentionDays("90") != 90 ||
		normalizeTrashRetentionDays("15") != 0 ||
		normalizeTrashRetentionDays("quatsch") != 0 {
		t.Fatal("trash retention normalization failed")
	}

	if err := repo.SaveSetting(ctx, trashRetentionDaysSettingKey, "60"); err != nil {
		t.Fatal(err)
	}
	days, err = server.trashRetentionDays(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if days != 60 {
		t.Fatalf("retention days = %d", days)
	}
}

func TestAppNameSettingDefaultsAndNormalizes(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	name, err := appName(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if name != defaultAppName {
		t.Fatalf("default app name = %q", name)
	}
	if normalizeAppName("  Meine   Ablage  ") != "Meine Ablage" || normalizeAppName("  ") != defaultAppName {
		t.Fatal("app name normalization failed")
	}

	if err := repo.SaveSetting(ctx, appNameSettingKey, "Archiv"); err != nil {
		t.Fatal(err)
	}
	name, err = appName(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Archiv" {
		t.Fatalf("app name = %q", name)
	}
}

func TestServerAppNameCache(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveSetting(ctx, appNameSettingKey, "Archiv"); err != nil {
		t.Fatal(err)
	}

	server := &Server{repo: repo}
	name, err := server.appName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Archiv" {
		t.Fatalf("initial cached app name = %q", name)
	}

	if err := repo.SaveSetting(ctx, appNameSettingKey, "Externe Änderung"); err != nil {
		t.Fatal(err)
	}
	name, err = server.appName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Archiv" {
		t.Fatalf("cached app name after external write = %q", name)
	}

	server.cacheAppName("Aktualisiert")
	name, err = server.appName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Aktualisiert" {
		t.Fatalf("updated cached app name = %q", name)
	}
}

func TestServerRenderSettingsCache(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveSetting(ctx, tagDisplayModeSettingKey, tagDisplayModeUpper); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, themeModeSettingKey, themeModeDesign2); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, homePageSettingKey, homePagePhotos); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "1"); err != nil {
		t.Fatal(err)
	}

	server := &Server{repo: repo}
	settings, err := server.renderSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.TagDisplayMode != tagDisplayModeUpper || settings.ThemeMode != themeModeDesign2 || settings.HomePage != homePagePhotos || !settings.DocumentCloudEnabled {
		t.Fatalf("initial render settings = %#v", settings)
	}

	if err := repo.SaveSetting(ctx, tagDisplayModeSettingKey, tagDisplayModeLower); err != nil {
		t.Fatal(err)
	}
	settings, err = server.renderSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.TagDisplayMode != tagDisplayModeUpper {
		t.Fatalf("cached tag display mode after external write = %q", settings.TagDisplayMode)
	}

	server.cacheRenderSettings(renderSettingsSnapshot{TagDisplayMode: tagDisplayModeFirst, ThemeMode: themeModeDefault, HomePage: homePageFolders})
	settings, err = server.renderSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.TagDisplayMode != tagDisplayModeFirst || settings.HomePage != homePageFolders {
		t.Fatalf("updated render settings = %#v", settings)
	}
}

func TestPhotoSettingsDefaultsSavesAndNormalizes(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{repo: repo}
	settings, err := server.photoSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings != defaultPhotoSettings() {
		t.Fatalf("default photo settings = %#v", settings)
	}

	form := url.Values{
		"photo_page_size":                   {"2000"},
		"folder_preview_count":              {"99"},
		"folder_thumbnail_size":             {"42"},
		"thumbnail_size":                    {"42"},
		"preview_size":                      {"5000"},
		"large_preview_size":                {"9999"},
		"slideshow_seconds":                 {"1"},
		"frame_seconds":                     {"999"},
		"photo_map_track_resolution_meters": {"42"},
		"index_worker_enabled":              {"1"},
		"index_worker_interval_minutes":     {"0"},
		"index_worker_delay_millis":         {"1"},
		"thumbnail_worker_enabled":          {"1"},
		"thumbnail_worker_interval_minutes": {"0"},
		"thumbnail_worker_batch_size":       {"2000"},
		"thumbnail_concurrency":             {"99"},
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/photos", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	parsed := photoSettingsFromRequest(req)
	want := PhotoSettings{
		PageSize:                       1000,
		FolderPreviewCount:             4,
		FolderThumbnailSize:            64,
		ThumbnailSize:                  64,
		PreviewSize:                    1920,
		LargePreviewSize:               3840,
		SlideshowSeconds:               2,
		FrameSeconds:                   300,
		PreloadAdjacent:                false,
		MapTrackResolutionMeters:       500,
		IndexWorkerEnabled:             true,
		IndexWorkerIntervalMinutes:     1,
		IndexWorkerDelayMillis:         50,
		ThumbnailWorkerEnabled:         true,
		ThumbnailWorkerIntervalMinutes: 1,
		ThumbnailWorkerBatchSize:       1000,
		ThumbnailConcurrency:           4,
	}
	if parsed != want {
		t.Fatalf("parsed photo settings = %#v", parsed)
	}
	if err := server.savePhotoSettings(ctx, parsed); err != nil {
		t.Fatal(err)
	}
	settings, err = server.photoSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings != want {
		t.Fatalf("saved photo settings = %#v", settings)
	}
}

func TestHandlePhotoSettingsRendersSavedValues(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	photoLib, err := photos.New(t.TempDir(), filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := server.savePhotoSettings(ctx, PhotoSettings{
		PageSize:                       36,
		FolderPreviewCount:             3,
		FolderThumbnailSize:            180,
		ThumbnailSize:                  640,
		PreviewSize:                    1600,
		LargePreviewSize:               3200,
		SlideshowSeconds:               7,
		FrameSeconds:                   12,
		PreloadAdjacent:                true,
		MapTrackResolutionMeters:       3000,
		IndexWorkerEnabled:             true,
		IndexWorkerIntervalMinutes:     45,
		IndexWorkerDelayMillis:         300,
		ThumbnailWorkerEnabled:         true,
		ThumbnailWorkerIntervalMinutes: 15,
		ThumbnailWorkerBatchSize:       25,
		ThumbnailConcurrency:           2,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/photos", nil)
	rec := httptest.NewRecorder()
	server.handlePhotoSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="/settings/photos">Fotos</a>`,
		`data-context-help-title="Bilder pro Seite"`,
		`data-context-help-title="Previewbilder pro Ordner"`,
		`data-context-help-title="Ordner-Previews"`,
		`data-context-help-title="Galerie-Thumbnails"`,
		`data-context-help-title="Große Vorschau"`,
		`data-context-help-title="HD-Vorschau"`,
		`name="photo_page_size" min="1" max="1000" value="36"`,
		`name="folder_preview_count" min="1" max="4" value="3"`,
		`name="folder_thumbnail_size" min="64" max="640" value="180"`,
		`name="thumbnail_size" min="64" max="1200" step="20" value="640"`,
		`name="preview_size" min="640" max="1920" step="80" value="1600"`,
		`name="large_preview_size" min="1920" max="3840" step="160" value="3200"`,
		`name="slideshow_seconds" min="2" max="60" value="7"`,
		`name="frame_seconds" min="3" max="300" value="12"`,
		`name="preload_adjacent" value="1" checked`,
		`name="photo_map_track_resolution_meters"`,
		`<option value="3000" selected>3 km</option>`,
		`name="index_worker_enabled" value="1" checked`,
		`name="index_worker_interval_minutes" min="1" max="10080" value="45"`,
		`name="index_worker_delay_millis" min="50" max="5000" step="50" value="300"`,
		`name="thumbnail_worker_enabled" value="1" checked`,
		`name="thumbnail_worker_interval_minutes" min="1" max="1440" value="15"`,
		`name="thumbnail_worker_batch_size" min="1" max="1000" value="25"`,
		`name="thumbnail_concurrency" min="1" max="4" value="2"`,
		`/settings/photos/index/run`,
		`/settings/photos/thumbnails/run`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestHandlePhotosSearchShowsSearchResultBreadcrumb(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/photos?path=album&q=demo", nil)
	rec := httptest.NewRecorder()
	server.handlePhotos(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<a class="folder-breadcrumb-link" href="/photos">Fotos</a>`,
		`<span class="folder-breadcrumb-current" aria-current="page">Suchergebnis</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, notWant := range []string{
		`<a class="folder-breadcrumb-link" href="/photos?path=album">album</a>`,
		`<span class="folder-breadcrumb-current" aria-current="page">album</span>`,
		`<input type="hidden" name="path" value="album">`,
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf("body should not contain %q for root search: %s", notWant, body)
		}
	}
}

func TestHandlePhotosSearchFindsAudioMediaByFilename(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".order_ascending_name.pg2conf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public-a.png"), []byte("not a real png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public-song.mp3"), []byte("not a real mp3"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "repo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
			{Username: "editor", Password: "secret", Role: "photos_editor"},
		}}},
		repo:      repo,
		photos:    photoLib,
		templates: templates,
		static:    http.NotFoundHandler(),
		authKey:   []byte("01234567890123456789012345678901"),
		apps:      testPhotoJobApps(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	handler := server.Handler()
	for _, target := range []string{"/photos?sort=ascending_name", "/photos?sort=ascending_name&q=public-song&type="} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.SetBasicAuth("editor", "secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", target, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-photo-title="public-song.mp3"`) {
			t.Fatalf("%s body missing audio result: %s", target, body)
		}
	}
}

func TestPhotoMapOnlyAvailableFromSecondFolderLevel(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album", "trip"), 0o750); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	render := func(target string) string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		server.handlePhotos(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", target, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	rootBody := render("/photos")
	if strings.Contains(rootBody, `href="/photos?view=map">Karte</a>`) {
		t.Fatalf("root page should not offer map action: %s", rootBody)
	}

	firstLevelBody := render("/photos?path=album")
	if strings.Contains(firstLevelBody, `href="/photos?path=album&amp;view=map">Karte</a>`) {
		t.Fatalf("first-level page should not offer map action: %s", firstLevelBody)
	}

	firstLevelMapBody := render("/photos?path=album&view=map")
	if strings.Contains(firstLevelMapBody, `class="photo-map-panel" data-photo-map`) {
		t.Fatalf("first-level direct map request should render gallery: %s", firstLevelMapBody)
	}
	if strings.Contains(firstLevelMapBody, `name="view" value="map"`) {
		t.Fatalf("first-level direct map request should not preserve map mode: %s", firstLevelMapBody)
	}

	secondLevelBody := render("/photos?path=album%2Ftrip")
	if !strings.Contains(secondLevelBody, `href="/photos?path=album%2Ftrip&amp;view=map">Karte</a>`) {
		t.Fatalf("second-level page should offer map action: %s", secondLevelBody)
	}

	secondLevelMapBody := render("/photos?path=album%2Ftrip&view=map")
	if !strings.Contains(secondLevelMapBody, `class="photo-map-panel" data-photo-map`) {
		t.Fatalf("second-level map request should render map: %s", secondLevelMapBody)
	}
	if !strings.Contains(secondLevelMapBody, `href="/photos?path=album%2Ftrip">Galerie</a>`) {
		t.Fatalf("second-level map request should offer gallery action: %s", secondLevelMapBody)
	}
}

func TestDecoratePhotoListingMarksCachedThumbnailReady(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()

	thumbPath := photoTestThumbnailPath(cacheDir, "album/ready.jpg", 420)
	folderThumbPath := photoTestThumbnailPath(cacheDir, "album/folder.jpg", 180)
	if err := os.MkdirAll(filepath.Dir(thumbPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(thumbPath, []byte("webp"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(folderThumbPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(folderThumbPath, []byte("webp"), 0o600); err != nil {
		t.Fatal(err)
	}

	listing := photos.Listing{
		Folders: []photos.Folder{
			{
				Name: "album",
				Path: "album",
				Previews: []photos.Media{
					{Path: "album/folder.jpg", Type: photos.MediaTypeImage},
				},
			},
		},
		Media: []photos.Media{
			{Path: "album/ready.jpg", Type: photos.MediaTypeImage},
			{Path: "album/missing.jpg", Type: photos.MediaTypeImage},
		},
	}
	view := newPhotoListingView(context.Background(), photoLib, listing, PhotoSettings{FolderThumbnailSize: 180, ThumbnailSize: 420, PreviewSize: 960})

	if !view.Media[0].ThumbReady {
		t.Fatal("cached thumbnail was not marked ready")
	}
	if view.Media[1].ThumbReady {
		t.Fatal("missing thumbnail was marked ready")
	}
	if !strings.Contains(view.Media[0].ThumbURL, "album%2Fready.jpg") {
		t.Fatalf("thumbnail url was not decorated: %#v", view.Media[0])
	}
	if !view.Folders[0].Previews[0].ThumbReady {
		t.Fatal("cached folder thumbnail was not marked ready")
	}
	if !strings.Contains(view.Folders[0].Previews[0].ThumbURL, "size=180") {
		t.Fatalf("folder thumbnail url used the wrong size: %#v", view.Folders[0].Previews[0])
	}
	if strings.Contains(view.Media[0].ThumbURL, "size=180") {
		t.Fatalf("gallery thumbnail url used folder size: %#v", view.Media[0])
	}
}

func TestPhotoFolderViewLabelsRecursiveMediaCounts(t *testing.T) {
	listing := photos.Listing{
		Folders: []photos.Folder{
			{Name: "album", Path: "album", DirectMediaCount: 1, MediaCount: 3, DirCount: 2},
			{Name: "single", Path: "single", DirectMediaCount: 1, MediaCount: 1},
		},
	}

	view := newPhotoListingView(context.Background(), nil, listing, PhotoSettings{})
	if view.Folders[0].MediaCountLabel != "3 Medien gesamt" {
		t.Fatalf("recursive count label = %q", view.Folders[0].MediaCountLabel)
	}
	if view.Folders[0].MediaCountTitle != "3 Medien inklusive Unterordner" {
		t.Fatalf("recursive count title = %q", view.Folders[0].MediaCountTitle)
	}
	if view.Folders[1].MediaCountLabel != "1 Medium" {
		t.Fatalf("direct count label = %q", view.Folders[1].MediaCountLabel)
	}
}

func TestDecoratePhotoListingTrustsGeneratedThumbnailIndexWhenFileIsMissing(t *testing.T) {
	installFakePhotoFFmpegThumbnailer(t)
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	thumbPath, err := photoLib.Thumbnail(context.Background(), "photo.jpg", 420)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(thumbPath); err != nil {
		t.Fatal(err)
	}

	media, err := photoLib.MediaContext(context.Background(), "photo.jpg")
	if err != nil {
		t.Fatal(err)
	}
	listing := photos.Listing{
		Media: []photos.Media{media},
	}
	view := newPhotoListingView(context.Background(), photoLib, listing, PhotoSettings{ThumbnailSize: 420})
	if !view.Media[0].ThumbReady {
		t.Fatalf("generated thumbnail metadata should mark media ready even without cache file: %#v", view.Media[0])
	}

	req := httptest.NewRequest(http.MethodGet, "/photos/thumbnail?path=photo.jpg&size=420", nil)
	rec := httptest.NewRecorder()
	server.handlePhotoThumbnail(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("thumbnail fallback status = %d body = %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(thumbPath); err != nil {
		t.Fatalf("thumbnail file should be regenerated: %v", err)
	}
}

func TestHandlePhotosRendersLeanLazyThumbnailPayload(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/photos", nil)
	rec := httptest.NewRecorder()
	server.handlePhotos(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`loading="lazy" decoding="async" data-photo-thumb-image`,
		`data-photo-path="photo.jpg"`,
		`data-photo-thumb-ready="0"`,
		`data-photo-thumb-src="/photos/thumbnail?path=photo.jpg&amp;size=420&amp;v=`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, notWant := range []string{
		`data-photo-preview=`,
		`data-photo-large-preview=`,
		`data-photo-fallback-src=`,
	} {
		if strings.Contains(body, notWant) {
			t.Fatalf("body contains %q: %s", notWant, body)
		}
	}
}

func TestHandlePhotosTraceRendersTimingPanelAndHeader(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "album", "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/photos?trace=1", nil)
	rec := httptest.NewRecorder()
	server.handlePhotos(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`class="photo-trace-panel"`,
		`photos.service.settings`,
		`photos.library.filesystem_listing`,
		`href="/photos?path=album&amp;trace=1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	timing := rec.Header().Get("Server-Timing")
	for _, want := range []string{"photo_total", "photos.render.template"} {
		if !strings.Contains(timing, want) {
			t.Fatalf("Server-Timing missing %q: %s", want, timing)
		}
	}
}

func TestPhotosTemplateSetsReadyThumbnailSrc(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	folderThumbURL := "/photos/thumbnail?path=album%2Fcover.jpg&size=320&v=1"
	mediaThumbURL := "/photos/thumbnail?path=photo.jpg&size=420&v=1"
	data := PageData{
		Title:     "Fotos",
		Active:    "photos",
		PhotoPage: true,
		Photos: PhotoListingView{
			Folders: []PhotoFolderView{
				{
					Folder: photos.Folder{
						Name: "album",
						Path: "album",
					},
					URL: "/photos?path=album",
					Previews: []PhotoMediaView{
						{
							Media:      photos.Media{Name: "cover.jpg", Path: "album/cover.jpg", Type: photos.MediaTypeImage},
							ThumbURL:   folderThumbURL,
							ThumbReady: true,
						},
					},
				},
			},
		},
		PhotoMediaGroups: []PhotoMediaGroup{
			{
				Media: []PhotoMediaView{
					{
						Media:      photos.Media{Name: "photo.jpg", Path: "photo.jpg", Type: photos.MediaTypeImage},
						ThumbURL:   mediaThumbURL,
						ThumbReady: true,
					},
				},
			},
		},
		PhotoSettings: defaultPhotoSettings(),
		Assets:        photoPageAssets(false),
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "photos.html", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		`src="/photos/thumbnail?path=album%2Fcover.jpg&amp;size=320&amp;v=1" data-photo-thumb-ready="1"`,
		`src="/photos/thumbnail?path=photo.jpg&amp;size=420&amp;v=1" data-photo-thumb-ready="1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestPhotosTemplateRendersTotalPageCount(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	data := PageData{
		Title:     "Fotos",
		Active:    "photos",
		PhotoPage: true,
		Photos: PhotoListingView{
			Page:     2,
			PageSize: 120,
			Total:    241,
		},
		PhotoFilter: PhotoFilter{
			PrevURL: "/photos",
			NextURL: "/photos?page=3",
		},
		PhotoSettings: defaultPhotoSettings(),
		Assets:        photoPageAssets(false),
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "photos.html", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if !strings.Contains(body, `Seite 2 von 3`) {
		t.Fatalf("body missing total page count: %s", body)
	}
	if strings.Contains(body, `pagination-pages`) {
		t.Fatalf("three-page pagination should remain simple: %s", body)
	}
}

func TestPhotosTemplateRendersCompactPaginationFromFourPages(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	data := PageData{
		Title:     "Fotos",
		Active:    "photos",
		PhotoPage: true,
		Photos: PhotoListingView{
			Page:     4,
			PageSize: 120,
			Total:    840,
		},
		PhotoFilter: PhotoFilter{
			PrevURL: "/photos?page=3",
			NextURL: "/photos?page=5",
			PageLinks: []PhotoPageLink{
				{Page: 1, URL: "/photos"},
				{Ellipsis: true},
				{Page: 3, URL: "/photos?page=3"},
				{Page: 4, Current: true},
				{Page: 5, URL: "/photos?page=5"},
				{Ellipsis: true},
				{Page: 7, URL: "/photos?page=7"},
			},
		},
		PhotoSettings: defaultPhotoSettings(),
		Assets:        photoPageAssets(false),
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "photos.html", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		`class="pagination-pages"`,
		`href="/photos" aria-label="Seite 1"`,
		`class="pagination-ellipsis"`,
		`class="pagination-page is-current" aria-current="page">4</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `Seite 4 von 7`) {
		t.Fatalf("compact pagination should replace plain page label: %s", body)
	}
}

func TestPhotoPageLinksPreserveFiltersAndCompactPages(t *testing.T) {
	if links := photoPageLinks(nil, 2, 3); links != nil {
		t.Fatalf("three-page links = %#v, want nil", links)
	}
	if links := photoPageLinks(nil, 2, 4); len(links) != 4 {
		t.Fatalf("four-page links = %#v", links)
	}

	base := url.Values{
		"path":  {"album"},
		"q":     {"tag:urlaub"},
		"type":  {"image"},
		"gps":   {"1"},
		"sort":  {"ascending_name"},
		"trace": {"1"},
	}
	got := photoPageLinks(base, 5, 10)
	if len(got) != 7 {
		t.Fatalf("links = %#v", got)
	}
	if got[0].Page != 1 || got[0].URL != "/photos?gps=1&path=album&q=tag%3Aurlaub&sort=ascending_name&trace=1&type=image" {
		t.Fatalf("first link = %#v", got[0])
	}
	if !got[1].Ellipsis || got[2].Page != 4 || !got[3].Current || got[4].Page != 6 || !got[5].Ellipsis || got[6].Page != 10 {
		t.Fatalf("compact sequence = %#v", got)
	}
	if got[2].URL != "/photos?gps=1&page=4&path=album&q=tag%3Aurlaub&sort=ascending_name&trace=1&type=image" {
		t.Fatalf("page 4 URL = %q", got[2].URL)
	}
}

func TestPhotosTemplateRendersAudioMedia(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	data := PageData{
		Title:     "Fotos",
		Active:    "photos",
		PhotoPage: true,
		PhotoMediaGroups: []PhotoMediaGroup{
			{
				Media: []PhotoMediaView{
					{
						Media:    photos.Media{Name: "song.mp3", Path: "song.mp3", Type: photos.MediaTypeAudio},
						MediaURL: "/photos/media?path=song.mp3",
					},
				},
			},
		},
		PhotoSettings: defaultPhotoSettings(),
		Assets:        photoPageAssets(false),
	}
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, "photos.html", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		`data-photo-type="audio"`,
		`data-photo-src="/photos/media?path=song.mp3"`,
		`class="photo-audio-placeholder"`,
		`data-photo-audio controls preload="metadata"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, `/photos/thumbnail?path=song.mp3`) {
		t.Fatalf("audio media should not request a generated thumbnail: %s", body)
	}
}

func TestHandlePhotoMediaInfoReturnsFaces(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "face.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "face.jpg.xmp"), []byte(`<x:xmpmeta xmlns:x="adobe:ns:meta/">
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description xmlns:mwg-rs="http://www.metadataworkinggroup.com/schemas/regions/" xmlns:stArea="http://ns.adobe.com/xmp/sType/Area#">
<mwg-rs:Regions rdf:parseType="Resource"><mwg-rs:RegionList><rdf:Bag>
<rdf:li rdf:parseType="Resource">
<mwg-rs:Area rdf:parseType="Resource"><stArea:h>0.10</stArea:h><stArea:w>0.20</stArea:w><stArea:x>0.50</stArea:x><stArea:y>0.40</stArea:y></mwg-rs:Area>
<mwg-rs:Name>Marie Curie</mwg-rs:Name><mwg-rs:Type>Face</mwg-rs:Type>
</rdf:li>
</rdf:Bag></mwg-rs:RegionList></mwg-rs:Regions>
</rdf:Description>
</rdf:RDF>
</x:xmpmeta>`), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/photos/media/info?path=face.jpg", nil)
	rec := httptest.NewRecorder()
	server.handlePhotoMediaInfo(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Media struct {
			Faces []struct {
				Name   string  `json:"name"`
				Left   float64 `json:"left"`
				Top    float64 `json:"top"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			} `json:"faces"`
		} `json:"media"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Media.Faces) != 1 {
		t.Fatalf("faces = %#v", payload.Media.Faces)
	}
	face := payload.Media.Faces[0]
	if face.Name != "Marie Curie" || face.Left < 0.3999 || face.Left > 0.4001 || face.Top < 0.3499 || face.Top > 0.3501 || face.Width != 0.2 || face.Height != 0.1 {
		t.Fatalf("face = %#v", face)
	}
}

func TestHandlePhotoMediaInfoBatch(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"one.jpg", "two.jpg"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte("photo"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodPost, "/photos/media/info", bytes.NewBufferString(`{"paths":["one.jpg","two.jpg","one.jpg"]}`))
	rec := httptest.NewRecorder()
	server.handlePhotoMediaInfo(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Media []struct {
			Path string `json:"path"`
			Src  string `json:"src"`
		} `json:"media"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Media) != 2 {
		t.Fatalf("media count = %d payload = %#v", len(payload.Media), payload.Media)
	}
	if payload.Media[0].Path != "one.jpg" || payload.Media[1].Path != "two.jpg" {
		t.Fatalf("media order = %#v", payload.Media)
	}
	if payload.Media[0].Src == "" || payload.Media[1].Src == "" {
		t.Fatalf("missing src in payload = %#v", payload.Media)
	}
}

func TestHandlePhotoThumbnailStatusAndPreviewGeneration(t *testing.T) {
	installFakePhotoFFmpegThumbnailer(t)
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	thumbPath := photoTestThumbnailPath(cacheDir, "photo.jpg", 960)
	req := httptest.NewRequest(http.MethodGet, "/photos/thumbnail/status?path=photo.jpg&size=960", nil)
	rec := httptest.NewRecorder()
	server.handlePhotoThumbnailStatus(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ready":false`) {
		t.Fatalf("initial status = %d body = %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/photos/thumbnail?path=photo.jpg&size=960&v=123", nil)
	rec = httptest.NewRecorder()
	server.handlePhotoThumbnail(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("thumbnail status = %d body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "image/webp" {
		t.Fatalf("content type = %q", contentType)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "private, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", cacheControl)
	}
	if _, err := os.Stat(thumbPath); err != nil {
		t.Fatalf("preview thumbnail missing: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/photos/thumbnail/status?path=photo.jpg&size=960", nil)
	rec = httptest.NewRecorder()
	server.handlePhotoThumbnailStatus(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ready":true`) {
		t.Fatalf("final status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandlePhotoThumbnailStatusBatchRejectsAdminOnlyPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", "private.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", photos.AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body := strings.NewReader(`{"items":[{"path":"public.jpg","size":420},{"path":"secret/private.jpg","size":420}]}`)
	req := httptest.NewRequest(http.MethodPost, "/photos/thumbnail/status", body)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	server.handlePhotoThumbnailStatus(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("batch admin-only status = %d body = %s", rec.Code, rec.Body.String())
	}
}

type testSQLiteCodeError struct {
	code int
}

func (e testSQLiteCodeError) Error() string {
	return "sqlite test error"
}

func (e testSQLiteCodeError) Code() int {
	return e.code
}

func TestPhotoThumbnailStatusQueueErrorTreatsBusyAsTransient(t *testing.T) {
	for _, err := range []error{
		testSQLiteCodeError{code: sqliteResultBusy},
		testSQLiteCodeError{code: sqliteResultLocked},
		testSQLiteCodeError{code: 517},
		context.DeadlineExceeded,
		context.Canceled,
	} {
		if !photoThumbnailStatusQueueError(err) {
			t.Fatalf("expected transient status queue error for %v", err)
		}
	}
	if photoThumbnailStatusQueueError(errors.New("broken query")) {
		t.Fatal("unexpected transient status queue error for unrelated error")
	}
}

func TestEnsurePhotoThumbnailsHonorsConfiguredWorkerBatchSize(t *testing.T) {
	installFakePhotoFFmpegThumbnailer(t)
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("photo"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		apps:   testPhotoJobApps(),
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	settings := defaultPhotoSettings()
	settings.ThumbnailSize = 120
	settings.ThumbnailWorkerBatchSize = 1

	generated, err := server.ensurePhotoThumbnails(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 1 || countPhotoThumbnailFiles(t, cacheDir) != 1 {
		t.Fatalf("first batch generated=%d files=%d", generated, countPhotoThumbnailFiles(t, cacheDir))
	}
	generated, err = server.ensurePhotoThumbnails(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 1 || countPhotoThumbnailFiles(t, cacheDir) != 2 {
		t.Fatalf("second batch generated=%d files=%d", generated, countPhotoThumbnailFiles(t, cacheDir))
	}
	generated, err = server.ensurePhotoThumbnails(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if generated != 0 || countPhotoThumbnailFiles(t, cacheDir) != 2 {
		t.Fatalf("complete batch generated=%d files=%d", generated, countPhotoThumbnailFiles(t, cacheDir))
	}
}

func TestAcquirePhotoJobHonorsAlreadyCanceledContext(t *testing.T) {
	server := &Server{apps: testPhotoJobApps()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	release, err := server.acquirePhotoJob(ctx)
	if release != nil {
		release()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire err = %v, want context.Canceled", err)
	}
	select {
	case server.apps.photo.jobs <- struct{}{}:
		<-server.apps.photo.jobs
	default:
		t.Fatal("canceled acquire occupied the photo job slot")
	}
}

func TestPhotoIndexJobBlocksThumbnailWorkerUntilContextCancel(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 4; i++ {
		dir := filepath.Join(root, fmt.Sprintf("album-%d", i))
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("photo"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		apps:   testPhotoJobApps(),
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	indexSettings := defaultPhotoSettings()
	indexSettings.IndexWorkerDelayMillis = 250
	done := make(chan error, 1)
	go func() {
		_, err := server.rebuildPhotoIndex(context.Background(), indexSettings)
		done <- err
	}()
	waitForPhotoJobLocked(t, server)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := server.ensurePhotoThumbnails(ctx, defaultPhotoSettings()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("thumbnail err while index locked = %v, want deadline", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPhotoThumbnailJobBlocksIndexWorkerUntilContextCancel(t *testing.T) {
	installSlowFakePhotoFFmpegThumbnailer(t, 250*time.Millisecond)
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		apps:   testPhotoJobApps(),
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	thumbnailSettings := defaultPhotoSettings()
	thumbnailSettings.ThumbnailSize = 120
	thumbnailSettings.ThumbnailWorkerBatchSize = 1
	done := make(chan error, 1)
	go func() {
		generated, err := server.ensurePhotoThumbnails(context.Background(), thumbnailSettings)
		if err == nil && generated != 1 {
			err = fmt.Errorf("generated = %d, want 1", generated)
		}
		done <- err
	}()
	waitForPhotoJobLocked(t, server)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := server.rebuildPhotoIndex(ctx, defaultPhotoSettings()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("index err while thumbnail locked = %v, want deadline", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if countPhotoThumbnailFiles(t, cacheDir) != 1 {
		t.Fatalf("thumbnail files = %d, want 1", countPhotoThumbnailFiles(t, cacheDir))
	}
}

func TestParallelPhotoBackgroundWorkersSerializeAndComplete(t *testing.T) {
	installSlowFakePhotoFFmpegThumbnailer(t, 80*time.Millisecond)
	ctx := context.Background()
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("photo"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		repo:   repo,
		photos: photoLib,
		apps:   testPhotoJobApps(),
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	settings := defaultPhotoSettings()
	settings.IndexWorkerEnabled = true
	settings.IndexWorkerIntervalMinutes = 1
	settings.IndexWorkerDelayMillis = 50
	settings.ThumbnailWorkerEnabled = true
	settings.ThumbnailWorkerIntervalMinutes = 1
	settings.ThumbnailWorkerBatchSize = 1
	settings.ThumbnailSize = 120
	if err := server.savePhotoSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}

	workerCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 2)
	go func() {
		server.RunPhotoIndexWorker(workerCtx)
		done <- struct{}{}
	}()
	go func() {
		server.RunPhotoThumbnailWorker(workerCtx)
		done <- struct{}{}
	}()
	waitForCondition(t, 3*time.Second, func() bool {
		return !photoLib.IndexTelemetry().FinishedAt.IsZero() && countPhotoThumbnailFiles(t, cacheDir) == 1
	})
	cancel()
	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("photo background worker did not stop after context cancel")
		}
	}
}

func TestManualPhotoWorkerStartsRunIndexAndThumbnails(t *testing.T) {
	installFakePhotoFFmpegThumbnailer(t)
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		apps:   testPhotoJobApps(),
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/photos/index/run", nil)
	rec := httptest.NewRecorder()
	server.handleRunPhotoIndexWorkerNow(rec, req)
	if rec.Code != http.StatusSeeOther || redirectNotice(t, rec.Header().Get("Location")) != "Foto-Indexierung im Hintergrund gestartet." {
		t.Fatalf("index worker status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	waitForCondition(t, 3*time.Second, func() bool {
		return photoLib.IndexTelemetry().Stats.Media == 1 && len(server.apps.photo.jobs) == 0
	})

	req = httptest.NewRequest(http.MethodPost, "/settings/photos/thumbnails/run", nil)
	rec = httptest.NewRecorder()
	server.handleRunPhotoThumbnailWorkerNow(rec, req)
	if rec.Code != http.StatusSeeOther || redirectNotice(t, rec.Header().Get("Location")) != "Thumbnail-Erzeugung im Hintergrund gestartet." {
		t.Fatalf("thumbnail worker status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	waitForCondition(t, 3*time.Second, func() bool {
		return countPhotoThumbnailFiles(t, cacheDir) == 1 && len(server.apps.photo.jobs) == 0
	})
}

func TestManualPhotoWorkerJobsUseBackgroundContext(t *testing.T) {
	installSlowFakePhotoFFmpegThumbnailer(t, 2*time.Second)
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		apps:   testPhotoJobApps(),
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	lifecycleCtx, cancel := context.WithCancel(context.Background())
	server.setBackgroundJobContext(lifecycleCtx)
	cancel()

	settings := defaultPhotoSettings()
	settings.IndexWorkerDelayMillis = 2_000
	settings.ThumbnailSize = 120
	settings.ThumbnailWorkerBatchSize = 1

	if !server.startPhotoIndexJob(settings) {
		t.Fatal("index job was not started")
	}
	waitForCondition(t, time.Second, func() bool {
		return len(server.apps.photo.jobs) == 0
	})

	if !server.startPhotoThumbnailJob(settings) {
		t.Fatal("thumbnail job was not started")
	}
	waitForCondition(t, time.Second, func() bool {
		return len(server.apps.photo.jobs) == 0
	})
	if countPhotoThumbnailFiles(t, cacheDir) != 0 {
		t.Fatalf("thumbnail files = %d, want none after canceled lifecycle context", countPhotoThumbnailFiles(t, cacheDir))
	}
}

func TestManualPhotoWorkerReportsBusyWhenPhotoJobRuns(t *testing.T) {
	photoLib, err := photos.New(t.TempDir(), filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		apps:   testPhotoJobApps(),
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	server.apps.photo.jobs <- struct{}{}
	defer func() { <-server.apps.photo.jobs }()

	req := httptest.NewRequest(http.MethodPost, "/settings/photos/index/run", nil)
	rec := httptest.NewRecorder()
	server.handleRunPhotoIndexWorkerNow(rec, req)
	if rec.Code != http.StatusSeeOther || redirectNotice(t, rec.Header().Get("Location")) != "Ein Foto-Hintergrundjob läuft bereits." {
		t.Fatalf("busy worker status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

func installFakePhotoFFmpegThumbnailer(t *testing.T) {
	t.Helper()
	tools := t.TempDir()
	ffmpeg := filepath.Join(tools, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\nprintf webp > \"$last\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func installSlowFakePhotoFFmpegThumbnailer(t *testing.T, delay time.Duration) {
	t.Helper()
	tools := t.TempDir()
	ffmpeg := filepath.Join(tools, "ffmpeg")
	script := fmt.Sprintf("#!/bin/sh\nsleep %.3f\nlast=\"\"\nfor arg in \"$@\"; do last=\"$arg\"; done\nprintf webp > \"$last\"\n", delay.Seconds())
	if err := os.WriteFile(ffmpeg, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func testPhotoJobApps() serverApplications {
	return serverApplications{
		photo: photoApplication{jobs: make(chan struct{}, 1)},
	}
}

func waitForPhotoJobLocked(t *testing.T, server *Server) {
	t.Helper()
	waitForCondition(t, time.Second, func() bool {
		return server != nil && len(server.apps.photo.jobs) == cap(server.apps.photo.jobs)
	})
}

func waitForCondition(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func redirectNotice(t *testing.T, location string) string {
	t.Helper()
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("invalid redirect location %q: %v", location, err)
	}
	return parsed.Query().Get("notice")
}

func countPhotoThumbnailFiles(t *testing.T, cacheDir string) int {
	t.Helper()
	count := 0
	root := filepath.Join(cacheDir, "thumbnails")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		count++
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return count
}

func TestHandlePhotoMediaAllowsAdminOnlyFilesOnlyForAdmins(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "secret"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", "private.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", photos.AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
			{Username: "admin", Password: "secret", Role: "admin"},
			{Username: "photos", Password: "secret", Role: "photos_read"},
		}}},
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/photos/media?path=secret/private.jpg", nil)
	req = authenticatedTestRequest(server, req, "photos")
	rec := httptest.NewRecorder()
	server.handlePhotoMedia(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("photos user status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/photos/media?path=secret/private.jpg", nil)
	req = authenticatedTestRequest(server, req, "admin")
	rec = httptest.NewRecorder()
	server.handlePhotoMedia(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin status = %d body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "image/jpeg" {
		t.Fatalf("content type = %q", contentType)
	}
}

func TestHandlePhotoRandomStreamsMediaDirectly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/photos/random", nil)
	rec := httptest.NewRecorder()
	server.handlePhotoRandom(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("unexpected redirect location = %q", location)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "image/jpeg" {
		t.Fatalf("content type = %q", contentType)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("cache control = %q", cacheControl)
	}
	if nosniff := rec.Header().Get("X-Content-Type-Options"); nosniff != "nosniff" {
		t.Fatalf("x-content-type-options = %q", nosniff)
	}
	if title := rec.Header().Get("X-BearStack-Photo-Title"); title != cleanPhotoFrameTitle("photo.jpg") {
		t.Fatalf("title header = %q", title)
	}
	if mediaPath := rec.Header().Get("X-BearStack-Photo-Path"); mediaPath != "photo.jpg" {
		t.Fatalf("media path header = %q", mediaPath)
	}
	if folderPath := rec.Header().Get("X-BearStack-Photo-Folder-Path"); folderPath != "" {
		t.Fatalf("folder path header = %q", folderPath)
	}
	if folderURL := rec.Header().Get("X-BearStack-Photo-Folder-URL"); folderURL != "http://example.com/photos" {
		t.Fatalf("folder url header = %q", folderURL)
	}
	if folderTitle := rec.Header().Get("X-BearStack-Photo-Folder-Title"); folderTitle != "Fotos" {
		t.Fatalf("folder title header = %q", folderTitle)
	}
	if link := rec.Header().Get("Link"); link != "<http://example.com/photos>; rel=\"up\"" {
		t.Fatalf("link header = %q", link)
	}
	if body := rec.Body.String(); body != "photo" {
		t.Fatalf("body = %q", body)
	}
}

func TestHandlePhotoRandomIncludesFolderLinkAndCleanTitleHeaders(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "urlaub", "sommer"), 0o750); err != nil {
		t.Fatal(err)
	}
	filename := "2026-05-18_Sommer-Urlaub.jpg"
	relativePath := filepath.ToSlash(filepath.Join("urlaub", "sommer", filename))
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(relativePath)), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/photos/random", nil)
	req.Host = "photos.example:8443"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	server.handlePhotoRandom(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if title := rec.Header().Get("X-BearStack-Photo-Title"); title != "Sommer Urlaub" {
		t.Fatalf("title header = %q", title)
	}
	if mediaPath := rec.Header().Get("X-BearStack-Photo-Path"); mediaPath != relativePath {
		t.Fatalf("media path header = %q", mediaPath)
	}
	if folderPath := rec.Header().Get("X-BearStack-Photo-Folder-Path"); folderPath != "urlaub/sommer" {
		t.Fatalf("folder path header = %q", folderPath)
	}
	if folderURL := rec.Header().Get("X-BearStack-Photo-Folder-URL"); folderURL != "https://photos.example:8443/photos?path=urlaub%2Fsommer" {
		t.Fatalf("folder url header = %q", folderURL)
	}
	if folderTitle := rec.Header().Get("X-BearStack-Photo-Folder-Title"); folderTitle != "sommer" {
		t.Fatalf("folder title header = %q", folderTitle)
	}
	if link := rec.Header().Get("Link"); link != "<https://photos.example:8443/photos?path=urlaub%2Fsommer>; rel=\"up\"" {
		t.Fatalf("link header = %q", link)
	}
}

func TestHandlePhotoRandomStreamsConfiguredSize(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := defaultPhotoSettings()
	writePhotoRandomThumbnail(t, cacheDir, "photo.jpg", settings.FolderThumbnailSize, "folder")
	writePhotoRandomThumbnail(t, cacheDir, "photo.jpg", settings.ThumbnailSize, "gallery")
	writePhotoRandomThumbnail(t, cacheDir, "photo.jpg", settings.PreviewSize, "large")
	writePhotoRandomThumbnail(t, cacheDir, "photo.jpg", settings.LargePreviewSize, "hd")
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	for _, tc := range []struct {
		target string
		body   string
	}{
		{"/photos/random?size=ordner", "folder"},
		{"/photos/random?size=galerie", "gallery"},
		{"/photos/random?size=gro%C3%9F", "large"},
		{"/photos/random?size=hd", "hd"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			rec := httptest.NewRecorder()
			server.handlePhotoRandom(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); contentType != "image/webp" {
				t.Fatalf("content type = %q", contentType)
			}
			if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
				t.Fatalf("cache control = %q", cacheControl)
			}
			if body := rec.Body.String(); body != tc.body {
				t.Fatalf("body = %q, want %q", body, tc.body)
			}
		})
	}
}

func TestPhotoRandomFallsBackToBasicAuthWhenNotHTML(t *testing.T) {
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/photos/random", nil)
	req.Header.Set("Accept", "image/webp")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatalf("missing auth challenge")
	}

	req = httptest.NewRequest(http.MethodGet, "/photos/random", nil)
	req.Header.Set("Accept", "text/html")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="return"`) {
		t.Fatalf("login page missing return field: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/photos/random", nil)
	req.SetBasicAuth("photos_read", "secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestParsePhotoRandomDeliverySize(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  photoRandomDeliverySize
		ok    bool
	}{
		{"", photoRandomDeliveryOriginal, true},
		{"original", photoRandomDeliveryOriginal, true},
		{"ordner", photoRandomDeliveryFolder, true},
		{"folder", photoRandomDeliveryFolder, true},
		{"galerie", photoRandomDeliveryGallery, true},
		{"gallery", photoRandomDeliveryGallery, true},
		{"groß", photoRandomDeliveryLarge, true},
		{"gross", photoRandomDeliveryLarge, true},
		{"large", photoRandomDeliveryLarge, true},
		{"hd", photoRandomDeliveryHD, true},
		{"unknown", "", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			got, ok := parsePhotoRandomDeliverySize(tc.value)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("parsePhotoRandomDeliverySize(%q) = %q, %v, want %q, %v", tc.value, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func writePhotoRandomThumbnail(t *testing.T, cacheDir, rel string, size int, body string) {
	t.Helper()
	path := photoTestThumbnailPath(cacheDir, rel, size)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	modTime := time.Now().Add(time.Second)
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func photoTestThumbnailPath(cacheDir, rel string, size int) string {
	key := photoTestThumbnailKey(rel)
	return filepath.Join(cacheDir, "thumbnails", "v2", key[:2], key[2:4], fmt.Sprintf("%s_%dq80.webp", key, photos.NormalizeThumbnailSize(size)))
}

func photoTestThumbnailKey(rel string) string {
	sum := sha256.Sum256([]byte(photoTestThumbnailCanonicalRel(rel)))
	return hex.EncodeToString(sum[:])
}

func photoTestThumbnailCanonicalRel(rel string) string {
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == string(filepath.Separator) || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		clean = filepath.Base(clean)
		if clean == "." || clean == string(filepath.Separator) {
			clean = "media"
		}
	}
	return filepath.ToSlash(clean)
}

func TestPhotoAdminOnlyHiddenForAdminUntilSessionToggle(t *testing.T) {
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/photos", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("default gallery status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `href="/photos?path=secret"`) {
		t.Fatalf("default admin gallery exposes admin-only folder: %s", body)
	}
	for _, want := range []string{`action="/photos/adminonly"`, `role="switch"`, `aria-checked="false"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("default admin gallery misses %q: %s", want, body)
		}
	}
	cookie := testCookieByName(t, rec.Result().Cookies(), authSessionCookieName)

	form := url.Values{"return": {"/photos"}, "show": {"1"}}
	req = httptest.NewRequest(http.MethodPost, "/photos/adminonly", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/photos" {
		t.Fatalf("toggle status = %d location = %q body = %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	cookie = testCookieByName(t, rec.Result().Cookies(), authSessionCookieName)

	req = httptest.NewRequest(http.MethodGet, "/photos", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("visible gallery status = %d body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, `href="/photos?path=secret"`) {
		t.Fatalf("toggled admin gallery hides admin-only folder: %s", body)
	}
	if !strings.Contains(body, `aria-checked="true"`) {
		t.Fatalf("toggled admin gallery misses checked switch: %s", body)
	}
}

func TestPhotoRoutePermissionMatrix(t *testing.T) {
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()
	photoReadRoles := allowedTestRoles("admin", "photos_read", "photos_editor", "photos_manager")
	photoEditRoles := allowedTestRoles("admin", "photos_editor", "photos_manager")
	photoManageRoles := allowedTestRoles("admin", "photos_manager")
	tagPageRoles := allowedTestRoles("admin", "documents_read", "documents_editor", "documents_manager", "photos_read", "photos_editor", "photos_manager")
	cases := []struct {
		name       string
		method     string
		target     string
		acceptJSON bool
		form       func(string) url.Values
		allowed    map[string]bool
	}{
		{name: "gallery", method: http.MethodGet, target: "/photos", allowed: photoReadRoles},
		{name: "frame", method: http.MethodGet, target: "/photos/frame", allowed: photoReadRoles},
		{name: "frame items", method: http.MethodGet, target: "/photos/frame/items", acceptJSON: true, allowed: photoReadRoles},
		{name: "random", method: http.MethodGet, target: "/photos/random", allowed: photoReadRoles},
		{name: "media", method: http.MethodGet, target: "/photos/media?path=public.jpg", allowed: photoReadRoles},
		{name: "media info", method: http.MethodGet, target: "/photos/media/info?path=public.jpg", acceptJSON: true, allowed: photoReadRoles},
		{name: "thumbnail", method: http.MethodGet, target: "/photos/thumbnail?path=public.jpg&size=420", allowed: photoReadRoles},
		{name: "thumbnail status", method: http.MethodGet, target: "/photos/thumbnail/status?path=public.jpg&size=420", acceptJSON: true, allowed: photoReadRoles},
		{name: "thumbnail batch status", method: http.MethodPost, target: "/photos/thumbnail/status", acceptJSON: true, allowed: photoReadRoles},
		{name: "photo tag page", method: http.MethodGet, target: "/tags?tab=photos", allowed: tagPageRoles},
		{name: "photo tag options", method: http.MethodGet, target: "/photos/tags/options", acceptJSON: true, allowed: photoEditRoles},
		{name: "set media tags", method: http.MethodPost, target: "/photos/tags?kind=media&path=public.jpg", acceptJSON: true, allowed: photoEditRoles, form: func(role string) url.Values {
			return url.Values{"tags": {"tag-" + safeRoleName(role)}}
		}},
		{name: "bulk add tags", method: http.MethodPost, target: "/photos/tags/add", acceptJSON: true, allowed: photoEditRoles, form: func(role string) url.Values {
			return url.Values{"ids": {"public.jpg"}, "tags": {"bulk-" + safeRoleName(role)}}
		}},
		{name: "bulk remove tags", method: http.MethodPost, target: "/photos/tags/remove", acceptJSON: true, allowed: photoEditRoles, form: func(role string) url.Values {
			return url.Values{"ids": {"public.jpg"}, "tags": {"bulk-" + safeRoleName(role)}}
		}},
		{name: "create library tag", method: http.MethodPost, target: "/photos/tags/library", allowed: photoManageRoles, form: func(role string) url.Values {
			return url.Values{"name": {"create-" + safeRoleName(role)}}
		}},
		{name: "rename library tag", method: http.MethodPost, target: "/photos/tags/library/rename", allowed: photoManageRoles, form: func(role string) url.Values {
			name := "rename-" + safeRoleName(role)
			return url.Values{"old_name": {name}, "name": {name + "-new"}}
		}},
		{name: "delete library tag", method: http.MethodPost, target: "/photos/tags/library/delete", allowed: photoManageRoles, form: func(role string) url.Values {
			return url.Values{"name": {"delete-" + safeRoleName(role)}, "password": {"secret"}}
		}},
		{name: "photo settings", method: http.MethodGet, target: "/settings/photos", allowed: photoManageRoles},
		{name: "save photo settings", method: http.MethodPost, target: "/settings/photos", allowed: photoManageRoles, form: func(string) url.Values {
			return photoSettingsTestForm()
		}},
		{name: "run photo index worker", method: http.MethodPost, target: "/settings/photos/index/run", allowed: photoManageRoles},
		{name: "run photo thumbnail worker", method: http.MethodPost, target: "/settings/photos/thumbnails/run", allowed: photoManageRoles},
	}
	for _, tc := range cases {
		for _, role := range photoAuthMatrixRoles() {
			t.Run(tc.name+"/"+role, func(t *testing.T) {
				req := newPhotoAuthMatrixRequest(tc.method, tc.target, role, tc.acceptJSON, testFormForRole(tc.form, role))
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				assertPhotoAuthMatrixStatus(t, role, tc.allowed[role], rec)
			})
		}
	}
}

func TestLegacyMailImportRoutesAreNotRegistered(t *testing.T) {
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()
	for _, tc := range []struct {
		method string
		target string
	}{
		{http.MethodGet, "/mail-import"},
		{http.MethodPost, "/mail-import"},
		{http.MethodPost, "/mail-import/test"},
		{http.MethodPost, "/mail-import/run"},
	} {
		req := httptest.NewRequest(tc.method, tc.target, nil)
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want 404", tc.method, tc.target, rec.Code)
		}
	}
}

func TestRootRouteRedirectsPhotoOnlyRolesToPhotos(t *testing.T) {
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()

	for _, role := range []string{"photos_read", "photos_editor", "photos_manager"} {
		t.Run("photo role "+role, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetBasicAuth(role, "secret")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/photos" {
				t.Fatalf("role %q status=%d location=%q body=%s", role, rec.Code, rec.Header().Get("Location"), rec.Body.String())
			}
		})
	}

	for _, role := range []string{"documents_read", "documents_editor", "documents_manager", "admin"} {
		t.Run("document role "+role, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetBasicAuth(role, "secret")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/documents" {
				t.Fatalf("role %q status=%d body=%s", role, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRootRouteUsesConfiguredHomePageWithPermissionFallbacks(t *testing.T) {
	ctx := context.Background()
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()

	cases := []struct {
		name         string
		setting      string
		cloudEnabled bool
		role         string
		location     string
	}{
		{name: "cloud for admin", setting: homePageCloud, cloudEnabled: true, role: "admin", location: "/cloud"},
		{name: "cloud disabled fallback", setting: homePageCloud, role: "admin", location: "/documents"},
		{name: "folders for document reader", setting: homePageFolders, role: "documents_read", location: "/folders"},
		{name: "photos for photo reader", setting: homePagePhotos, role: "photos_read", location: "/photos"},
		{name: "photos fallback for document reader", setting: homePagePhotos, role: "documents_read", location: "/documents"},
		{name: "invalid fallback", setting: "kaputt", role: "admin", location: "/documents"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := server.repo.SaveSetting(ctx, homePageSettingKey, tc.setting); err != nil {
				t.Fatal(err)
			}
			if err := server.repo.SaveSetting(ctx, documentCloudEnabledSettingKey, boolSettingValue(tc.cloudEnabled)); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetBasicAuth(tc.role, "secret")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != tc.location {
				t.Fatalf("status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
			}
		})
	}
}

func TestDocumentMainRoutesRenderOnFixedPaths(t *testing.T) {
	ctx := context.Background()
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()
	req := httptest.NewRequest(http.MethodGet, "/documents", nil)
	req.SetBasicAuth("documents_read", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/documents status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := server.repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "1"); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/cloud", nil)
	req.SetBasicAuth("documents_read", "secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/cloud status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDocumentCloudDisabledByDefaultHidesNavigationAndRoute(t *testing.T) {
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()
	req := httptest.NewRequest(http.MethodGet, "/cloud", nil)
	req.SetBasicAuth("documents_read", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/cloud status=%d body=%s", rec.Code, rec.Body.String())
	}

	for _, target := range []string{"/documents", "/help"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.SetBasicAuth("documents_read", "secret")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", target, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `href="/cloud"`) {
			t.Fatalf("%s rendered disabled cloud navigation: %s", target, rec.Body.String())
		}
	}
}

func TestPhotoAdminOnlyPermissionMatrix(t *testing.T) {
	server := newPhotoAuthMatrixServer(t)
	handler := server.Handler()
	adminOnly := allowedTestRoles("admin")
	cases := []struct {
		name       string
		method     string
		target     string
		acceptJSON bool
		form       func(string) url.Values
		allowed    map[string]bool
	}{
		{name: "gallery folder", method: http.MethodGet, target: "/photos?path=secret", allowed: allowedTestRoles()},
		{name: "media", method: http.MethodGet, target: "/photos/media?path=secret/private.jpg"},
		{name: "media info", method: http.MethodGet, target: "/photos/media/info?path=secret/private.jpg", acceptJSON: true},
		{name: "thumbnail", method: http.MethodGet, target: "/photos/thumbnail?path=secret/private.jpg&size=420"},
		{name: "thumbnail status", method: http.MethodGet, target: "/photos/thumbnail/status?path=secret/private.jpg&size=420", acceptJSON: true},
		{name: "toggle visibility", method: http.MethodPost, target: "/photos/adminonly", form: func(role string) url.Values {
			return url.Values{"return": {"/photos"}, "show": {"1"}}
		}},
		{name: "set media tags", method: http.MethodPost, target: "/photos/tags?kind=media&path=secret/private.jpg", acceptJSON: true, form: func(role string) url.Values {
			return url.Values{"tags": {"secret-" + safeRoleName(role)}}
		}},
		{name: "set folder tags", method: http.MethodPost, target: "/photos/tags?kind=folder&path=secret", acceptJSON: true, form: func(role string) url.Values {
			return url.Values{"tags": {"secret-folder-" + safeRoleName(role)}}
		}},
		{name: "bulk add tags", method: http.MethodPost, target: "/photos/tags/add", acceptJSON: true, form: func(role string) url.Values {
			return url.Values{"ids": {"secret/private.jpg"}, "tags": {"secret-bulk-" + safeRoleName(role)}}
		}},
	}
	for _, tc := range cases {
		for _, role := range []string{"photos_read", "photos_editor", "photos_manager", "admin"} {
			t.Run(tc.name+"/"+role, func(t *testing.T) {
				req := newPhotoAuthMatrixRequest(tc.method, tc.target, role, tc.acceptJSON, testFormForRole(tc.form, role))
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				allowed := adminOnly
				if tc.allowed != nil {
					allowed = tc.allowed
				}
				assertPhotoAuthMatrixStatus(t, role, allowed[role], rec)
			})
		}
	}
}

func newPhotoAuthMatrixServer(t *testing.T) *Server {
	t.Helper()
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "repo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if _, err := repo.SaveTag(ctx, "Dokument", "", "#176b87", false, false); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := os.Mkdir(filepath.Join(root, "secret"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", "private.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", photos.AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	writePhotoAuthMatrixThumbnail(t, cacheDir, "public.jpg", 420)
	writePhotoAuthMatrixThumbnail(t, cacheDir, "secret/private.jpg", 420)
	photoLib, err := photos.New(root, cacheDir, filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = photoLib.Close() })
	for _, role := range photoAuthMatrixRoles() {
		if role == "" {
			continue
		}
		name := safeRoleName(role)
		if _, err := photoLib.SaveTag(ctx, "rename-"+name); err != nil {
			t.Fatal(err)
		}
		if _, err := photoLib.SaveTag(ctx, "delete-"+name); err != nil {
			t.Fatal(err)
		}
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	return &Server{
		cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
			{Username: "api_uploader", Password: "secret", Role: "api_uploader"},
			{Username: "documents_read", Password: "secret", Role: "documents_read"},
			{Username: "documents_editor", Password: "secret", Role: "documents_editor"},
			{Username: "documents_manager", Password: "secret", Role: "documents_manager"},
			{Username: "photos_read", Password: "secret", Role: "photos_read"},
			{Username: "photos_editor", Password: "secret", Role: "photos_editor"},
			{Username: "photos_manager", Password: "secret", Role: "photos_manager"},
			{Username: "admin", Password: "secret", Role: "admin"},
		}}},
		repo:      repo,
		photos:    photoLib,
		templates: templates,
		static:    http.NotFoundHandler(),
		authKey:   []byte("01234567890123456789012345678901"),
		apps:      testPhotoJobApps(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func writePhotoAuthMatrixThumbnail(t *testing.T, cacheDir, rel string, size int) {
	t.Helper()
	path := photoTestThumbnailPath(cacheDir, rel, size)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("webp"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func photoAuthMatrixRoles() []string {
	return []string{"", "api_uploader", "documents_read", "documents_editor", "documents_manager", "photos_read", "photos_editor", "photos_manager", "admin"}
}

func allowedTestRoles(roles ...string) map[string]bool {
	allowed := map[string]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	return allowed
}

func testFormForRole(fn func(string) url.Values, role string) url.Values {
	if fn == nil {
		return nil
	}
	return fn(role)
}

func newPhotoAuthMatrixRequest(method, target, role string, acceptJSON bool, form url.Values) *http.Request {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if acceptJSON {
		req.Header.Set("Accept", "application/json")
	}
	if role != "" {
		req.SetBasicAuth(role, "secret")
	}
	return req
}

func assertPhotoAuthMatrixStatus(t *testing.T, role string, allowed bool, rec *httptest.ResponseRecorder) {
	t.Helper()
	if role == "" {
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous status = %d body = %s", rec.Code, rec.Body.String())
		}
		return
	}
	if !allowed {
		if rec.Code != http.StatusForbidden {
			t.Fatalf("forbidden role %q status = %d body = %s", role, rec.Code, rec.Body.String())
		}
		return
	}
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Fatalf("allowed role %q status = %d body = %s", role, rec.Code, rec.Body.String())
	}
}

func safeRoleName(role string) string {
	if role == "" {
		return "anonymous"
	}
	return strings.ReplaceAll(role, "_", "-")
}

func testCookieByName(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %q in %#v", name, cookies)
	return nil
}

func photoSettingsTestForm() url.Values {
	return url.Values{
		"page_size":                         {"50"},
		"folder_preview_count":              {"4"},
		"folder_thumbnail_size":             {"320"},
		"thumbnail_size":                    {"420"},
		"preview_size":                      {"1200"},
		"large_preview_size":                {"3840"},
		"slideshow_seconds":                 {"6"},
		"frame_seconds":                     {"30"},
		"preload_adjacent":                  {"1"},
		"index_worker_enabled":              {"1"},
		"index_worker_interval_minutes":     {"45"},
		"index_worker_delay_millis":         {"300"},
		"thumbnail_worker_enabled":          {"1"},
		"thumbnail_worker_interval_minutes": {"15"},
		"thumbnail_worker_batch_size":       {"25"},
		"thumbnail_concurrency":             {"2"},
	}
}

func TestHandleBulkPhotoTagsRejectsAdminOnlySelectionWithoutPartialUpdate(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "secret"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", "private.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", photos.AdminOnlyMarkerName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
			{Username: "editor", Password: "secret", Role: "photos_editor"},
		}}},
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{
		"ids":  {"public.jpg", "secret/private.jpg"},
		"tags": {"Reise"},
	}
	req := httptest.NewRequest(http.MethodPost, "/photos/tags/add", strings.NewReader(form.Encode()))
	req = authenticatedTestRequest(server, req, "editor")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	server.handleAddPhotoTags(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	media, err := photoLib.MediaContext(context.Background(), "public.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Tags) != 0 {
		t.Fatalf("public media was changed before admin-only rejection: %#v", media.Tags)
	}
}

func TestHandleBulkPhotoTagsRejectsEmptyNormalizedTags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "public.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server := &Server{
		photos: photoLib,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{
		"ids":  {"public.jpg"},
		"tags": {" ", ","},
	}
	req := httptest.NewRequest(http.MethodPost, "/photos/tags/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	server.handleAddPhotoTags(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "mindestens einen Tag") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	media, err := photoLib.MediaContext(context.Background(), "public.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(media.Tags) != 0 {
		t.Fatalf("media tags changed after empty bulk tags: %#v", media.Tags)
	}
}

func TestHandleSaveSettingsStoresDesktopPreviewMode(t *testing.T) {
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
	if err := repo.SaveSetting(ctx, appNameSettingKey, "Vorher"); err != nil {
		t.Fatal(err)
	}
	name, err := server.appName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Vorher" {
		t.Fatalf("preloaded app name = %q", name)
	}
	form := url.Values{"app_name": {"Archiv"}, "desktop_preview_mode": {"inline"}, "tag_display_mode": {"strtoupper"}, "theme_mode": {"design2"}, "home_page": {"cloud"}, "document_cloud_enabled": {"1"}, "trash_retention_days": {"60"}, "folder_tag_min_documents": {"7"}}
	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleSaveSettings(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	value, ok, err := repo.GetSetting(ctx, appNameSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "Archiv" {
		t.Fatalf("app name setting ok=%v value=%q", ok, value)
	}
	if err := repo.SaveSetting(ctx, appNameSettingKey, "Externe Änderung"); err != nil {
		t.Fatal(err)
	}
	name, err = server.appName(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Archiv" {
		t.Fatalf("cached app name after save = %q", name)
	}
	value, ok, err = repo.GetSetting(ctx, desktopPreviewModeSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != desktopPreviewModeInline {
		t.Fatalf("setting ok=%v value=%q", ok, value)
	}
	value, ok, err = repo.GetSetting(ctx, tagDisplayModeSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != tagDisplayModeUpper {
		t.Fatalf("tag setting ok=%v value=%q", ok, value)
	}
	value, ok, err = repo.GetSetting(ctx, themeModeSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != themeModeDesign2 {
		t.Fatalf("theme setting ok=%v value=%q", ok, value)
	}
	value, ok, err = repo.GetSetting(ctx, homePageSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != homePageCloud {
		t.Fatalf("home page setting ok=%v value=%q", ok, value)
	}
	value, ok, err = repo.GetSetting(ctx, documentCloudEnabledSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "1" {
		t.Fatalf("document cloud setting ok=%v value=%q", ok, value)
	}
	value, ok, err = repo.GetSetting(ctx, trashRetentionDaysSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "60" {
		t.Fatalf("trash retention setting ok=%v value=%q", ok, value)
	}
	value, ok, err = repo.GetSetting(ctx, folderTagMinDocumentsSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "7" {
		t.Fatalf("folder tag min documents setting ok=%v value=%q", ok, value)
	}
}

func TestHandleSettingsRendersDesktopPreviewMode(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveSetting(ctx, desktopPreviewModeSettingKey, desktopPreviewModeInline); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, tagDisplayModeSettingKey, tagDisplayModeFirst); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, themeModeSettingKey, themeModeDesign2); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, trashRetentionDaysSettingKey, "90"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, folderTagMinDocumentsSettingKey, "12"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, appNameSettingKey, "Aktenkiste"); err != nil {
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
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	server.handleSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `<title>Einstellungen - Aktenkiste</title>`) {
		t.Fatalf("custom title not rendered: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<html lang="de" data-theme="design2">`) {
		t.Fatalf("theme mode not rendered on html element: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<a class="brand" href="/documents" aria-label="Aktenkiste">`) ||
		!strings.Contains(rec.Body.String(), `class="brand-icon" src="/static/bearstack.svg`) ||
		!strings.Contains(rec.Body.String(), `<span class="brand-label">Aktenkiste</span>`) {
		t.Fatalf("custom brand not rendered: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="app_name" maxlength="80" value="Aktenkiste"`) {
		t.Fatalf("app name setting not rendered: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="desktop_preview_mode" value="inline" checked`) {
		t.Fatalf("inline preview setting not rendered checked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="tag_display_mode" value="ucfirst" checked`) {
		t.Fatalf("tag display setting not rendered checked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="theme_mode" value="design2" checked`) {
		t.Fatalf("theme setting not rendered checked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<strong>Wüste</strong>`) {
		t.Fatalf("theme label not rendered: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<legend>Startseite</legend>`) || !strings.Contains(rec.Body.String(), `name="home_page" value="documents" checked`) {
		t.Fatalf("home page setting not rendered checked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `href="/cloud"`) {
		t.Fatalf("enabled cloud navigation not rendered: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="document_cloud_enabled" value="1" checked`) {
		t.Fatalf("document cloud setting not rendered checked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `option value="90" selected`) {
		t.Fatalf("trash retention setting not rendered selected: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="folder_tag_min_documents" min="0" max="100000" value="12"`) {
		t.Fatalf("folder tag minimum setting not rendered: %s", rec.Body.String())
	}
}

func TestHandleUploadFaviconStoresServesAndRendersCustomIcon(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	data := testFaviconPNG()
	body, contentType := multipartUploadBody(t, customFaviconFormField, "favicon.png", data)
	req := httptest.NewRequest(http.MethodPost, "/settings/favicon", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	server.handleUploadFavicon(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	icon, ok, err := server.customFavicon(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("custom favicon was not stored")
	}
	if icon.Filename != "favicon.png" || icon.MIMEType != "image/png" || !bytes.Equal(icon.Data, data) {
		t.Fatalf("stored favicon = %#v data=%q", icon, string(icon.Data))
	}

	req = httptest.NewRequest(http.MethodGet, customFaviconRoute+"?v="+customFaviconVersion(icon), nil)
	rec = httptest.NewRecorder()
	server.handleCustomFavicon(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("favicon status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Fatalf("cache control = %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("served favicon = %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec = httptest.NewRecorder()
	server.handleSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("settings status = %d body = %s", rec.Code, rec.Body.String())
	}
	settingsBody := rec.Body.String()
	for _, want := range []string{
		`<link rel="icon" href="/favicon/custom?v=`,
		`action="/settings/favicon" enctype="multipart/form-data"`,
		`favicon.png · `,
		`form="favicon-reset-form"`,
	} {
		if !strings.Contains(settingsBody, want) {
			t.Fatalf("settings body missing %q: %s", want, settingsBody)
		}
	}
	if strings.Contains(settingsBody, `<link rel="icon" href="/static/bearstack.svg?v=`) {
		t.Fatalf("settings body rendered multiple favicon links: %s", settingsBody)
	}
}

func TestHandleResetFaviconRestoresDefault(t *testing.T) {
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
	icon, err := newCustomFavicon("favicon.png", testFaviconPNG())
	if err != nil {
		t.Fatal(err)
	}
	if err := server.settingsService().SaveCustomFavicon(ctx, icon); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/favicon/reset", nil)
	rec := httptest.NewRecorder()

	server.handleResetFavicon(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	_, ok, err := server.customFavicon(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("custom favicon still active after reset")
	}
	value, ok, err := repo.GetSetting(ctx, customFaviconSettingKey)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "" {
		t.Fatalf("favicon setting ok=%v value=%q", ok, value)
	}
}

func TestHandleUploadFaviconRejectsUnsupportedType(t *testing.T) {
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
	body, contentType := multipartUploadBody(t, customFaviconFormField, "favicon.svg", []byte("<svg></svg>"))
	req := httptest.NewRequest(http.MethodPost, "/settings/favicon", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	server.handleUploadFavicon(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "Dateityp+nicht+unterst%C3%BCtzt") {
		t.Fatalf("location = %q", location)
	}
	if _, ok, err := server.customFavicon(ctx); err != nil || ok {
		t.Fatalf("custom favicon stored after rejected upload: ok=%v err=%v", ok, err)
	}
}

func testFaviconPNG() []byte {
	return []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00,
	}
}

func TestHandleIndexRendersInlineDesktopPreviewFromSetting(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveSetting(ctx, desktopPreviewModeSettingKey, desktopPreviewModeInline); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "preview.pdf",
		StoredPath:    "2026/05/preview.pdf",
		Title:         "Preview",
		MIMEType:      "application/pdf",
		SizeBytes:     42,
		SHA256:        "inline-preview",
		SearchVersion: document.CurrentSearchVersion,
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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-preview-mode="inline"`) || !strings.Contains(body, `data-side-preview`) {
		t.Fatalf("inline side preview missing: %s", body)
	}
}

func TestHandleIndexRendersDesktopDateUnderTitleSetting(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	settings, err := json.Marshal(storedDocumentColumnSettings{
		Order:                 defaultDocumentColumnOrder(nil),
		Visible:               []string{"name", "title", "tags", "upload_date", "size", "actions"},
		DesktopDateUnderTitle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSetting(ctx, documentColumnsSettingKey, string(settings)); err != nil {
		t.Fatal(err)
	}
	documentDate := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:      "date-under-title.pdf",
		StoredPath:        "2026/05/date-under-title.pdf",
		Title:             "Datum unter Titel",
		MIMEType:          "application/pdf",
		SizeBytes:         42,
		SHA256:            "date-under-title",
		DocumentDate:      &documentDate,
		SearchVersion:     document.CurrentSearchVersion,
		ContentTextSource: document.ContentTextSourceNone,
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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`document-list-table-desktop-date-under-title`,
		`name="desktop_date_under_title" value="1" checked`,
		`class="document-title-date" data-label="Dateidatum"`,
		`value="2026-05-10"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestHandleSaveColumnsStoresDesktopDateUnderTitle(t *testing.T) {
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
	form := url.Values{
		"return":                   {"/"},
		"column_order":             defaultDocumentColumnOrder(nil),
		"columns":                  defaultDocumentColumns,
		"desktop_date_under_title": {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/columns", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleSaveColumns(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	columnSettings, err := server.documentColumnSettings(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !columnSettings.desktopDateUnderTitle {
		t.Fatal("desktop date under title setting was not stored")
	}
}

func TestHandleIndexFiltersCustomFieldValues(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if err := repo.SaveCustomField(ctx, "Kunde", true); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 {
		t.Fatalf("fields = %#v", fields)
	}
	acmeID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "acme-filter.pdf",
		StoredPath:   "2026/05/acme-filter.pdf",
		Title:        "ACME Filter",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "index-field-filter-acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, acmeID, "ACME Filter", "", nil, nil, map[int64]string{fields[0].ID: "ACME-4711"}); err != nil {
		t.Fatal(err)
	}
	betaID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "beta-filter.pdf",
		StoredPath:   "2026/05/beta-filter.pdf",
		Title:        "Beta Filter",
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "index-field-filter-beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, betaID, "Beta Filter", "", nil, nil, map[int64]string{fields[0].ID: "BETA-4711"}); err != nil {
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
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/?field_%d=ACME", fields[0].ID), nil)
	rec := httptest.NewRecorder()

	server.handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, fmt.Sprintf(`name="field_%d" value="ACME"`, fields[0].ID)) ||
		!strings.Contains(body, fmt.Sprintf(`data-custom-field-values-url="/fields/%d/values"`, fields[0].ID)) {
		t.Fatalf("custom field filter input missing: %s", body)
	}
	if !strings.Contains(body, "acme-filter.pdf") || strings.Contains(body, "beta-filter.pdf") {
		t.Fatalf("custom field filter result mismatch: %s", body)
	}
}

func TestHandleIndexHidesDetailTagsInListSummary(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.SaveTag(ctx, "Intern", "", "#176b87", false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveTag(ctx, "Sichtbar", "", "#2f855a", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "tags.pdf",
		StoredPath:   "2026/05/tags.pdf",
		Title:        "Tags",
		Tags:         []string{"intern", "sichtbar"},
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "list-hidden-tags",
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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-hide-list-tags="true"`) || !strings.Contains(body, ">sichtbar</span>") {
		t.Fatalf("visible list tag missing: %s", body)
	}
	if !strings.Contains(body, `class="tag tag-hidden-indicator" title="intern" aria-label="intern">...</span>`) {
		t.Fatalf("hidden tag indicator tooltip missing: %s", body)
	}
	if strings.Contains(body, `tag-hidden-indicator" title="intern" aria-label="intern">intern</span>`) {
		t.Fatalf("hidden tag rendered visibly instead of ellipsis: %s", body)
	}
}

func TestHandleIndexAppliesTagDisplayMode(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveSetting(ctx, tagDisplayModeSettingKey, tagDisplayModeUpper); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveTag(ctx, "Steuer", "", "#176b87", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "steuer.pdf",
		StoredPath:   "2026/05/steuer.pdf",
		Title:        "Steuer",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "tag-display-mode",
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
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.handleIndex(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-tag-display-mode="strtoupper"`) ||
		!strings.Contains(body, `data-name="steuer" data-display-name="STEUER"`) ||
		!strings.Contains(body, ">STEUER</span>") {
		t.Fatalf("tag display mode not applied: %s", body)
	}
}

func TestHandleCloudRendersCentralAndPrimaryLinks(t *testing.T) {
	t.Run("central", func(t *testing.T) {
		ctx := context.Background()
		repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()
		if err := repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "1"); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: "steuer.pdf",
			StoredPath:   "cloud/steuer.pdf",
			Title:        "Steuer",
			Tags:         []string{"steuer", "privat"},
			MIMEType:     "application/pdf",
			SizeBytes:    42,
			SHA256:       "cloud-handler-central",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: "steuer-zwei.pdf",
			StoredPath:   "cloud/steuer-zwei.pdf",
			Title:        "Steuer Zwei",
			Tags:         []string{"steuer"},
			MIMEType:     "application/pdf",
			SizeBytes:    42,
			SHA256:       "cloud-handler-central-two",
		}); err != nil {
			t.Fatal(err)
		}
		templates, err := parseTemplates()
		if err != nil {
			t.Fatal(err)
		}
		server := &Server{repo: repo, templates: templates, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
		req := httptest.NewRequest(http.MethodGet, "/cloud", nil)
		rec := httptest.NewRecorder()
		server.handleCloud(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "<h1>Wolke</h1>") ||
			!strings.Contains(body, `href="/documents?tags=steuer"`) ||
			!strings.Contains(body, `--cloud-size: 2.55rem`) ||
			!strings.Contains(body, `--cloud-size: 1.79rem`) ||
			!strings.Contains(body, `; --cloud-size: 2.55rem`) ||
			!strings.Contains(body, `--cloud-left:`) ||
			strings.Contains(body, "<small") ||
			strings.Contains(body, "tag-cloud-word-primary") {
			t.Fatalf("central cloud not rendered as expected: %s", body)
		}
	})

	t.Run("primary", func(t *testing.T) {
		ctx := context.Background()
		repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()
		if err := repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "1"); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.SaveTag(ctx, "Arbeit", "", "#176b87", false, false, false, true); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.SaveTag(ctx, "Projekt", "", "#2f855a", false, false, false, true); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: "arbeit.pdf",
			StoredPath:   "cloud/arbeit.pdf",
			Title:        "Arbeit",
			Tags:         []string{"arbeit", "kunde"},
			MIMEType:     "application/pdf",
			SizeBytes:    42,
			SHA256:       "cloud-handler-primary",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: "projekt-a.pdf",
			StoredPath:   "cloud/projekt-a.pdf",
			Title:        "Projekt A",
			Tags:         []string{"projekt", "kunde"},
			MIMEType:     "application/pdf",
			SizeBytes:    42,
			SHA256:       "cloud-handler-primary-projekt-a",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: "projekt-b.pdf",
			StoredPath:   "cloud/projekt-b.pdf",
			Title:        "Projekt B",
			Tags:         []string{"projekt", "angebot"},
			MIMEType:     "application/pdf",
			SizeBytes:    42,
			SHA256:       "cloud-handler-primary-projekt-b",
		}); err != nil {
			t.Fatal(err)
		}
		templates, err := parseTemplates()
		if err != nil {
			t.Fatal(err)
		}
		server := &Server{repo: repo, templates: templates, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
		req := httptest.NewRequest(http.MethodGet, "/cloud", nil)
		rec := httptest.NewRecorder()
		server.handleCloud(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, `tag-cloud-word-primary`) ||
			!strings.Contains(body, `tag-cloud-cluster-words`) ||
			!strings.Contains(body, `href="/documents?tags=arbeit"`) ||
			!strings.Contains(body, `href="/documents?tags=projekt"`) ||
			!strings.Contains(body, `href="/documents?tags=arbeit&amp;tags=kunde"`) ||
			strings.Count(body, `tag-cloud-word tag-cloud-word-primary`) != 2 ||
			strings.Count(body, `--cloud-size: 2.65rem`) != 2 ||
			!strings.Contains(body, `--cloud-size: 1.90rem`) ||
			strings.Contains(body, `--cloud-left:`) ||
			strings.Contains(body, "<small") {
			t.Fatalf("primary cloud not rendered as expected: %s", body)
		}
	})
}

func TestTagsListDoesNotRenderDeleteAction(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "1"); err != nil {
		t.Fatal(err)
	}
	tagID, err := repo.SaveTag(ctx, "Steuer", "", "#176b87", false, false, false, true)
	if err != nil {
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

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	rec := httptest.NewRecorder()
	server.handleTags(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tags status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, fmt.Sprintf(`/tags/%d/delete`, tagID)) || strings.Contains(body, `Tag steuer löschen?`) {
		t.Fatalf("tags list renders delete action: %s", body)
	}
	if !strings.Contains(body, `<span class="badge">Primär</span>`) {
		t.Fatalf("tags list does not render primary badge: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tags/%d", tagID), nil)
	req.SetPathValue("id", strconv.FormatInt(tagID, 10))
	rec = httptest.NewRecorder()
	server.handleTagDetail(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tag detail status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), fmt.Sprintf(`/tags/%d/delete`, tagID)) {
		t.Fatalf("tag detail does not render delete action: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="name" value="steuer" required`) {
		t.Fatalf("tag detail does not render rename input: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="primary_tag" value="1" checked`) {
		t.Fatalf("tag detail does not render primary checkbox checked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `<fieldset class="tag-options-fieldset">`) {
		t.Fatalf("tag detail does not render grouped tag options: %s", rec.Body.String())
	}
}

func TestPrimaryTagCheckboxHiddenWhenDocumentCloudDisabled(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	tagID, err := repo.SaveTag(ctx, "Steuer", "", "#176b87", false, false, false, true)
	if err != nil {
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

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	rec := httptest.NewRecorder()
	server.handleTags(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tags status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `name="primary_tag"`) {
		t.Fatalf("tags list renders primary checkbox while cloud disabled: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tags/%d", tagID), nil)
	req.SetPathValue("id", strconv.FormatInt(tagID, 10))
	rec = httptest.NewRecorder()
	server.handleTagDetail(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tag detail status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `name="primary_tag"`) {
		t.Fatalf("tag detail renders primary checkbox while cloud disabled: %s", rec.Body.String())
	}

	form := url.Values{"name": {"Steuer"}, "description": {"Unterlagen"}, "color": {"#2f855a"}}
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tags/%d", tagID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(tagID, 10))
	rec = httptest.NewRecorder()
	server.handleUpdateTag(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("update status = %d body = %s", rec.Code, rec.Body.String())
	}
	tag, err := repo.GetTag(ctx, tagID)
	if err != nil {
		t.Fatal(err)
	}
	if !tag.PrimaryTag {
		t.Fatalf("hidden primary tag checkbox cleared existing value: %#v", tag)
	}

	form = url.Values{"name": {"Neu"}, "color": {"#176b87"}, "primary_tag": {"1"}}
	req = httptest.NewRequest(http.MethodPost, "/tags", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	server.handleSaveTag(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d body = %s", rec.Code, rec.Body.String())
	}
	tag, err = repo.GetTagByName(ctx, "neu")
	if err != nil {
		t.Fatal(err)
	}
	if tag.PrimaryTag {
		t.Fatalf("disabled cloud accepted primary tag on create: %#v", tag)
	}
}

func TestHandleSaveAndUpdateTagPrimaryFlag(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveSetting(ctx, documentCloudEnabledSettingKey, "1"); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{"name": {"Steuer"}, "color": {"#176b87"}, "primary_tag": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/tags", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	server.handleSaveTag(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save status = %d body = %s", rec.Code, rec.Body.String())
	}
	tag, err := repo.GetTagByName(ctx, "steuer")
	if err != nil {
		t.Fatal(err)
	}
	if !tag.PrimaryTag {
		t.Fatalf("primary tag not saved: %#v", tag)
	}

	form = url.Values{"name": {"Steuer"}, "description": {"Unterlagen"}, "color": {"#2f855a"}}
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tags/%d", tag.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(tag.ID, 10))
	rec = httptest.NewRecorder()
	server.handleUpdateTag(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("update status = %d body = %s", rec.Code, rec.Body.String())
	}
	tag, err = repo.GetTagByName(ctx, "steuer")
	if err != nil {
		t.Fatal(err)
	}
	if tag.PrimaryTag {
		t.Fatalf("primary tag not cleared: %#v", tag)
	}
}

func TestHandleUpdateTagRenamesDocumentTag(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	tagID, err := repo.SaveTag(ctx, "Steuer", "Alt", "#176b87", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveTag(ctx, "Privat", "", "#2f855a", false, false); err != nil {
		t.Fatal(err)
	}
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "scan.pdf",
		StoredPath:   "rename/scan.pdf",
		Title:        "Dokument",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "server-tag-rename",
	})
	if err != nil {
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

	form := url.Values{"name": {"Abgabe"}, "description": {"Neu"}, "color": {"#2f855a"}}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tags/%d", tagID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(tagID, 10))
	rec := httptest.NewRecorder()
	server.handleUpdateTag(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("rename status = %d body = %s", rec.Code, rec.Body.String())
	}
	tag, err := repo.GetTag(ctx, tagID)
	if err != nil {
		t.Fatal(err)
	}
	if tag.Name != "abgabe" || tag.Description != "Neu" || tag.Color != "#2f855a" {
		t.Fatalf("renamed tag = %#v", tag)
	}
	doc, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	hasNew, hasOld := false, false
	for _, name := range doc.Tags {
		hasNew = hasNew || name == "abgabe"
		hasOld = hasOld || name == "steuer"
	}
	if !hasNew || hasOld {
		t.Fatalf("document tags = %#v", doc.Tags)
	}

	form = url.Values{"name": {"Privat"}, "description": {"Konflikt"}, "color": {"#176b87"}}
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tags/%d", tagID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(tagID, 10))
	rec = httptest.NewRecorder()
	server.handleUpdateTag(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("conflict status = %d body = %s", rec.Code, rec.Body.String())
	}
	tag, err = repo.GetTag(ctx, tagID)
	if err != nil {
		t.Fatal(err)
	}
	if tag.Name != "abgabe" {
		t.Fatalf("tag renamed after conflict: %#v", tag)
	}
}

func TestTagsListRendersSeparatePhotoTags(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.SaveTag(ctx, "Steuer", "", "#176b87", false, false); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "album"), 0o750); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	if _, err := photoLib.SetFolderTags("album", []string{"Urlaub"}); err != nil {
		t.Fatal(err)
	}

	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		photos:    photoLib,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	rec := httptest.NewRecorder()
	server.handleTags(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("tags status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{"1 Dokument-Tag", "1 Foto-Tag", "Foto-Tag anlegen", `class="search-favorite-row tag-row tag-list-row"`, `class="search-favorite-row tag-row photo-tag-row"`, "Einstellungen", "<summary>Bearbeiten</summary>", `/photos/tags/library`, `/photos/tags/library/rename`, `/photos/tags/library/delete`, `data-password-prompt=`, `name="color"`, "--tag-color: #176b87", "steuer", "urlaub", "1 Foto-Element"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("tags page misses %q: %s", expected, body)
		}
	}
	if strings.Contains(body, `/tags/0`) {
		t.Fatalf("photo tag renders as document tag action: %s", body)
	}
}

func TestSearchFavoritesRenderInManagementAndDocumentFilters(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", true); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 {
		t.Fatalf("fields = %#v", fields)
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name:  "Rechnungen",
		Query: "rechnung",
		Tags:  []string{"steuer"},
		CustomFields: []document.CustomFieldFilter{
			{FieldID: fields[0].ID, Value: "ACME"},
		},
		DateMode: document.SearchFavoriteDateThisYear,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{
		Name:     "Steuer 2026",
		Tags:     []string{"steuer"},
		DateMode: document.SearchFavoriteDateYear,
		DateYear: 2026,
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

	req := httptest.NewRequest(http.MethodGet, "/search-favorites", nil)
	rec := httptest.NewRecorder()
	server.handleSearchFavorites(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("management status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `Suchfavoriten`) || !strings.Contains(body, `action="/search-favorites/`) || !strings.Contains(body, `Anwenden`) {
		t.Fatalf("management body missing favorite controls: %s", body)
	}
	if !strings.Contains(body, fmt.Sprintf(`name="field_%d" value="ACME"`, fields[0].ID)) ||
		!strings.Contains(body, fmt.Sprintf(`data-custom-field-values-url="/fields/%d/values"`, fields[0].ID)) {
		t.Fatalf("management body missing custom field filters: %s", body)
	}
	if !strings.Contains(body, `Letzte 30 Tage`) || !strings.Contains(body, `Dieser Monat`) || !strings.Contains(body, `Dieses Halbjahr`) {
		t.Fatalf("management body missing dynamic date options: %s", body)
	}
	if got := strings.Count(body, `data-search-favorite-year-field hidden`); got != 2 {
		t.Fatalf("fixed year field hidden count = %d body = %s", got, body)
	}
	if got := strings.Count(body, `data-search-favorite-year-field >`); got != 1 || !strings.Contains(body, `value="2026"`) {
		t.Fatalf("fixed year field visible state missing: count=%d body=%s", got, body)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	server.handleIndex(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("index status = %d body = %s", rec.Code, rec.Body.String())
	}
	body = rec.Body.String()
	if !strings.Contains(body, `class="search-favorite-menu"`) || !strings.Contains(body, `Suchfavoriten`) || !strings.Contains(body, `Rechnungen`) {
		t.Fatalf("index body missing favorite menu: %s", body)
	}
	if !strings.Contains(body, fmt.Sprintf(`field_%d=ACME`, fields[0].ID)) || !strings.Contains(body, "Felder: Kunde: ACME") {
		t.Fatalf("index body missing favorite custom field filter: %s", body)
	}
}

func TestSearchFavoriteValidationErrorReturnsToManagement(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{"name": {"Leer"}, "date_mode": {document.SearchFavoriteDateNone}}
	req := httptest.NewRequest(http.MethodPost, "/search-favorites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	server.handleSaveSearchFavorite(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "mindestens Suchwort, Tag oder Zeitraum auswählen") ||
		!strings.Contains(body, `href="/search-favorites"`) ||
		!strings.Contains(body, `>Zurück</a>`) {
		t.Fatalf("validation error missing search favorite return link: %s", body)
	}
}

func TestValidationErrorReturnLinks(t *testing.T) {
	cases := []struct {
		name        string
		wantStatus  int
		wantMessage string
		wantHref    string
		wantHrefFn  func(t *testing.T, repo *repository.Repository) string
		run         func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder
	}{
		{
			name:        "field create",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Feldname fehlt",
			wantHref:    "/fields",
			run: func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder {
				req := newFormTestRequest(http.MethodPost, "/fields", url.Values{"label": {""}})
				rec := httptest.NewRecorder()
				server.handleSaveField(rec, req)
				return rec
			},
		},
		{
			name:        "field value",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "alter und neuer Wert werden benötigt",
			wantHrefFn: func(t *testing.T, repo *repository.Repository) string {
				fields, err := repo.ListCustomFields(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				return fieldValuesReturnURL(fields[0].ID)
			},
			run: func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder {
				if err := repo.SaveCustomField(context.Background(), "Kunde", false); err != nil {
					t.Fatal(err)
				}
				fields, err := repo.ListCustomFields(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				fieldID := fields[0].ID
				req := newFormTestRequest(http.MethodPost, fmt.Sprintf("/fields/%d/values", fieldID), url.Values{
					"old_value": {"ACME"},
				})
				req.SetPathValue("id", strconv.FormatInt(fieldID, 10))
				rec := httptest.NewRecorder()
				server.handleUpdateFieldValue(rec, req)
				return rec
			},
		},
		{
			name:        "document tag create",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Tagname fehlt",
			wantHref:    "/tags",
			run: func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder {
				req := newFormTestRequest(http.MethodPost, "/tags", url.Values{"name": {""}})
				rec := httptest.NewRecorder()
				server.handleSaveTag(rec, req)
				return rec
			},
		},
		{
			name:        "document tag rules",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "ungültige Regel-ID",
			wantHrefFn: func(t *testing.T, repo *repository.Repository) string {
				tags, err := repo.ListTags(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				return tagDetailReturnURL(tags[0].ID)
			},
			run: func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder {
				tagID, err := repo.SaveTag(context.Background(), "Steuer", "", "#176b87", false, false)
				if err != nil {
					t.Fatal(err)
				}
				req := newFormTestRequest(http.MethodPost, fmt.Sprintf("/tags/%d/rules", tagID), url.Values{
					"delete_rule": {"abc"},
				})
				req.SetPathValue("id", strconv.FormatInt(tagID, 10))
				rec := httptest.NewRecorder()
				server.handleSaveTagRules(rec, req)
				return rec
			},
		},
		{
			name:        "document bulk tags",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "mindestens einen Tag auswählen",
			wantHref:    "/documents?q=steuer",
			run: func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder {
				req := newFormTestRequest(http.MethodPost, "/documents/tags/add", url.Values{
					"ids":    {"1"},
					"return": {"/documents?q=steuer"},
				})
				rec := httptest.NewRecorder()
				server.handleAddDocumentTags(rec, req)
				return rec
			},
		},
		{
			name:        "page size",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "ungültige Anzahl pro Seite",
			wantHref:    "/documents?q=steuer",
			run: func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder {
				req := newFormTestRequest(http.MethodPost, "/settings/page-size", url.Values{
					"page_size": {"abc"},
					"return":    {"/documents?q=steuer"},
				})
				rec := httptest.NewRecorder()
				server.handleSavePageSize(rec, req)
				return rec
			},
		},
		{
			name:        "export",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "keine Dokumente ausgewählt",
			wantHref:    "/documents?q=steuer",
			run: func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodGet, "/export?return=%2Fdocuments%3Fq%3Dsteuer", nil)
				rec := httptest.NewRecorder()
				server.handleExport(rec, req)
				return rec
			},
		},
		{
			name:        "metadata date",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "ungültiges Dokumentdatum",
			wantHrefFn: func(t *testing.T, repo *repository.Repository) string {
				docs, err := repo.ListDocuments(context.Background(), document.ListFilter{})
				if err != nil {
					t.Fatal(err)
				}
				return documentURL(docs[0].ID, "/documents?q=steuer", "")
			},
			run: func(t *testing.T, server *Server, repo *repository.Repository) *httptest.ResponseRecorder {
				docID, err := repo.CreateDocument(context.Background(), document.Document{
					OriginalName: "meta.pdf",
					StoredPath:   "meta.pdf",
					Title:        "Meta",
					MIMEType:     "application/pdf",
					SizeBytes:    1,
					SHA256:       "validation-return-meta",
				})
				if err != nil {
					t.Fatal(err)
				}
				req := newFormTestRequest(http.MethodPost, fmt.Sprintf("/documents/%d/metadata", docID), url.Values{
					"title":         {"Meta"},
					"document_date": {"kaputt"},
					"return":        {"/documents?q=steuer"},
				})
				req.SetPathValue("id", strconv.FormatInt(docID, 10))
				rec := httptest.NewRecorder()
				server.handleMetadata(rec, req)
				return rec
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, repo := newErrorReturnTestServer(t)
			rec := tc.run(t, server, repo)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			wantHref := tc.wantHref
			if tc.wantHrefFn != nil {
				wantHref = tc.wantHrefFn(t, repo)
			}
			assertErrorReturnLink(t, rec.Body.String(), tc.wantMessage, wantHref)
		})
	}
}

func TestPhotoValidationErrorReturnLinks(t *testing.T) {
	server, _ := newErrorReturnTestServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "public.jpg"), []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	photoLib, err := photos.New(root, filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer photoLib.Close()
	server.photos = photoLib

	req := newFormTestRequest(http.MethodPost, "/photos/tags/library", url.Values{"name": {""}})
	rec := httptest.NewRecorder()
	server.handleSavePhotoTag(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	assertErrorReturnLink(t, rec.Body.String(), "Foto-Tagname fehlt", "/tags?tab=photos")

	req = newFormTestRequest(http.MethodPost, "/photos/tags/add", url.Values{
		"ids":    {"public.jpg"},
		"tags":   {" "},
		"return": {"/photos?path=album"},
	})
	rec = httptest.NewRecorder()
	server.handleAddPhotoTags(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bulk status = %d body = %s", rec.Code, rec.Body.String())
	}
	assertErrorReturnLink(t, rec.Body.String(), "mindestens einen Tag auswählen", "/photos?path=album")
}

func newErrorReturnTestServer(t *testing.T) (*Server, *repository.Repository) {
	t.Helper()
	repo, err := repository.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Fatalf("close repo: %v", err)
		}
	})
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return server, repo
}

func newFormTestRequest(method, target string, form url.Values) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func assertErrorReturnLink(t *testing.T, body, wantMessage, wantHref string) {
	t.Helper()
	if !strings.Contains(body, wantMessage) ||
		!strings.Contains(body, `href="`+wantHref+`"`) ||
		!strings.Contains(body, `>Zurück</a>`) {
		t.Fatalf("error body missing message %q or return link %q: %s", wantMessage, wantHref, body)
	}
}

func TestCountDocumentsCacheIgnoresPagingAndInvalidates(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung-1.pdf",
		StoredPath:   "2026/05/rechnung-1.pdf",
		Title:        "Rechnung 1",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "count-cache-1",
	}); err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung-2.pdf",
		StoredPath:   "2026/05/rechnung-2.pdf",
		Title:        "Rechnung 2",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "count-cache-2",
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{repo: repo}
	filter := document.ListFilter{Query: "rechnung", Sort: "name", Direction: "asc", Limit: 25, Page: 1}
	total, err := server.countDocuments(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}

	filter.Page = 2
	filter.Offset = 25
	filter.Sort = "size"
	total, err = server.countDocuments(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("cached total = %d, want 2", total)
	}
	if size := server.apps.documents.counts.countSize(); size != 1 {
		t.Fatalf("cache size = %d, want 1", size)
	}

	if err := repo.SoftDelete(ctx, secondID); err != nil {
		t.Fatal(err)
	}
	server.invalidateDocumentCountCache()
	total, err = server.countDocuments(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("total after invalidation = %d, want 1", total)
	}
}

func TestDocumentCountKeyIncludesCustomFieldFilters(t *testing.T) {
	first := documentCountKey(document.ListFilter{
		CustomFields: []document.CustomFieldFilter{{FieldID: 7, Value: "ACME"}},
	})
	second := documentCountKey(document.ListFilter{
		CustomFields: []document.CustomFieldFilter{{FieldID: 7, Value: "Beta"}},
	})
	if first == second {
		t.Fatalf("custom field filters are missing from count key: %#v", first)
	}

	reordered := documentCountKey(document.ListFilter{
		CustomFields: []document.CustomFieldFilter{
			{FieldID: 9, Value: "Umbau"},
			{FieldID: 7, Value: "ACME"},
		},
	})
	ordered := documentCountKey(document.ListFilter{
		CustomFields: []document.CustomFieldFilter{
			{FieldID: 7, Value: "acme"},
			{FieldID: 9, Value: "Umbau"},
		},
	})
	if reordered != ordered {
		t.Fatalf("custom field count key should be order-insensitive: %#v != %#v", reordered, ordered)
	}

	exact := documentCountKey(document.ListFilter{
		CustomFields: []document.CustomFieldFilter{{FieldID: 7, Value: "ACME", Exact: true}},
	})
	if exact == first {
		t.Fatalf("custom field count key should separate exact and partial filters: %#v", exact)
	}
}

func TestHandleTrashRendersMobileTableColumns(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "rechnung.pdf",
		StoredPath:    "2026/05/rechnung.pdf",
		Title:         "Rechnung",
		MIMEType:      "application/pdf",
		SizeBytes:     42,
		SHA256:        "trash-mobile-table",
		SearchVersion: document.CurrentSearchVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SoftDelete(ctx, docID); err != nil {
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
	req := httptest.NewRequest(http.MethodGet, "/trash", nil)
	rec := httptest.NewRecorder()

	server.handleTrash(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`class="document-table trash-table"`,
		`data-column="name" data-label="Name"`,
		`data-column="deleted_at" data-label="Gelöscht"`,
		`data-column="actions" data-label="Aktionen"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("trash table is missing %q in body %s", want, body)
		}
	}
}

func TestHandleAPITags(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.SaveTag(ctx, "Steuer", "Unterlagen", "#2f855a", false, true, false, true); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	rec := httptest.NewRecorder()

	server.handleAPITags(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Tags []struct {
			Name       string `json:"name"`
			PrimaryTag bool   `json:"primary_tag"`
			ListHidden bool   `json:"list_hidden"`
			Count      int    `json:"count"`
		} `json:"tags"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Tags) != 1 || payload.Tags[0].Name != "steuer" || !payload.Tags[0].PrimaryTag || !payload.Tags[0].ListHidden || payload.Tags[0].Count != 0 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleDocumentHidesDeleteForProtectedDocument(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.SaveTag(ctx, "Archiv", "", "#176b87", false, false, true); err != nil {
		t.Fatal(err)
	}
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "schutz.pdf",
		StoredPath:   "2026/05/schutz.pdf",
		Title:        "Schutz",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "delete-protected",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, docID, "Schutz", "", nil, []string{"archiv"}, nil); err != nil {
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

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/documents/%d", docID), nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handleDocument(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, fmt.Sprintf(`/documents/%d/delete`, docID)) {
		t.Fatalf("protected document renders delete action: %s", body)
	}
	if !strings.Contains(body, "Löschschutz") {
		t.Fatalf("protected document does not show protection state: %s", body)
	}

	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%d/delete", docID), nil)
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec = httptest.NewRecorder()

	server.handleDelete(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteTagRequiresPassword(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	tagID, err := repo.SaveTag(ctx, "Steuer", "Unterlagen", "#2f855a", false, false)
	if err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:       config.Config{Auth: config.AuthConfig{Username: "admin", Password: "secret"}},
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{"password": {"wrong"}}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tags/%d/delete", tagID), strings.NewReader(form.Encode()))
	req = authenticatedTestRequest(server, req, "admin")
	req.SetPathValue("id", strconv.FormatInt(tagID, 10))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleDeleteTag(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if _, err := repo.GetTag(ctx, tagID); err != nil {
		t.Fatalf("tag should still exist: %v", err)
	}

	form = url.Values{"password": {"secret"}}
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tags/%d/delete", tagID), strings.NewReader(form.Encode()))
	req = authenticatedTestRequest(server, req, "admin")
	req.SetPathValue("id", strconv.FormatInt(tagID, 10))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()

	server.handleDeleteTag(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if _, err := repo.GetTag(ctx, tagID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("tag err = %v", err)
	}
}

func TestHandleAPIFields(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kundennummer", true); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/fields", nil)
	rec := httptest.NewRecorder()

	server.handleAPIFields(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Fields []struct {
			ID                  int64  `json:"id"`
			Label               string `json:"label"`
			Position            int    `json:"position"`
			AutocompleteEnabled bool   `json:"autocomplete_enabled"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Fields) != 1 || payload.Fields[0].Label != "Kundennummer" || !payload.Fields[0].AutocompleteEnabled {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleFieldValueSuggestions(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", true); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, value := range []string{"ACME", "ACME", "Beta"} {
		docID, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: fmt.Sprintf("doc-%d.pdf", i),
			StoredPath:   fmt.Sprintf("doc-%d.pdf", i),
			Title:        "Dokument",
			MIMEType:     "application/pdf",
			SizeBytes:    1,
			SHA256:       fmt.Sprintf("field-suggestion-%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMetadata(ctx, docID, "Dokument", "", nil, nil, map[int64]string{fields[0].ID: value}); err != nil {
			t.Fatal(err)
		}
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/fields/%d/values?q=ac", fields[0].ID), nil)
	req.Header.Set("Accept", "application/json")
	req.SetPathValue("id", strconv.FormatInt(fields[0].ID, 10))
	rec := httptest.NewRecorder()

	server.handleFieldValueSuggestions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Values []string `json:"values"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Values) != 1 || payload.Values[0] != "ACME" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestHandleUpdateFieldValueRenamesAcrossDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", true); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		docID, err := repo.CreateDocument(ctx, document.Document{
			OriginalName: fmt.Sprintf("field-value-%d.pdf", i),
			StoredPath:   fmt.Sprintf("field-value-%d.pdf", i),
			Title:        "Dokument",
			MIMEType:     "application/pdf",
			SizeBytes:    1,
			SHA256:       fmt.Sprintf("field-value-rename-%d", i),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateMetadata(ctx, docID, "Dokument", "", nil, nil, map[int64]string{fields[0].ID: "ACME"}); err != nil {
			t.Fatal(err)
		}
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := strings.NewReader(url.Values{
		"old_value": {"ACME"},
		"new_value": {"ACME AG"},
	}.Encode())
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/fields/%d/values", fields[0].ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(fields[0].ID, 10))
	rec := httptest.NewRecorder()

	server.handleUpdateFieldValue(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, fmt.Sprintf("/fields/%d/values", fields[0].ID)) {
		t.Fatalf("location = %q", location)
	}
	values, err := repo.CustomFieldValues(ctx, fields[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Value != "ACME AG" || values[0].Count != 2 {
		t.Fatalf("values = %#v", values)
	}
}

func TestHandleFieldAutocompleteToggle(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/fields/%d/autocomplete", fields[0].ID), strings.NewReader("autocomplete_enabled=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(fields[0].ID, 10))
	rec := httptest.NewRecorder()

	server.handleFieldAutocomplete(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	fields, err = repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !fields[0].AutocompleteEnabled {
		t.Fatalf("autocomplete was not enabled: %#v", fields)
	}
}

func TestHandleUpdateField(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	body := strings.NewReader(url.Values{
		"label":                {"Kunden-ID"},
		"autocomplete_enabled": {"1"},
	}.Encode())
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/fields/%d", fields[0].ID), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(fields[0].ID, 10))
	rec := httptest.NewRecorder()

	server.handleUpdateField(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	fields, err = repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if fields[0].Label != "Kunden-ID" || !fields[0].AutocompleteEnabled {
		t.Fatalf("field = %#v", fields[0])
	}
}

func TestHandleDeleteFieldRequiresPassword(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fieldID := fields[0].ID
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:       config.Config{Auth: config.AuthConfig{Username: "admin", Password: "secret"}},
		repo:      repo,
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{"password": {"wrong"}}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/fields/%d/delete", fieldID), strings.NewReader(form.Encode()))
	req = authenticatedTestRequest(server, req, "admin")
	req.SetPathValue("id", strconv.FormatInt(fieldID, 10))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleDeleteField(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	fields, err = repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 {
		t.Fatalf("field should still exist: %#v", fields)
	}

	form = url.Values{"password": {"secret"}}
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/fields/%d/delete", fieldID), strings.NewReader(form.Encode()))
	req = authenticatedTestRequest(server, req, "admin")
	req.SetPathValue("id", strconv.FormatInt(fieldID, 10))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()

	server.handleDeleteField(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	fields, err = repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 0 {
		t.Fatalf("field should be deleted: %#v", fields)
	}
}

func TestHandleHelpRendersDocumentation(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()

	server.handleHelp(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Gliederung", "Regelsets", "Gruppenmodus", "Detailtags", "Auto-Vervollständigung", "Fotos", "Fotoframe", "tag:urlaub", ".adminonly", "Fotoindex", "Backup", "auth-session.key", "tar -czf bearstack-backup.tgz data", "WebDAV", "Löschschutz", "/webdav/"} {
		if !strings.Contains(body, want) {
			t.Fatalf("help page missing %q in body %s", want, body)
		}
	}
}

func TestSearchFavoriteURLDynamicQuarter(t *testing.T) {
	now := time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC)
	target := searchFavoriteURL(document.SearchFavorite{
		Query:    "rechnung",
		Tags:     []string{"steuer", "offen"},
		DateMode: document.SearchFavoriteDateLastQuarter,
	}, now)
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if parsed.Path != "/" || q.Get("q") != "rechnung" || q.Get("from") != "2026-01-01" || q.Get("to") != "2026-03-31" {
		t.Fatalf("url = %s", target)
	}
	if got := q["tags"]; strings.Join(got, ",") != "steuer,offen" {
		t.Fatalf("tags = %#v", got)
	}
}

func TestSearchFavoriteURLAdditionalDynamicRanges(t *testing.T) {
	now := time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		mode string
		from string
		to   string
	}{
		{name: "this month", mode: document.SearchFavoriteDateThisMonth, from: "2026-05-01", to: "2026-05-31"},
		{name: "last month", mode: document.SearchFavoriteDateLastMonth, from: "2026-04-01", to: "2026-04-30"},
		{name: "this half year", mode: document.SearchFavoriteDateThisHalf, from: "2026-01-01", to: "2026-06-30"},
		{name: "last half year", mode: document.SearchFavoriteDateLastHalf, from: "2025-07-01", to: "2025-12-31"},
		{name: "last seven days", mode: document.SearchFavoriteDateLast7Days, from: "2026-05-01", to: "2026-05-07"},
		{name: "last thirty days", mode: document.SearchFavoriteDateLast30Days, from: "2026-04-08", to: "2026-05-07"},
		{name: "last ninety days", mode: document.SearchFavoriteDateLast90Days, from: "2026-02-07", to: "2026-05-07"},
		{name: "last year as days", mode: document.SearchFavoriteDateLast365Days, from: "2025-05-08", to: "2026-05-07"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := searchFavoriteURL(document.SearchFavorite{DateMode: tc.mode}, now)
			parsed, err := url.Parse(target)
			if err != nil {
				t.Fatal(err)
			}
			q := parsed.Query()
			if q.Get("from") != tc.from || q.Get("to") != tc.to {
				t.Fatalf("url = %s", target)
			}
		})
	}
}

func TestSearchFavoriteURLFixedYear(t *testing.T) {
	target := searchFavoriteURL(document.SearchFavorite{
		Tags:     []string{"archiv"},
		DateMode: document.SearchFavoriteDateYear,
		DateYear: 2025,
	}, time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC))
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("year") != "2025" || q.Get("from") != "" || q.Get("to") != "" {
		t.Fatalf("url = %s", target)
	}
}

func TestSearchFavoriteItemsCountsInBatch(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{Name: "Steuer", Tags: []string{"steuer"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSearchFavorite(ctx, document.SearchFavorite{Name: "Privat", Tags: []string{"privat"}}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc := folderApplicationService{
		repo: repo,
		countDocumentFilters: func(_ context.Context, filters []document.ListFilter) ([]int, error) {
			calls++
			if len(filters) != 2 {
				t.Fatalf("filters = %#v", filters)
			}
			return []int{3, 4}, nil
		},
	}

	items, err := svc.SearchFavoriteItems(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("batch count calls = %d, want 1", calls)
	}
	if len(items) != 2 || items[0].Count != 3 || items[1].Count != 4 {
		t.Fatalf("items = %#v", items)
	}
}

func TestSearchFavoriteCustomFieldFilters(t *testing.T) {
	favorite := document.SearchFavorite{
		Query: "rechnung",
		CustomFields: []document.CustomFieldFilter{
			{FieldID: 7, Value: " ACME  4711 "},
		},
	}
	target := searchFavoriteURL(favorite, time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC))
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query().Get("field_7"); got != "ACME 4711" {
		t.Fatalf("field filter URL value = %q in %s", got, target)
	}

	filter := searchFavoriteFilter(favorite, time.Date(2026, time.May, 7, 10, 0, 0, 0, time.UTC), 2, 25)
	if filter.Page != 2 || filter.Limit != 25 || filter.Offset != 25 {
		t.Fatalf("pagination filter = %#v", filter)
	}
	if len(filter.CustomFields) != 1 || filter.CustomFields[0].FieldID != 7 || filter.CustomFields[0].Value != " ACME  4711 " {
		t.Fatalf("custom field filter = %#v", filter.CustomFields)
	}
}

func TestHandleDocumentDateUpdatesOnlyDocumentDate(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kundennummer", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}

	oldDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung.pdf",
		StoredPath:   "2026/03/rechnung.pdf",
		Title:        "Rechnung",
		Description:  "Bezahlt",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "server-doc-date",
		DocumentDate: &oldDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, docID, "Rechnung", "Bezahlt", &oldDate, []string{"steuer", "archiv"}, map[int64]string{fields[0].ID: "ACME-42"}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{"document_date": {"2026-04-10"}}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%d/document-date", docID), strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	rec := httptest.NewRecorder()

	server.handleDocumentDate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		DocumentDate      string `json:"document_date"`
		DocumentDateInput string `json:"document_date_input"`
		Notice            string `json:"notice"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.DocumentDate != "10.04.2026" || payload.DocumentDateInput != "2026-04-10" {
		t.Fatalf("payload = %#v", payload)
	}

	updated, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	newDate := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	if updated.DocumentDate == nil || !updated.DocumentDate.Equal(newDate) {
		t.Fatalf("document date = %#v", updated.DocumentDate)
	}
	if updated.Title != "Rechnung" || updated.Description != "Bezahlt" || updated.CustomValues[fields[0].ID] != "ACME-42" {
		t.Fatalf("metadata changed = %#v", updated)
	}
	tagSet := map[string]bool{}
	for _, tag := range updated.Tags {
		tagSet[tag] = true
	}
	if len(updated.Tags) != 2 || !tagSet["steuer"] || !tagSet["archiv"] {
		t.Fatalf("tags changed = %#v", updated.Tags)
	}
}

func TestDocumentEditorCannotCreateUnknownTagsFromDocumentHandlers(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if _, err := repo.SaveTag(ctx, "Archiv", "", "#176b87", false, false); err != nil {
		t.Fatal(err)
	}
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "rechnung.pdf",
		StoredPath:   "2026/05/rechnung.pdf",
		Title:        "Rechnung",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "server-editor-tags",
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
			{Username: "editor", Password: "secret", Role: "documents_editor"},
		}}},
		repo:    repo,
		authKey: []byte("01234567890123456789012345678901"),
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{"tags": {"neu"}}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%d/tags", docID), strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	req = authenticatedTestRequest(server, req, "editor")
	rec := httptest.NewRecorder()
	server.handleDocumentTags(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tag update status = %d body = %s", rec.Code, rec.Body.String())
	}

	form = url.Values{"title": {"Rechnung neu"}, "tags": {"neu"}}
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%d/metadata", docID), strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	req = authenticatedTestRequest(server, req, "editor")
	rec = httptest.NewRecorder()
	server.handleMetadata(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("metadata status = %d body = %s", rec.Code, rec.Body.String())
	}

	form = url.Values{"tags": {"archiv"}}
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%d/tags", docID), strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(docID, 10))
	req = authenticatedTestRequest(server, req, "editor")
	rec = httptest.NewRecorder()
	server.handleDocumentTags(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("existing tag status = %d body = %s", rec.Code, rec.Body.String())
	}

	if _, err := repo.GetTagByName(ctx, "neu"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown tag lookup err = %v", err)
	}
	updated, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Rechnung" || !hasTags(updated.Tags, "archiv") {
		t.Fatalf("updated document = %#v", updated)
	}
}

func TestHandleBulkDocumentFieldsSetsCustomValues(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveCustomField(ctx, "Kunde", true); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveCustomField(ctx, "Projekt", false); err != nil {
		t.Fatal(err)
	}
	fields, err := repo.ListCustomFields(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "a.pdf",
		StoredPath:   "a.pdf",
		Title:        "A",
		Tags:         []string{"steuer"},
		MIMEType:     "application/pdf",
		SizeBytes:    42,
		SHA256:       "bulk-fields-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "b.pdf",
		StoredPath:   "b.pdf",
		Title:        "B",
		MIMEType:     "application/pdf",
		SizeBytes:    43,
		SHA256:       "bulk-fields-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateMetadata(ctx, firstID, "A", "", nil, []string{"steuer"}, map[int64]string{fields[1].ID: "Alt"}); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"ids":                              {strconv.FormatInt(firstID, 10), strconv.FormatInt(secondID, 10)},
		"return":                           {"/"},
		customFieldInputName(fields[0].ID): {"ACME"},
	}
	req := httptest.NewRequest(http.MethodPost, "/documents/fields", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleBulkDocumentFields(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	first, err := repo.GetDocument(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetDocument(ctx, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if first.CustomValues[fields[0].ID] != "ACME" || first.CustomValues[fields[1].ID] != "Alt" {
		t.Fatalf("first custom values = %#v", first.CustomValues)
	}
	if second.CustomValues[fields[0].ID] != "ACME" {
		t.Fatalf("second custom values = %#v", second.CustomValues)
	}
}

func TestHandleLinkDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	firstID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "first.pdf",
		StoredPath:   "2024/01/first.pdf",
		Title:        "First",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "server-link-first",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "second.pdf",
		StoredPath:   "2024/01/second.pdf",
		Title:        "Second",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "server-link-second",
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"ids":    {strconv.FormatInt(firstID, 10), strconv.FormatInt(secondID, 10)},
		"return": {"/?q=abc"},
	}
	req := httptest.NewRequest(http.MethodPost, "/documents/link", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.handleLinkDocuments(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/?notice=2+Dokumente+verkn%C3%BCpft.&q=abc" {
		t.Fatalf("location = %q", location)
	}
	links, err := repo.LinkedDocuments(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].ID != secondID {
		t.Fatalf("links = %#v", links)
	}

	unlinkForm := url.Values{"return": {"/?q=abc"}}
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%d/links/%d/delete", firstID, secondID), strings.NewReader(unlinkForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(firstID, 10))
	req.SetPathValue("linkedID", strconv.FormatInt(secondID, 10))
	rec = httptest.NewRecorder()

	server.handleUnlinkDocument(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("unlink status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != documentURL(firstID, "/?q=abc", "Verknüpfung aufgehoben.") {
		t.Fatalf("unlink location = %q", location)
	}
	links, err = repo.LinkedDocuments(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 0 {
		t.Fatalf("links after unlink = %#v", links)
	}
}

func TestHandleAddDocumentTagsAppendsToSelectedDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	firstID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "first.pdf",
		StoredPath:   "2024/01/first.pdf",
		Title:        "First",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "server-bulk-tags-first",
		Tags:         []string{"steuer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "second.pdf",
		StoredPath:   "2024/01/second.pdf",
		Title:        "Second",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "server-bulk-tags-second",
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"ids":  {strconv.FormatInt(firstID, 10), strconv.FormatInt(secondID, 10)},
		"tags": {"archiv"},
	}
	req := httptest.NewRequest(http.MethodPost, "/documents/tags/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	server.handleAddDocumentTags(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Updated int      `json:"updated"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Updated != 2 || len(payload.Tags) != 1 || payload.Tags[0] != "archiv" {
		t.Fatalf("payload = %#v", payload)
	}

	first, err := repo.GetDocument(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetDocument(ctx, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTags(first.Tags, "steuer", "archiv") {
		t.Fatalf("first tags = %#v", first.Tags)
	}
	if !hasTags(second.Tags, "archiv") {
		t.Fatalf("second tags = %#v", second.Tags)
	}
}

func TestHandleAddDocumentTagsRejectsUnknownTagsForDocumentEditor(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "first.pdf",
		StoredPath:   "2024/01/editor-first.pdf",
		Title:        "First",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "server-editor-bulk-tags",
		Tags:         []string{"steuer"},
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
			{Username: "editor", Password: "secret", Role: "documents_editor"},
		}}},
		repo:    repo,
		authKey: []byte("01234567890123456789012345678901"),
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"ids":  {strconv.FormatInt(docID, 10)},
		"tags": {"neu"},
	}
	req := httptest.NewRequest(http.MethodPost, "/documents/tags/add", strings.NewReader(form.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = authenticatedTestRequest(server, req, "editor")
	rec := httptest.NewRecorder()

	server.handleAddDocumentTags(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if _, err := repo.GetTagByName(ctx, "neu"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown tag lookup err = %v", err)
	}
	updated, err := repo.GetDocument(ctx, docID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTags(updated.Tags, "steuer") {
		t.Fatalf("tags = %#v", updated.Tags)
	}
}

func TestHandleRemoveDocumentTagsRemovesFromSelectedDocuments(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	firstID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "first.pdf",
		StoredPath:   "2024/01/remove-first.pdf",
		Title:        "First",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "server-remove-tags-first",
		Tags:         []string{"steuer", "archiv"},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "second.pdf",
		StoredPath:   "2024/01/remove-second.pdf",
		Title:        "Second",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "server-remove-tags-second",
		Tags:         []string{"archiv"},
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"ids":  {strconv.FormatInt(firstID, 10), strconv.FormatInt(secondID, 10)},
		"tags": {"archiv"},
	}
	req := httptest.NewRequest(http.MethodPost, "/documents/tags/remove", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	server.handleRemoveDocumentTags(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Updated int      `json:"updated"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Updated != 2 || len(payload.Tags) != 1 || payload.Tags[0] != "archiv" {
		t.Fatalf("payload = %#v", payload)
	}

	first, err := repo.GetDocument(ctx, firstID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.GetDocument(ctx, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasTags(first.Tags, "steuer") {
		t.Fatalf("first tags = %#v", first.Tags)
	}
	if !hasTags(second.Tags) {
		t.Fatalf("second tags = %#v", second.Tags)
	}
}

func hasTags(values []string, expected ...string) bool {
	if len(values) != len(expected) {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range expected {
		if !seen[value] {
			return false
		}
	}
	return true
}

func TestHandleDocumentShowsGroupedDocumentsWithoutUnlinkAction(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	if _, err := repo.SaveTag(ctx, "Projekt", "", "#176b87", true, false); err != nil {
		t.Fatal(err)
	}

	mainID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "main.pdf",
		StoredPath:   "2026/05/main.pdf",
		Title:        "Main",
		Tags:         []string{"projekt"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "detail-group-main",
	})
	if err != nil {
		t.Fatal(err)
	}
	linkedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "linked.pdf",
		StoredPath:   "2026/05/linked.pdf",
		Title:        "Linked",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "detail-group-linked",
	})
	if err != nil {
		t.Fatal(err)
	}
	groupedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "grouped.pdf",
		StoredPath:   "2026/05/grouped.pdf",
		Title:        "Grouped",
		Tags:         []string{"projekt"},
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "detail-group-grouped",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.LinkDocuments(ctx, []int64{mainID, linkedID}); err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: templates,
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/documents/%d", mainID), nil)
	req.SetPathValue("id", strconv.FormatInt(mainID, 10))
	rec := httptest.NewRecorder()

	server.handleDocument(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Verknüpfte und gruppierte Dateien") || !strings.Contains(body, "linked.pdf") || !strings.Contains(body, "grouped.pdf") {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, ">Gruppiert<") {
		t.Fatalf("generic grouped badge still shown: %s", body)
	}
	if !strings.Contains(body, `class="tag grouped-tag"`) || !strings.Contains(body, ">projekt</span>") {
		t.Fatalf("group tag pill missing: %s", body)
	}
	if !strings.Contains(body, fmt.Sprintf("/documents/%d/links/%d/delete", mainID, linkedID)) {
		t.Fatalf("linked unlink action missing: %s", body)
	}
	if strings.Contains(body, fmt.Sprintf("/documents/%d/links/%d/delete", mainID, groupedID)) {
		t.Fatalf("grouped document has unlink action: %s", body)
	}
}

func TestExportMetadataPayloadIncludesLinkedDocuments(t *testing.T) {
	uploadedAt := time.Date(2026, 5, 5, 9, 30, 0, 0, time.UTC)

	raw, err := exportMetadataPayload([]document.Document{{
		ID:           1,
		OriginalName: "haupt.pdf",
		Title:        "Hauptdokument",
		MIMEType:     "application/pdf",
		SHA256:       "abc",
		UploadedAt:   uploadedAt,
	}}, map[int64][]document.Document{
		1: {{
			ID:           2,
			OriginalName: "anlage.pdf",
			Title:        "Anlage",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	var payload []struct {
		LinkedDocuments []struct {
			ID           int64  `json:"id"`
			Title        string `json:"title"`
			OriginalName string `json:"original_name"`
		} `json:"linked_documents"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || len(payload[0].LinkedDocuments) != 1 {
		t.Fatalf("payload = %#v", payload)
	}
	linked := payload[0].LinkedDocuments[0]
	if linked.ID != 2 || linked.Title != "Anlage" || linked.OriginalName != "anlage.pdf" {
		t.Fatalf("linked document = %#v", linked)
	}
}

func TestHandleExportFailsBeforeZipWhenStoredFileIsMissing(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}
	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "missing.pdf",
		StoredPath:   "2026/05/missing.pdf",
		Title:        "Missing",
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "missing-export",
	})
	if err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		store:     store,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: templates,
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/export?ids=%d", docID), nil)
	rec := httptest.NewRecorder()

	server.handleExport(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content type = %q", got)
	}
	if strings.Contains(rec.Header().Get("Content-Disposition"), "bearstack-export") {
		t.Fatalf("export disposition was set before missing file error: %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestHandleExportRejectsTooManyDocuments(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: templates,
	}
	values := url.Values{}
	for id := 1; id <= maxExportDocuments+1; id++ {
		values.Add("ids", strconv.Itoa(id))
	}
	req := httptest.NewRequest(http.MethodGet, "/export?"+values.Encode(), nil)
	rec := httptest.NewRecorder()

	server.handleExport(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Disposition"), "bearstack-export") {
		t.Fatalf("export disposition was set for rejected export: %q", rec.Header().Get("Content-Disposition"))
	}
}

func TestHandleExportWritesSelectedDocumentsAndMetadata(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(store.Root(), "2026", "05"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root(), "2026", "05", "main.pdf"), []byte("%PDF-1.7\nmain"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root(), "2026", "05", "linked.pdf"), []byte("%PDF-1.7\nlinked"), 0o600); err != nil {
		t.Fatal(err)
	}
	mainID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "main.pdf",
		StoredPath:   "2026/05/main.pdf",
		Title:        "Main",
		MIMEType:     "application/pdf",
		SizeBytes:    13,
		SHA256:       "export-main",
	})
	if err != nil {
		t.Fatal(err)
	}
	linkedID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName: "linked.pdf",
		StoredPath:   "2026/05/linked.pdf",
		Title:        "Linked",
		MIMEType:     "application/pdf",
		SizeBytes:    15,
		SHA256:       "export-linked",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.LinkDocuments(ctx, []int64{mainID, linkedID}); err != nil {
		t.Fatal(err)
	}
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		repo:      repo,
		store:     store,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		templates: templates,
	}
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/export?ids=%d&metadata=1&download_token=export-done-123", mainID), nil)
	rec := httptest.NewRecorder()

	server.handleExport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	cookie := testCookieByName(t, rec.Result().Cookies(), exportDownloadCookieName)
	if cookie.Value != "export-done-123" || cookie.Path != "/" || cookie.MaxAge != 60 {
		t.Fatalf("export download cookie = %#v", cookie)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if zipFileByName(zr, fmt.Sprintf("%d-main.pdf", mainID)) == nil {
		t.Fatalf("exported document missing from zip")
	}
	metadata := zipFileByName(zr, "metadata.json")
	if metadata == nil {
		t.Fatal("metadata.json missing")
	}
	reader, err := metadata.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var payload []struct {
		LinkedDocuments []struct {
			ID int64 `json:"id"`
		} `json:"linked_documents"`
	}
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || len(payload[0].LinkedDocuments) != 1 || payload[0].LinkedDocuments[0].ID != linkedID {
		t.Fatalf("metadata = %#v", payload)
	}
}

func zipFileByName(zr *zip.Reader, name string) *zip.File {
	for _, file := range zr.File {
		if file.Name == name {
			return file
		}
	}
	return nil
}
