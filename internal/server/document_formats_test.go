package server

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
	"bearstack/internal/testutil"
)

func TestPreviewAndBackgroundThumbnailShareFormatRules(t *testing.T) {
	testutil.InstallSoffice(t)
	installFakePDFToPPM(t)
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mime string
		data       []byte
		supported  bool
	}{
		{"office", "\tAPPLICATION/RTF ; charset=utf-8", []byte(`{\rtf1\ansi text}`), true},
		{"text", " TEXT/PLAIN ; charset=utf-8", []byte("preview text"), true},
		{"pdf", "APPLICATION/PDF; version=1.4", []byte("%PDF-1.4\nfixture"), true},
		{"image", " IMAGE/PNG ; param=value", picture.Bytes(), true},
		{"unsupported", "text/plain-extra", []byte("not a supported type"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			stored := writeStoredTestFile(t, store, "opaque.bin", tc.data)
			id, err := repo.CreateDocument(ctx, document.Document{OriginalName: "opaque.bin", StoredPath: stored, MIMEType: tc.mime, Title: "format", SizeBytes: int64(len(tc.data))})
			if err != nil {
				t.Fatal(err)
			}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			service := newThumbnailService(repo, store, log, make(chan struct{}, 1))
			if err := service.EnsureAll(ctx); err != nil {
				t.Fatal(err)
			}
			doc, err := repo.GetDocumentFile(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if (doc.ThumbnailPath != "") != tc.supported {
				t.Fatalf("thumbnail path = %q", doc.ThumbnailPath)
			}
			s := &Server{repo: repo, store: store, log: log}
			r := httptest.NewRequest(http.MethodGet, "/documents/"+strconv.FormatInt(id, 10)+"/preview", nil)
			r.SetPathValue("id", strconv.FormatInt(id, 10))
			w := httptest.NewRecorder()
			s.handlePreview(w, r)
			want := http.StatusOK
			if !tc.supported {
				want = http.StatusUnsupportedMediaType
			}
			if w.Code != want {
				t.Fatalf("preview status = %d, body = %s", w.Code, w.Body.String())
			}
		})
	}
}
