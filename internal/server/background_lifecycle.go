package server

import (
	"context"
	"sync"
)

// Registration and stopping share a lock so shutdown cannot miss a new job.
type backgroundTasks struct {
	mu      sync.Mutex
	active  int
	stopped bool
	done    chan struct{}
}

func (g *backgroundTasks) start(run func()) bool {
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		return false
	}
	g.active++
	g.mu.Unlock()
	go func() {
		defer func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.active--
			if g.stopped && g.active == 0 {
				close(g.done)
			}
		}()
		run()
	}()
	return true
}

func (g *backgroundTasks) stop() <-chan struct{} {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.stopped {
		g.stopped = true
		g.done = make(chan struct{})
		if g.active == 0 {
			close(g.done)
		}
	}
	return g.done
}

func (g *backgroundTasks) wait(ctx context.Context) error {
	done := g.stop()
	select {
	case <-done:
		return nil
	default:
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StopBackgroundJobs rejects new jobs and asks running workers to finish.
func (s *Server) StopBackgroundJobs() {
	s.background.stop()
	s.jobCtxMu.Lock()
	if s.jobCancel != nil {
		s.jobCancel()
	}
	s.jobCtxMu.Unlock()
	s.stopFaceRun()
}

func (s *Server) WaitBackgroundJobs(ctx context.Context) error {
	return s.background.wait(ctx)
}

// Shutdown is called after HTTP requests have drained. On timeout the photo
// database stays open, allowing still-running jobs to finish safely.
func (s *Server) Shutdown(ctx context.Context) error {
	s.StopBackgroundJobs()
	if err := s.WaitBackgroundJobs(ctx); err != nil {
		return err
	}
	if s.photos != nil {
		return s.photos.Close()
	}
	return nil
}
