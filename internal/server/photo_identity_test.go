package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
	"bearstack/internal/testutil/apicontract"
)

func TestPhotoIdentityRoutesPermissionsReviewAndContract(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	data, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := apicontract.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/settings/photos/identities", "/photos/faces/review"} {
		w := faceRequest(s, "GET", path, "reader", nil)
		if w.Code != 403 {
			t.Fatal(path, w.Code, w.Body.String())
		}
		w = faceRequest(s, "GET", path, "manager", nil)
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body.String())
		}
		if err = spec.ValidateResponse(path, "GET", w.Code, w.Header(), w.Body.Bytes()); err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("GET", path, nil)
		r.SetBasicAuth("manager", "secret")
		w = httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "</html>") {
			t.Fatal("render", path, w.Code, w.Body.String())
		}
	}
	if err = s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	v := make([]float32, 128)
	v[0] = 1
	if err = s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{{X: .1, Y: .1, Width: .3, Height: .3, Confidence: .99, Embedding: v}}}); err != nil {
		t.Fatal(err)
	}
	fs, err := s.photos.AutomaticFaces(ctx, "one.jpg")
	if err != nil || len(fs) != 1 {
		t.Fatal(fs, err)
	}
	if _, err = s.photos.FaceThumbnail(ctx, fs[0].ID); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(s.photos.Root(), "one.jpg"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write([]byte("changed"))
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.photos.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	path := "/photos/faces/" + strconv.FormatInt(fs[0].ID, 10) + "/review"
	w := faceRequest(s, "GET", path, "editor", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = spec.ValidateResponse("/photos/faces/{id}/review", "GET", w.Code, w.Header(), w.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	var review photos.FaceSourceReview
	if err = json.Unmarshal(w.Body.Bytes(), &review); err != nil || !review.Face.NeedsReview {
		t.Fatal(review, err)
	}
	r := httptest.NewRequest("GET", path, nil)
	r.SetBasicAuth("editor", "secret")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "data-review-stage") {
		t.Fatal(w.Code, w.Body.String())
	}
	form := url.Values{"revision": {"stale"}, "x": {"0.1"}, "y": {"0.1"}, "width": {"0.3"}, "height": {"0.3"}, "name": {"Ada"}}
	w = faceRequest(s, "POST", path, "editor", form)
	if w.Code != 409 {
		t.Fatal("stale", w.Code, w.Body.String())
	}
	form.Set("revision", review.Revision)
	w = faceRequest(s, "POST", path, "editor", form)
	if w.Code != 200 {
		t.Fatal("confirm", w.Code, w.Body.String())
	}
	if err = spec.ValidateResponse("/photos/faces/{id}/review", "POST", w.Code, w.Header(), w.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
}

func TestFaceReviewHTMLContinuesAndWrapsOnlyPendingFaces(t *testing.T) {
	s := faceTestServer(t)
	ctx := context.Background()
	if err := s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	job, err := s.photos.NextFaceJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	vector := make([]float32, 128)
	vector[0] = 1
	if err = s.photos.CommitFaceResult(ctx, job, facerec.Result{Model: facerec.Model, Faces: []facerec.Detection{
		{X: .1, Y: .1, Width: .2, Height: .2, Confidence: .99, Embedding: vector},
		{X: .6, Y: .6, Width: .2, Height: .2, Confidence: .99, Embedding: vector},
	}}); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(s.photos.Root(), "one.jpg"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("changed")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.photos.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	pending, err := s.photos.PendingFaceReviews(ctx, 0)
	if err != nil || len(pending) != 2 {
		t.Fatal(pending, err)
	}
	// Starting with the last pending face must wrap to the earlier one.
	for _, index := range []int{1, 0} {
		id := pending[index].ID
		review, err := s.photos.FaceSourceReview(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		form := url.Values{"revision": {review.Revision}, "x": {"0.1"}, "y": {"0.1"}, "width": {"0.2"}, "height": {"0.2"}, "name": {"Ada"}, "next": {"1"}}
		route := "/photos/faces/" + strconv.FormatInt(id, 10) + "/review"
		req := httptest.NewRequest("POST", route, strings.NewReader(form.Encode()))
		req.SetBasicAuth("editor", "secret")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.com")
		recorder := httptest.NewRecorder()
		s.Handler().ServeHTTP(recorder, req)
		if recorder.Code != 303 {
			t.Fatal(recorder.Code, recorder.Body.String())
		}
		destination, err := url.Parse(recorder.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		want := "/photos/faces/review"
		if index == 1 {
			want = "/photos/faces/" + strconv.FormatInt(pending[0].ID, 10) + "/review"
		}
		if destination.Path != want {
			t.Fatalf("next location: %s, want %s", destination.Path, want)
		}
	}
}

func TestReviewPagesUseFormattedPhotoPaths(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"photo.jpg", "2026_07_15_Holiday/photo.jpg", "2026/2026_07_15_Holiday/Nested_Folder/photo.jpg"} {
		face := photos.RecognizedFace{ID: 1, PersonID: 2, Path: path, NeedsReview: true}
		data := PageData{PeopleSection: "review", FaceSourceReview: photos.FaceSourceReview{Face: face}, FaceSourceReviews: []photos.RecognizedFace{face}}
		for _, page := range []string{"face_source_review.html", "face_source_reviews.html"} {
			var output strings.Builder
			if err := templates.ExecuteTemplate(&output, page, data); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), photos.MediaDisplayPath(path)) {
				t.Fatalf("%s misses formatted path %s", page, path)
			}
		}
	}
}
