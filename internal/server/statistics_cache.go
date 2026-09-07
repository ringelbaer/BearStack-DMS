// Datei cached teure Statistikberechnungen und invalidiert sie bei relevanten Aenderungen.
package server

import (
	"context"
	"sync"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/photos"
)

const statisticsCacheTTL = 30 * time.Second

type statisticsCacheState struct {
	mu                 sync.RWMutex
	documents          documentStatisticsCacheEntry
	documentFlight     *documentStatisticsFlight
	documentGeneration uint64
	photos             photoStatisticsCacheEntry
	photoFlight        *photoStatisticsFlight
	photoGeneration    uint64
}

type photoStatisticsFlight struct {
	done  chan struct{}
	value photos.Statistics
	err   error
}

type documentStatisticsFlight struct {
	done  chan struct{}
	value document.Statistics
	err   error
}

type documentStatisticsCacheEntry struct {
	value     document.Statistics
	expiresAt time.Time
}

type photoStatisticsCacheEntry struct {
	value     photos.Statistics
	expiresAt time.Time
}

func (s *Server) cachedDocumentStatistics(ctx context.Context) (document.Statistics, error) {
	return s.apps.statistics.documentStatistics(ctx, s.repo.Statistics)
}

func (state *statisticsCacheState) documentStatistics(ctx context.Context, calculate func(context.Context) (document.Statistics, error)) (document.Statistics, error) {
	if err := ctx.Err(); err != nil {
		return document.Statistics{}, err
	}
	now := time.Now()
	state.mu.Lock()
	entry := state.documents
	if now.Before(entry.expiresAt) {
		state.mu.Unlock()
		return entry.value, nil
	}
	if flight := state.documentFlight; flight != nil {
		state.mu.Unlock()
		select {
		case <-ctx.Done():
			return document.Statistics{}, ctx.Err()
		case <-flight.done:
			return flight.value, flight.err
		}
	}
	flight := &documentStatisticsFlight{done: make(chan struct{})}
	state.documentFlight = flight
	generation := state.documentGeneration
	state.mu.Unlock()

	stats, err := calculate(ctx)
	state.mu.Lock()
	if err == nil && generation == state.documentGeneration {
		state.documents = documentStatisticsCacheEntry{value: stats, expiresAt: time.Now().Add(statisticsCacheTTL)}
	}
	flight.value, flight.err = stats, err
	state.documentFlight = nil
	close(flight.done)
	state.mu.Unlock()
	return stats, err
}

func (s *Server) cachedPhotoStatistics(ctx context.Context) (photos.Statistics, error) {
	now := time.Now()
	state := &s.apps.statistics
	state.mu.Lock()
	entry := state.photos
	if now.Before(entry.expiresAt) {
		state.mu.Unlock()
		return entry.value, nil
	}
	if flight := state.photoFlight; flight != nil {
		state.mu.Unlock()
		select {
		case <-ctx.Done():
			return photos.Statistics{}, ctx.Err()
		case <-flight.done:
			return flight.value, flight.err
		}
	}
	flight := &photoStatisticsFlight{done: make(chan struct{})}
	state.photoFlight = flight
	generation := state.photoGeneration
	state.mu.Unlock()

	stats, err := s.photos.Statistics(ctx)
	state.mu.Lock()
	if err == nil && generation == state.photoGeneration {
		state.photos = photoStatisticsCacheEntry{value: stats, expiresAt: time.Now().Add(statisticsCacheTTL)}
	}
	flight.value, flight.err = stats, err
	state.photoFlight = nil
	close(flight.done)
	state.mu.Unlock()
	return stats, err
}

func (s *Server) invalidateDocumentStatisticsCache() {
	if s == nil {
		return
	}
	state := &s.apps.statistics
	state.mu.Lock()
	state.documents = documentStatisticsCacheEntry{}
	state.documentGeneration++
	state.mu.Unlock()
}

func (s *Server) invalidatePhotoStatisticsCache() {
	if s == nil {
		return
	}
	state := &s.apps.statistics
	state.mu.Lock()
	state.photos = photoStatisticsCacheEntry{}
	state.photoGeneration++
	state.mu.Unlock()
}

func (s *Server) runPhotoCacheStatistics(ctx context.Context) {
	if s.photos == nil {
		return
	}
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		if err := s.photos.RefreshThumbnailCacheStatistics(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			logWarn(s.log, "photo cache statistics refresh failed", "error", err)
		} else {
			s.invalidatePhotoStatisticsCache()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
