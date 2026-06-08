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
	mu        sync.RWMutex
	documents documentStatisticsCacheEntry
	photos    photoStatisticsCacheEntry
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
	now := time.Now()
	state := &s.apps.statistics
	state.mu.RLock()
	entry := state.documents
	if now.Before(entry.expiresAt) {
		state.mu.RUnlock()
		return entry.value, nil
	}
	state.mu.RUnlock()

	stats, err := s.repo.Statistics(ctx)
	if err != nil {
		return document.Statistics{}, err
	}
	state.mu.Lock()
	state.documents = documentStatisticsCacheEntry{
		value:     stats,
		expiresAt: time.Now().Add(statisticsCacheTTL),
	}
	state.mu.Unlock()
	return stats, nil
}

func (s *Server) cachedPhotoStatistics(ctx context.Context) (photos.Statistics, error) {
	now := time.Now()
	state := &s.apps.statistics
	state.mu.RLock()
	entry := state.photos
	if now.Before(entry.expiresAt) {
		state.mu.RUnlock()
		return entry.value, nil
	}
	state.mu.RUnlock()

	stats, err := s.photos.Statistics(ctx)
	if err != nil {
		return photos.Statistics{}, err
	}
	state.mu.Lock()
	state.photos = photoStatisticsCacheEntry{
		value:     stats,
		expiresAt: time.Now().Add(statisticsCacheTTL),
	}
	state.mu.Unlock()
	return stats, nil
}

func (s *Server) invalidateDocumentStatisticsCache() {
	if s == nil {
		return
	}
	state := &s.apps.statistics
	state.mu.Lock()
	state.documents = documentStatisticsCacheEntry{}
	state.mu.Unlock()
}

func (s *Server) invalidatePhotoStatisticsCache() {
	if s == nil {
		return
	}
	if s.photos != nil {
		s.photos.InvalidateStatisticsCache()
	}
	state := &s.apps.statistics
	state.mu.Lock()
	state.photos = photoStatisticsCacheEntry{}
	state.mu.Unlock()
}
