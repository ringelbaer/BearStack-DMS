package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"bearstack/internal/photos"
)

func TestShutdownWaitsForWorkerCleanupBeforeClosingPhotoDB(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("photo"), 0600); err != nil {
		t.Fatal(err)
	}
	library, err := photos.New(root, t.TempDir(), filepath.Join(t.TempDir(), "photos.db"), 20)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	s := &Server{photos: library}
	ctx := s.backgroundJobContext()
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	s.background.start(func() {
		<-ctx.Done()
		close(entered)
		<-release
		_, err := library.RebuildIndex(context.Background())
		finished <- err
	})
	deadline, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.Shutdown(deadline); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown = %v", err)
	}
	<-entered
	if s.background.start(func() { t.Error("job started after shutdown") }) {
		t.Fatal("accepted new job")
	}
	if _, err := library.RebuildIndex(context.Background()); err != nil {
		t.Fatalf("DB closed while worker active: %v", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatalf("worker cleanup could not use DB: %v", err)
	}
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := library.RebuildIndex(context.Background()); err == nil {
		t.Fatal("photo DB was not closed")
	}
}

func TestBackgroundTasksStopCannotMissConcurrentRegistrations(t *testing.T) {
	var tasks backgroundTasks
	var registrations sync.WaitGroup
	release := make(chan struct{})
	for i := 0; i < 100; i++ {
		registrations.Add(1)
		go func() { defer registrations.Done(); tasks.start(func() { <-release }) }()
	}
	tasks.stop()
	registrations.Wait()
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := tasks.wait(ctx); err != nil {
		t.Fatal(err)
	}
	tasks.mu.Lock()
	defer tasks.mu.Unlock()
	if tasks.active != 0 {
		t.Fatalf("active tasks = %d", tasks.active)
	}
}

func TestBackgroundWorkersShutdownCancelsAndAwaitsWorkers(t *testing.T) {
	s := &Server{}
	started, release := make(chan struct{}), make(chan struct{})
	workers := BackgroundWorkers{server: s, runDocumentPostImport: func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		<-release
	}}
	workers.Start(context.Background())
	<-started
	done := make(chan error, 1)
	go func() { done <- s.Shutdown(context.Background()) }()
	<-s.backgroundJobContext().Done()
	select {
	case err := <-done:
		t.Fatalf("returned before cleanup: %v", err)
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
