package photos

import (
	"context"
	"errors"
	"sync"
	"time"
)

const faceProgressTTL = 5 * time.Second

type faceProgressFlight struct {
	done   chan struct{}
	status FaceStatus
	err    error
}

type faceProgressState struct {
	mu      sync.Mutex
	status  FaceStatus
	expires time.Time
	flight  *faceProgressFlight
}

// FaceProgress contains only indexed counts, no per-file errors or names. It
// never walks gallery directories. A short shared snapshot bounds polling cost
// across tabs; concrete face access still performs fresh visibility checks.
func (l *Library) FaceProgress(ctx context.Context) (FaceStatus, error) {
	c := &l.faceProgress
	for {
		if err := ctx.Err(); err != nil {
			return FaceStatus{}, err
		}
		c.mu.Lock()
		if time.Now().Before(c.expires) {
			status := c.status
			c.mu.Unlock()
			return status, nil
		}
		if flight := c.flight; flight != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return FaceStatus{}, ctx.Err()
			case <-flight.done:
				if errors.Is(flight.err, context.Canceled) || errors.Is(flight.err, context.DeadlineExceeded) {
					continue
				}
				return flight.status, flight.err
			}
		}
		flight := &faceProgressFlight{done: make(chan struct{})}
		c.flight = flight
		c.mu.Unlock()
		status, err := l.faceCounts(ctx)
		c.mu.Lock()
		if err == nil {
			c.status, c.expires = status, time.Now().Add(faceProgressTTL)
		}
		flight.status, flight.err = status, err
		c.flight = nil
		close(flight.done)
		c.mu.Unlock()
		return status, err
	}
}

func (l *Library) faceCounts(ctx context.Context) (FaceStatus, error) {
	var s FaceStatus
	var drawn int
	err := l.index.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM photo_face_jobs WHERE status='queued'),(SELECT count(*) FROM photo_face_jobs WHERE status='done'),(SELECT count(*) FROM photo_face_jobs WHERE status='failed'),(SELECT count(*) FROM photo_faces WHERE ignored=0),(SELECT count(DISTINCT person_id) FROM photo_faces WHERE ignored=0),(SELECT count(*) FROM photo_faces WHERE ignored=0 AND recognition_assignment='matched'),(SELECT count(*) FROM photo_faces WHERE ignored=0 AND recognition_assignment='new'),(SELECT count(*) FROM photo_faces WHERE drawn=1 AND ignored=0)`).Scan(&s.Queued, &s.Done, &s.Failed, &s.Faces, &s.People, &s.RecognitionMatched, &s.RecognitionNew, &drawn)
	s.RecognitionUnknown = s.Faces - s.RecognitionMatched - s.RecognitionNew - drawn
	if total := s.RecognitionMatched + s.RecognitionNew; total > 0 {
		s.RecognitionMatchPercent = 100 * float64(s.RecognitionMatched) / float64(total)
	}
	return s, err
}
