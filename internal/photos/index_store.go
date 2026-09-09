// Datei kapselt Oeffnen, Verfuegbarkeit und Lebenszyklus des Fotoindex-Stores.
package photos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
)

type photoIndexStore struct {
	db               *sql.DB
	mapIndexesDone   chan struct{}
	mapIndexesCancel context.CancelFunc
	mapIndexesErr    error // published by closing mapIndexesDone
}

func openPhotoIndexStore(path string) (*photoIndexStore, string, error) {
	db, abs, err := openIndexDB(path)
	if err != nil || db == nil {
		return nil, abs, err
	}
	return &photoIndexStore{db: db}, abs, nil
}

func (s *photoIndexStore) available() bool {
	return s != nil && s.db != nil
}

func (s *photoIndexStore) close() error {
	if !s.available() {
		return nil
	}
	if s.mapIndexesCancel != nil {
		s.mapIndexesCancel()
		<-s.mapIndexesDone
	}
	return s.db.Close()
}

// Called once, after mandatory startup work. SQLite keeps each CREATE INDEX
// atomic; cancellation or a failed build can safely resume on the next open.
// FILE temp storage bounds sorting memory, and WAL readers remain available.
func (s *photoIndexStore) startMapIndexes() {
	ctx, cancel := context.WithCancel(context.Background())
	s.mapIndexesCancel = cancel
	s.mapIndexesDone = make(chan struct{})
	go func() {
		defer close(s.mapIndexesDone)
		for {
			s.mapIndexesErr = ensurePhotoMapIndexes(ctx, s.db)
			var sqliteErr *sqlite.Error
			if ctx.Err() != nil || !errors.As(s.mapIndexesErr, &sqliteErr) || (sqliteErr.Code()&255 != 5 && sqliteErr.Code()&255 != 6) {
				return
			}
			// An ordinary scanner/writer may outlast SQLite's busy timeout.
			// Retry contention without requiring another application restart.
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				s.mapIndexesErr = ctx.Err()
				return
			case <-timer.C:
			}
		}
	}()
}

func (s *photoIndexStore) waitMapIndexes(ctx context.Context) error {
	if s.mapIndexesDone == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.mapIndexesDone:
		if s.mapIndexesErr != nil {
			return fmt.Errorf("%w: preparing map indexes: %v", ErrMapIndexUnavailable, s.mapIndexesErr)
		}
		return nil
	}
}
