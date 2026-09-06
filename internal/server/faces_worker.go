package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"bearstack/internal/facerec"
	"bearstack/internal/photos"
)

const faceSettingsKey = "photo_face_settings"

type FaceSettings struct {
	Enabled         bool `json:"enabled"`
	BatchSize       int  `json:"batch_size"`
	DelayMillis     int  `json:"delay_millis"`
	IntervalMinutes int  `json:"interval_minutes"`
}
type faceWorkerState struct {
	wake         chan struct{}
	lastFinished time.Time
	mu           sync.Mutex
	run          sync.Mutex
	cancel       context.CancelFunc
	running      bool
	lastError    string
	lastScan     time.Time
}
type FaceSettingsView struct {
	Settings   FaceSettings      `json:"settings"`
	Status     photos.FaceStatus `json:"status"`
	Running    bool              `json:"running"`
	Error      string            `json:"error,omitempty"`
	Configured bool              `json:"configured"`
}

func (s *Server) faceSettings(ctx context.Context) (FaceSettings, error) {
	v := FaceSettings{BatchSize: 100, DelayMillis: 1000, IntervalMinutes: 15}
	raw, _, err := s.repo.GetSetting(ctx, faceSettingsKey)
	if err != nil {
		return v, err
	}
	if raw != "" {
		if err = json.Unmarshal([]byte(raw), &v); err != nil {
			return v, err
		}
	}
	v.BatchSize = max(1, min(1000, v.BatchSize))
	v.DelayMillis = max(100, min(60000, v.DelayMillis))
	v.IntervalMinutes = max(1, min(1440, v.IntervalMinutes))
	return v, nil
}
func (s *Server) saveFaceSettings(ctx context.Context, v FaceSettings) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err = s.repo.SaveSetting(ctx, faceSettingsKey, string(b)); err != nil {
		return err
	}
	return s.photos.SetFaceProcessingEnabled(ctx, v.Enabled)
}
func (s *Server) faceClient() (*facerec.Client, error) {
	return facerec.New(s.cfg.Photos.FaceServiceURL, s.cfg.Photos.FaceServiceToken)
}
func (s *Server) stopFaceRun() {
	s.faceWorker.mu.Lock()
	if s.faceWorker.cancel != nil {
		s.faceWorker.cancel()
	}
	s.faceWorker.mu.Unlock()
}
func (s *Server) startFaceRun() bool {
	if s.photos == nil || !s.faceWorker.run.TryLock() {
		return false
	}
	ctx, cancel := context.WithCancel(s.backgroundJobContext())
	s.faceWorker.mu.Lock()
	s.faceWorker.cancel = cancel
	s.faceWorker.running = true
	s.faceWorker.lastError = ""
	s.faceWorker.mu.Unlock()
	go func() {
		defer s.faceWorker.run.Unlock()
		defer cancel()
		err := s.processFaceBatch(ctx)
		s.faceWorker.mu.Lock()
		s.faceWorker.running = false
		s.faceWorker.lastFinished = time.Now()
		s.faceWorker.cancel = nil
		if err != nil && ctx.Err() == nil {
			s.faceWorker.lastError = err.Error()
		}
		s.faceWorker.mu.Unlock()
		select {
		case s.faceWorker.wake <- struct{}{}:
		default:
		}
	}()
	return true
}
func (s *Server) RunFaceWorker(ctx context.Context) {
	if s.photos == nil {
		return
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			s.stopFaceRun()
			return
		case <-timer.C:
		case <-s.faceWorker.wake:
		}
		settings, err := s.faceSettings(ctx)
		s.faceWorker.mu.Lock()
		running, finished := s.faceWorker.running, s.faceWorker.lastFinished
		s.faceWorker.mu.Unlock()
		start, wait := faceWorkerSchedule(settings, running, finished, time.Now())
		if err == nil && start {
			s.startFaceRun()
		}

		timer.Reset(wait)
	}
}
func (s *Server) processFaceBatch(ctx context.Context) error {
	settings, err := s.faceSettings(ctx)
	if err != nil || !settings.Enabled {
		return err
	}
	client, err := s.faceClient()
	if err != nil {
		return err
	}
	if err = client.Health(ctx); err != nil {
		return err
	}
	photoSettings, err := s.photoSettings(ctx)
	if err != nil {
		return err
	}
	if !photoSettings.IndexWorkerEnabled && time.Since(s.faceWorker.lastScan) >= time.Hour {
		if _, err = s.rebuildPhotoIndex(ctx, photoSettings); err != nil {
			return err
		}
		s.faceWorker.lastScan = time.Now()
	}
	if err = s.photos.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		return err
	}
	for n := 0; n < settings.BatchSize; n++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		current, e := s.faceSettings(ctx)
		if e != nil {
			return e
		}
		if !current.Enabled {
			return nil
		}
		job, e := s.photos.NextFaceJob(ctx)
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		if e != nil {
			return e
		}
		data, e := s.photos.FaceImage(ctx, job.Path)
		if e == nil {
			var result facerec.Result
			result, e = client.Analyze(ctx, data)
			if e == nil {
				e = s.photos.CommitFaceResult(ctx, job, result)
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e != nil {
			reason := "Bild konnte nicht analysiert werden"
			if errors.Is(e, photos.ErrAdminOnly()) {
				reason = "Foto ist geschützt"
			}
			if err = s.photos.FailFaceJob(ctx, job, reason); err != nil {
				return err
			}
			s.faceWorker.mu.Lock()
			s.faceWorker.lastError = reason
			s.faceWorker.mu.Unlock()
		}
		timer := time.NewTimer(time.Duration(settings.DelayMillis) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// The configured interval starts after completion, including long initial batches.
func faceWorkerSchedule(settings FaceSettings, running bool, finished, now time.Time) (bool, time.Duration) {
	if !settings.Enabled || running {
		return false, time.Minute
	}
	if finished.IsZero() {
		return true, time.Minute
	}
	wait := finished.Add(time.Duration(settings.IntervalMinutes) * time.Minute).Sub(now)
	if wait <= 0 {
		return true, time.Minute
	}
	return false, wait
}
