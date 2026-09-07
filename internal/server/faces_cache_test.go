package server

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

func TestAllFaceThumbnailRoutesUsePhotoCache(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	if err := s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	embedding := make([]float32, facerec.Dimensions)
	embedding[0] = 1
	if err := s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .5, Height: .5, Confidence: .99, Embedding: embedding}}}); err != nil {
		t.Fatal(err)
	}
	faces, err := s.photos.AutomaticFaces(ctx, job.Path)
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces: %v %v", faces, err)
	}
	id := faces[0].ID
	cacheDir := filepath.Join(s.photos.CacheDir(), "faces", "v1")
	for _, size := range []int{160, 640} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			routes := []string{fmt.Sprintf("/api/photos/labeling/v1/faces/%d/thumbnail?size=%d", id, size)}
			if size == 160 {
				routes = append(routes, fmt.Sprintf("/photos/faces/%d/thumbnail", id), fmt.Sprintf("/api/photos/labeling/v1/faces/%d/thumbnail", id))
			}
			// The first request must create the file in the configured photo cache.
			first := labelRequest(s, "GET", routes[0], "manager", "")
			if first.Code != 200 {
				t.Fatalf("initial request: %d %s", first.Code, first.Body.String())
			}
			original := bytes.Clone(first.Body.Bytes())
			files, err := filepath.Glob(filepath.Join(cacheDir, "*", "*", "*.jpg"))
			if err != nil {
				t.Fatal(err)
			}
			var cachedPath string
			for _, path := range files {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Equal(data, original) {
					cachedPath = path
					break
				}
			}
			if cachedPath == "" {
				t.Fatal("response was not saved in photo cache")
			}
			// Use a distinct valid JPEG to prove that responses read the cache, rather
			// than regenerating identical bytes from the original photograph.
			img := image.NewRGBA(image.Rect(0, 0, size, size))
			img.SetRGBA(size/2, size/2, color.RGBA{R: 255, A: 255})
			var cached bytes.Buffer
			if err := jpeg.Encode(&cached, img, &jpeg.Options{Quality: 95}); err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(cached.Bytes(), original) {
				t.Fatal("fixture must differ from original crop")
			}
			if err := os.WriteFile(cachedPath, cached.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			stamp := time.Unix(1700000000, 0)
			if err := os.Chtimes(cachedPath, stamp, stamp); err != nil {
				t.Fatal(err)
			}

			// Reopen the library: no process-local state may be required for a cache hit.
			root, cache, db := s.photos.Root(), s.photos.CacheDir(), s.photos.DBPath()
			if err := s.photos.Close(); err != nil {
				t.Fatal(err)
			}
			s.photos, err = photos.New(root, cache, db, 60)
			if err != nil {
				t.Fatal(err)
			}
			for _, route := range routes {
				response := labelRequest(s, "GET", route, "manager", "")
				if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), cached.Bytes()) {
					t.Fatalf("%s did not serve cached JPEG: %d", route, response.Code)
				}
				info, err := os.Stat(cachedPath)
				if err != nil || !info.ModTime().Equal(stamp) {
					t.Fatalf("%s rewrote cache file: %v", route, err)
				}
			}
			// Every entry point must also populate the shared cache when it is missing.
			for _, route := range routes {
				if err := os.Remove(cachedPath); err != nil {
					t.Fatal(err)
				}
				response := labelRequest(s, "GET", route, "manager", "")
				if response.Code != 200 || !bytes.Equal(response.Body.Bytes(), original) {
					t.Fatalf("%s did not regenerate missing crop: %d", route, response.Code)
				}
				data, err := os.ReadFile(cachedPath)
				if err != nil || !bytes.Equal(data, original) {
					t.Fatalf("%s did not restore cache file: %v", route, err)
				}
			}
		})
	}
}
