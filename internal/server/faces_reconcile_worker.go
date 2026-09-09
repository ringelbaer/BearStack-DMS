package server

import "context"

func (s *Server) stopFaceReconciliationRun() {
	s.faceWorker.mu.Lock()
	defer s.faceWorker.mu.Unlock()
	if s.faceWorker.reconcileCancel != nil {
		s.faceWorker.reconcileCancel()
	}
}

// Stored-vector work has its own bounded batches and never needs a live service.
func (s *Server) startFaceReconciliationRun() bool {
	if s.photos == nil || !s.faceWorker.reconcileRun.TryLock() {
		return false
	}
	ctx, cancel := context.WithCancel(s.backgroundJobContext())
	s.faceWorker.mu.Lock()
	s.faceWorker.reconcileCancel = cancel
	s.faceWorker.reconcileRunning = true
	s.faceWorker.reconcileError = ""
	s.faceWorker.mu.Unlock()
	if !s.background.start(func() {
		defer s.faceWorker.reconcileRun.Unlock()
		defer cancel()
		settings, err := s.faceSettings(ctx)
		if err == nil && settings.ReconcileEnabled {
			_, err = s.photos.ReconcileFacesBatch(ctx, min(settings.BatchSize, 100))
		}
		s.faceWorker.mu.Lock()
		s.faceWorker.reconcileRunning = false
		s.faceWorker.reconcileCancel = nil
		if err != nil && ctx.Err() == nil {
			s.faceWorker.reconcileError = "Gesichtszuordnungen konnten nicht geprüft werden."
		}
		s.faceWorker.mu.Unlock()
	}) {
		cancel()
		s.faceWorker.mu.Lock()
		s.faceWorker.reconcileRunning = false
		s.faceWorker.reconcileCancel = nil
		s.faceWorker.mu.Unlock()
		s.faceWorker.reconcileRun.Unlock()
		return false
	}
	return true
}
