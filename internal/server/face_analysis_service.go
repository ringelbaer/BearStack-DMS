package server

import (
	"context"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

type faceAnalysisLibrary interface {
	FaceImage(context.Context, string) ([]byte, error)
	RefineFaceResult(context.Context, string, facerec.Result, func(context.Context, []byte) (facerec.Result, error)) (facerec.Result, error)
	CommitFaceResult(context.Context, photos.FaceJob, facerec.Result) error
}

// One service owns the analysis gate shared by workers, manual analysis,
// source reviews and deletion. Construction performs no I/O.
type faceAnalysisService struct {
	library   faceAnalysisLibrary
	newClient func() (*facerec.Client, error)
	gate      chan struct{}
}

func newFaceAnalysisService(library faceAnalysisLibrary, newClient func() (*facerec.Client, error)) *faceAnalysisService {
	return &faceAnalysisService{library: library, newClient: newClient, gate: make(chan struct{}, 1)}
}

// Callers hold the gate across job selection/revision checks and the final
// mutation, including failure recording. Waiting remains cancellable.
func (svc *faceAnalysisService) Acquire(ctx context.Context) (func(), error) {
	select {
	case svc.gate <- struct{}{}:
		return func() { <-svc.gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type faceAnalyzer struct {
	library faceAnalysisLibrary
	client  *facerec.Client
}

// A worker reuses one analyzer for its batch; request handlers close theirs
// after the action. Closing never affects another caller's transport.
func (svc *faceAnalysisService) NewAnalyzer() (*faceAnalyzer, error) {
	client, err := svc.newClient()
	if err != nil {
		return nil, err
	}
	return &faceAnalyzer{library: svc.library, client: client}, nil
}

func (a *faceAnalyzer) Close() { a.client.HTTP.CloseIdleConnections() }

func (a *faceAnalyzer) Health(ctx context.Context) error { return a.client.Health(ctx) }

// Source reviews analyze a prepared image without replacing the saved faces.
func (a *faceAnalyzer) Analyze(ctx context.Context, data []byte) (facerec.Result, error) {
	return a.client.Analyze(ctx, data)
}

// Caller holds the service gate. Manual and background jobs share orientation,
// refinement, matching and preservation of existing manual decisions.
func (a *faceAnalyzer) AnalyzeJob(ctx context.Context, job photos.FaceJob) error {
	data, err := a.library.FaceImage(ctx, job.Path)
	if err != nil {
		return err
	}
	result, err := a.Analyze(ctx, data)
	if err != nil {
		return err
	}
	result, err = a.library.RefineFaceResult(ctx, job.Path, result, a.Analyze)
	if err != nil {
		return err
	}
	return a.library.CommitFaceResult(ctx, job, result)
}
