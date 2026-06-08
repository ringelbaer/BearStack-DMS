// Datei koordiniert asynchrone Verarbeitungsschritte fuer Import, OCR, Thumbnails und Indizes.
package server

import (
	"context"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/photos"
)

func (s *Server) EnsureThumbnails(ctx context.Context) error {
	return s.thumbnailService().EnsureAll(ctx)
}

func (s *Server) RunPhotoThumbnailWorker(ctx context.Context) {
	if s.photos == nil {
		return
	}
	for {
		settings, err := s.photoSettings(ctx)
		if err != nil {
			if s.log != nil {
				s.log.Warn("photo thumbnail settings failed", "error", err)
			}
		} else if settings.ThumbnailWorkerEnabled {
			s.configurePhotoThumbnailer(settings)
			generated, err := s.ensurePhotoThumbnails(ctx, settings)
			if err != nil && ctx.Err() == nil {
				if s.log != nil {
					s.log.Warn("photo thumbnail worker failed", "error", err)
				}
			} else if generated > 0 && s.log != nil {
				s.log.Info("photo thumbnail worker generated thumbnails", "count", generated)
			}
		}

		wait := time.Minute
		if settings.ThumbnailWorkerEnabled && settings.ThumbnailWorkerIntervalMinutes > 0 {
			wait = time.Duration(settings.ThumbnailWorkerIntervalMinutes) * time.Minute
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Server) RunPhotoIndexWorker(ctx context.Context) {
	if s.photos == nil {
		return
	}
	for {
		settings, err := s.photoSettings(ctx)
		if err != nil {
			if s.log != nil {
				s.log.Warn("photo index settings failed", "error", err)
			}
		} else if settings.IndexWorkerEnabled {
			stats, err := s.rebuildPhotoIndex(ctx, settings)
			if err != nil && ctx.Err() == nil {
				if s.log != nil {
					s.log.Warn("photo index worker failed", "error", err)
				}
			} else if s.log != nil {
				s.log.Info("photo index worker updated index", "media", stats.Media, "folders", stats.Folders, "blogs", stats.Blogs)
			}
		}

		wait := time.Minute
		if settings.IndexWorkerEnabled && settings.IndexWorkerIntervalMinutes > 0 {
			wait = time.Duration(settings.IndexWorkerIntervalMinutes) * time.Minute
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func photoIndexOptions(settings PhotoSettings) photos.IndexOptions {
	return photos.IndexOptions{
		EntryDelay:  time.Duration(settings.IndexWorkerDelayMillis) * time.Millisecond,
		LowPriority: true,
	}
}

func (s *Server) ensurePhotoThumbnails(ctx context.Context, settings PhotoSettings) (int, error) {
	if s.photos == nil {
		return 0, nil
	}
	s.configurePhotoThumbnailer(settings)
	release, err := s.acquirePhotoJob(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	generated, err := s.photos.EnsureThumbnails(ctx, []int{settings.ThumbnailSize}, settings.ThumbnailWorkerBatchSize)
	if generated > 0 {
		s.invalidatePhotoStatisticsCache()
	}
	return generated, err
}

func (s *Server) startPhotoThumbnailJob(settings PhotoSettings) bool {
	return s.startPhotoThumbnailJobForSizes(settings, []int{settings.ThumbnailSize})
}

func (s *Server) startPhotoThumbnailJobForSizes(settings PhotoSettings, sizes []int) bool {
	if s == nil || s.photos == nil {
		return false
	}
	if len(sizes) == 0 {
		return false
	}
	release, ok := s.tryAcquirePhotoJob()
	if !ok {
		return false
	}
	go func() {
		defer release()
		ctx := s.backgroundJobContext()
		s.configurePhotoThumbnailer(settings)
		generated, err := s.photos.EnsureThumbnails(ctx, sizes, settings.ThumbnailWorkerBatchSize)
		if err != nil {
			if s.log != nil {
				s.log.Warn("manual photo thumbnail job failed", "error", err)
			}
			return
		}
		if generated > 0 {
			s.invalidatePhotoStatisticsCache()
		}
		if s.log != nil {
			s.log.Info("manual photo thumbnail job finished", "count", generated)
		}
	}()
	return true
}

func (s *Server) configurePhotoThumbnailer(settings PhotoSettings) {
	if s == nil || s.photos == nil {
		return
	}
	s.photos.SetThumbnailConcurrency(settings.ThumbnailConcurrency)
}

func (s *Server) rebuildPhotoIndex(ctx context.Context, settings PhotoSettings) (photos.IndexStats, error) {
	if s.photos == nil {
		return photos.IndexStats{}, nil
	}
	release, err := s.acquirePhotoJob(ctx)
	if err != nil {
		return photos.IndexStats{}, err
	}
	defer release()
	stats, err := s.photos.RebuildIndexWithOptions(ctx, photoIndexOptions(settings))
	if err == nil {
		s.invalidatePhotoStatisticsCache()
	}
	return stats, err
}

func (s *Server) startPhotoIndexJob(settings PhotoSettings) bool {
	if s == nil || s.photos == nil {
		return false
	}
	release, ok := s.tryAcquirePhotoJob()
	if !ok {
		return false
	}
	go func() {
		defer release()
		ctx := s.backgroundJobContext()
		stats, err := s.photos.RebuildIndexWithOptions(ctx, photoIndexOptions(settings))
		if err != nil {
			if s.log != nil {
				s.log.Warn("manual photo index job failed", "error", err)
			}
			return
		}
		s.invalidatePhotoStatisticsCache()
		if s.log != nil {
			s.log.Info("manual photo index job finished", "media", stats.Media, "folders", stats.Folders, "blogs", stats.Blogs)
		}
	}()
	return true
}

func (s *Server) acquirePhotoJob(ctx context.Context) (func(), error) {
	if s.apps.photo.jobs == nil {
		return func() {}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case s.apps.photo.jobs <- struct{}{}:
		return func() { <-s.apps.photo.jobs }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Server) tryAcquirePhotoJob() (func(), bool) {
	if s.apps.photo.jobs == nil {
		return func() {}, true
	}
	select {
	case s.apps.photo.jobs <- struct{}{}:
		return func() { <-s.apps.photo.jobs }, true
	default:
		return nil, false
	}
}

func (s *Server) ensureDocumentThumbnail(ctx context.Context, doc document.Document) error {
	return s.thumbnailService().Ensure(ctx, doc)
}

func (s *Server) enqueueOCRJob(jobID int64) {
	s.ocrService().Enqueue(jobID)
}

func (s *Server) prepareOCRDocument(doc document.Document) (string, error) {
	return s.ocrService().PrepareDocument(doc)
}
