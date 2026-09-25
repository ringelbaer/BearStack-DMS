package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bearstack/internal/facerec"
)

func TestMissingFaceSourceDoesNotExposeFilesystemPath(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	if err := s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vector := make([]float32, facerec.Dimensions)
	vector[0] = 1
	if err = s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .3, Height: .3, Confidence: .99, Embedding: vector}}}); err != nil {
		t.Fatal(err)
	}
	faces, err := s.photos.AutomaticFaces(ctx, "one.jpg")
	if err != nil || len(faces) != 1 {
		t.Fatalf("faces=%v error=%v", faces, err)
	}
	id := strconv.FormatInt(faces[0].ID, 10)
	if err = os.Remove(filepath.Join(s.photos.Root(), "one.jpg")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/photos/faces/" + id + "/thumbnail", "/api/photos/v1/faces/" + id + "/thumbnail", "/photos/faces/" + id + "/review"} {
		t.Run(path, func(t *testing.T) {
			w := faceRequest(s, http.MethodGet, path, "editor", nil)
			if w.Code != http.StatusNotFound {
				t.Errorf("status=%d, want 404", w.Code)
			}
			for _, private := range []string{s.photos.Root(), "lstat ", "stat ", "no such file"} {
				if strings.Contains(w.Body.String(), private) {
					t.Errorf("response exposes %q", private)
				}
			}
			if strings.HasSuffix(path, "/review") {
				var body map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["code"] != "not_found" {
					t.Fatalf("error contract changed: %s (%v)", w.Body.String(), err)
				}
			}
		})
	}
}

func TestFaceFilesystemFailuresAreInternalErrors(t *testing.T) {
	s := faceTestServer(t)
	for _, cause := range []error{os.ErrPermission, errors.New("private device failure")} {
		err := &os.PathError{Op: "open", Path: "/private/photo-root/cache/face.jpg", Err: cause}
		w := httptest.NewRecorder()
		s.faceError(w, httptest.NewRequest(http.MethodGet, "/photos/faces/1/thumbnail", nil), err)
		if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "Interner Serverfehler") || strings.Contains(w.Body.String(), err.Path) || strings.Contains(w.Body.String(), cause.Error()) {
			t.Fatalf("filesystem failure exposed or misclassified: status=%d", w.Code)
		}
	}
}
