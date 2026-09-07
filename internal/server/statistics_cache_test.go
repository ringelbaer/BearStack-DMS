package server

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bearstack/internal/document"
)

// Signals when a caller reaches a cancellable wait, without scheduling sleeps.
type observedWaitContext struct {
	context.Context
	once    sync.Once
	waiting chan struct{}
}

func (c *observedWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func awaitTestSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for worker")
	}
}

func TestDocumentStatisticsCoalescesRequestsAndAllowsWaiterCancellation(t *testing.T) {
	var state statisticsCacheState
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	calculate := func(context.Context) (document.Statistics, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return document.Statistics{ActiveDocuments: 7}, nil
	}
	var wg sync.WaitGroup
	run := func(ctx context.Context) {
		defer wg.Done()
		stats, err := state.documentStatistics(ctx, calculate)
		if err != nil || stats.ActiveDocuments != 7 {
			t.Errorf("stats=%v error=%v", stats.ActiveDocuments, err)
		}
	}
	wg.Add(1)
	go run(context.Background())
	awaitTestSignal(t, started)
	for i := 0; i < 10; i++ {
		ctx := &observedWaitContext{Context: context.Background(), waiting: make(chan struct{})}
		wg.Add(1)
		go run(ctx)
		awaitTestSignal(t, ctx.waiting)
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	ctx := &observedWaitContext{Context: cancelCtx, waiting: make(chan struct{})}
	canceled := make(chan error, 1)
	go func() { _, err := state.documentStatistics(ctx, calculate); canceled <- err }()
	awaitTestSignal(t, ctx.waiting)
	cancel()
	if err := <-canceled; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter: %v", err)
	}
	unblock()
	wg.Wait()
	if _, err := state.documentStatistics(context.Background(), calculate); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calculations=%d, want 1", got)
	}
}

func TestDocumentStatisticsDoesNotUndoConcurrentInvalidation(t *testing.T) {
	s := &Server{}
	state := &s.apps.statistics
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = state.documentStatistics(context.Background(), func(context.Context) (document.Statistics, error) {
			close(started)
			<-release
			return document.Statistics{ActiveDocuments: 1}, nil
		})
	}()
	awaitTestSignal(t, started)
	s.invalidateDocumentCountCache()
	close(release)
	awaitTestSignal(t, done)
	stats, err := state.documentStatistics(context.Background(), func(context.Context) (document.Statistics, error) {
		return document.Statistics{ActiveDocuments: 2}, nil
	})
	if err != nil || stats.ActiveDocuments != 2 {
		t.Fatalf("stale cache: %v, %v", stats.ActiveDocuments, err)
	}
}

func TestDocumentStatisticsRetriesAfterErrorAndExpiry(t *testing.T) {
	var state statisticsCacheState
	wantErr := errors.New("statistics unavailable")
	if _, err := state.documentStatistics(context.Background(), func(context.Context) (document.Statistics, error) {
		return document.Statistics{}, wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("error=%v", err)
	}
	for _, want := range []int{2, 3} {
		stats, err := state.documentStatistics(context.Background(), func(context.Context) (document.Statistics, error) {
			return document.Statistics{ActiveDocuments: want}, nil
		})
		if err != nil || stats.ActiveDocuments != want {
			t.Fatalf("count=%d, error=%v", stats.ActiveDocuments, err)
		}
		state.documents.expiresAt = time.Now().Add(-time.Second)
	}
}
