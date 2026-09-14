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
