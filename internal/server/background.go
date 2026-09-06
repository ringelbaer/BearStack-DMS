// Datei startet und koordiniert Hintergrundaufgaben wie Indexierung, Import und Wartung.
package server

import (
	"context"
	"log/slog"
	"time"
)

type BackgroundWorkers struct {
	server                  *Server
	log                     *slog.Logger
	ensureThumbnails        func(context.Context) error
	runDocumentPostImport   func(context.Context)
	runOCRQueue             func(context.Context)
	runMailImport           func(context.Context)
	runTrashRetention       func(context.Context)
	runPhotoIndexWorker     func(context.Context)
	runPhotoThumbnailWorker func(context.Context)
}

const thumbnailStartupDelay = 5 * time.Second

func (s *Server) BackgroundWorkers() BackgroundWorkers {
	return BackgroundWorkers{
		server:                  s,
		log:                     s.log,
		ensureThumbnails:        s.thumbnailService().EnsureAll,
		runDocumentPostImport:   s.documentPostProcessor().Run,
		runOCRQueue:             s.ocrService().RunQueue,
		runMailImport:           s.mailImportService().Run,
		runTrashRetention:       s.trashService().RunRetention,
		runPhotoIndexWorker:     s.RunPhotoIndexWorker,
		runPhotoThumbnailWorker: s.RunPhotoThumbnailWorker,
	}
}

func (s *Server) setBackgroundJobContext(ctx context.Context) {
	if s == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.jobCtxMu.Lock()
	s.jobCtx = ctx
	s.jobCtxMu.Unlock()
}

func (s *Server) backgroundJobContext() context.Context {
	if s == nil {
		return context.Background()
	}
	s.jobCtxMu.RLock()
	ctx := s.jobCtx
	s.jobCtxMu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (w BackgroundWorkers) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if w.server != nil {
		w.server.setBackgroundJobContext(ctx)
	}
	if w.ensureThumbnails != nil {
		go func() {
			timer := time.NewTimer(thumbnailStartupDelay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			if err := w.ensureThumbnails(ctx); err != nil && w.log != nil {
				w.log.Warn("thumbnail generation failed", "error", err)
			}
		}()
	}

	start := func(run func(context.Context)) {
		if run != nil {
			go run(ctx)
		}
	}
	start(w.runDocumentPostImport)
	start(w.runOCRQueue)
	start(w.runMailImport)
	start(w.runTrashRetention)
	start(w.runPhotoIndexWorker)
	start(w.runPhotoThumbnailWorker)
	if w.server != nil {
		start(w.server.RunFaceWorker)
	}
}
