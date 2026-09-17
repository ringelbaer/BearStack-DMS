package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

type faceAnalysisLibraryStub struct {
	calls  []string
	failAt string
	err    error
	job    photos.FaceJob
	result facerec.Result
}

func (l *faceAnalysisLibraryStub) FaceImage(_ context.Context, path string) ([]byte, error) {
	l.calls = append(l.calls, "image:"+path)
	if l.failAt == "image" {
		return nil, l.err
	}
	return []byte("oriented image"), nil
}

func (l *faceAnalysisLibraryStub) RefineFaceResult(ctx context.Context, path string, result facerec.Result, analyze func(context.Context, []byte) (facerec.Result, error)) (facerec.Result, error) {
	l.calls = append(l.calls, "refine:"+path)
	if l.failAt == "refine" {
		return facerec.Result{}, l.err
	}
	if _, err := analyze(ctx, []byte("crop")); err != nil {
		return facerec.Result{}, err
	}
	result.Faces = append(result.Faces, facerec.Detection{X: .25})
	return result, nil
}

func (l *faceAnalysisLibraryStub) CommitFaceResult(_ context.Context, job photos.FaceJob, result facerec.Result) error {
	l.calls = append(l.calls, "commit")
	l.job, l.result = job, result
	return nil
}

type faceAnalysisTransport struct {
	images []string
	closed bool
	fail   bool
}

func (tr *faceAnalysisTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	tr.images = append(tr.images, string(data))
	if tr.fail {
		return nil, errors.New("offline")
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"model":"` + facerec.Model + `","faces":[]}`))}, nil
}

func (tr *faceAnalysisTransport) CloseIdleConnections() { tr.closed = true }

func TestFaceAnalysisServicePreservesJobAndDoesNotCommitFailures(t *testing.T) {
	for _, failure := range []string{"", "image", "inference", "refine"} {
		t.Run("failure="+failure, func(t *testing.T) {
			lib := &faceAnalysisLibraryStub{failAt: failure, err: errors.New("source unavailable")}
			transport := &faceAnalysisTransport{fail: failure == "inference"}
			svc := newFaceAnalysisService(lib, func() (*facerec.Client, error) {
				return &facerec.Client{URL: "http://faces.test", HTTP: &http.Client{Transport: transport}}, nil
			})
			analyzer, err := svc.NewAnalyzer()
			if err != nil {
				t.Fatal(err)
			}
			defer analyzer.Close()
			release, err := svc.Acquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			job := photos.FaceJob{Path: "trip/photo.jpg"}
			err = analyzer.AnalyzeJob(context.Background(), job)
			if failure != "" {
				if err == nil || lib.result.Model != "" {
					t.Fatalf("failed analysis committed: result=%+v err=%v", lib.result, err)
				}
				if failure != "inference" && !errors.Is(err, lib.err) {
					t.Fatalf("lost source error: %v", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(lib.job, job) || len(lib.result.Faces) != 1 || lib.result.Faces[0].X != .25 {
				t.Fatalf("refined result/job not preserved: %+v %+v %v", lib.job, lib.result, err)
			}
			if !reflect.DeepEqual(transport.images, []string{"oriented image", "crop"}) || !reflect.DeepEqual(lib.calls, []string{"image:trip/photo.jpg", "refine:trip/photo.jpg", "commit"}) {
				t.Fatalf("analysis flow: %v %v", transport.images, lib.calls)
			}
		})
	}
}

func TestFaceAnalysisServiceReviewDoesNotReplaceSavedFaces(t *testing.T) {
	lib := &faceAnalysisLibraryStub{}
	var transports []*faceAnalysisTransport
	svc := newFaceAnalysisService(lib, func() (*facerec.Client, error) {
		tr := &faceAnalysisTransport{}
		transports = append(transports, tr)
		return &facerec.Client{URL: "http://faces.test", HTTP: &http.Client{Transport: tr}}, nil
	})
	worker, err := svc.NewAnalyzer()
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	review, err := svc.NewAnalyzer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := review.Analyze(context.Background(), []byte("current image")); err != nil {
		t.Fatal(err)
	}
	review.Close()
	if len(lib.calls) != 0 {
		t.Fatalf("review changed saved faces: %v", lib.calls)
	}
	if transports[0].closed || !transports[1].closed {
		t.Fatal("closing review affected worker transport")
	}
}
