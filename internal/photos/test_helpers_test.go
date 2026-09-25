package photos

import (
	"context"
	"io"
)

func (l *Library) saveMedia(media Media) {
	if err := l.saveMediaContext(context.Background(), media); err != nil {
		l.logWriteError("photo media cache update failed", media.Path, err)
	}
}

func (l *Library) nearestPerson(ctx context.Context, tx faceRowsQuery, v []float32, excluded map[int64]bool) (int64, error) {
	named, err := faceNamedPeople(ctx, tx)
	if err != nil {
		return 0, err
	}
	return l.nearestPersonInGroups(ctx, tx, v, excluded, named)
}

func decodeGPX(ctx context.Context, input io.Reader, maxBytes int64, maxPoints int) ([]GPXPoint, error) {
	points, _, err := decodeGPXSegments(ctx, input, maxBytes, maxPoints)
	return points, err
}

// facePersonCandidates combines ranking and validation for candidate tests.
// Caller holds faceRuntime.mu and has called ensureFaceGraph.
func (l *Library) facePersonCandidates(ctx context.Context, tx faceRowsQuery, v []float32, excluded map[int64]bool, limit int) ([]facePersonCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}
	rt := &l.faceRuntime
	ranked, err := rt.rankFacePersons(ctx, v, excluded)
	if err != nil {
		return nil, err
	}
	return l.validateFacePersonCandidates(ctx, tx, v, ranked, limit)
}
