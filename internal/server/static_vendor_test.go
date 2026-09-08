package server

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestVersionedPDFJSAssetsCacheAllResourceTypes(t *testing.T) {
	staticFS, err := fs.Sub(webFS, "static")
	if err != nil {
		t.Fatal(err)
	}
	handler := cacheStaticAssets(staticFS)
	for _, asset := range []string{"build/pdf.mjs", "build/pdf.worker.mjs", "wasm/openjpeg.wasm", "cmaps/78-EUC-V.bcmap", "standard_fonts/LiberationSans-Regular.ttf", "iccs/CGATS001Compat-v2-micro.icc"} {
		for _, method := range []string{"GET", "HEAD"} {
			t.Run(method+"/"+asset, func(t *testing.T) {
				r := httptest.NewRequest(method, "/vendor/pdfjs-6.2.108/"+asset, nil)
				r.Header.Set("Accept-Encoding", "gzip")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
					t.Fatalf("%d %v", w.Code, w.Header())
				}
				if method == "HEAD" && w.Body.Len() != 0 {
					t.Fatal("HEAD body")
				}
			})
		}
	}
	r := httptest.NewRequest("GET", "/vendor/pdfjs-6.2.108/build/pdf.mjs", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	r.Header.Set("Range", "bytes=0-3")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.Len() != 4 || w.Header().Get("Content-Encoding") != "" || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("range: %d %v", w.Code, w.Header())
	}
}

func TestVersionedPDFJSCacheDoesNotIncludeUnversionedOrOtherPaths(t *testing.T) {
	handler := cacheStaticAssets(fstest.MapFS{
		"vendor/pdfjs-6.2.108/build/pdf.mjs":       {Data: []byte("export {}")},
		"vendor/pdfjs-dev/build/pdf.mjs":           {Data: []byte("export {}")},
		"vendor/pdfjs-6.2.x/build/pdf.mjs":         {Data: []byte("export {}")},
		"vendor/pdfjs-6.2.108-extra/build/pdf.mjs": {Data: []byte("export {}")},
		"app.js": {Data: []byte("test")},
	})
	for _, path := range []string{"/app.js", "/vendor/pdfjs-dev/build/pdf.mjs", "/vendor/pdfjs-6.2.x/build/pdf.mjs", "/vendor/pdfjs-6.2.108-extra/build/pdf.mjs", "/vendor/pdfjs-1.0.0/build/pdf.mjs", "/vendor/pdfjs-6.2.108/../pdfjs-dev/build/pdf.mjs"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if strings.Contains(w.Header().Get("Cache-Control"), "immutable") || (w.Code == 200 && w.Header().Get("Cache-Control") != "public, max-age=300") {
			t.Errorf("%s got %s", path, w.Header().Get("Cache-Control"))
		}
	}
}
