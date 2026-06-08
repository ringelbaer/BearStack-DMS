// Datei koordiniert Laufzeitverhalten und Hintergrundverarbeitung der Thumbnail-Erzeugung.
package photos

import (
	"context"
	"sync"
)

type thumbnailRuntime struct {
	concurrency int
	sem         chan struct{}
	flights     map[string]*thumbnailFlight
	mu          sync.Mutex
}

type thumbnailFlight struct {
	done chan struct{}
	path string
	err  error
}

func newThumbnailRuntime(concurrency int) thumbnailRuntime {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 4 {
		concurrency = 4
	}
	return thumbnailRuntime{
		concurrency: concurrency,
		sem:         make(chan struct{}, concurrency),
		flights:     map[string]*thumbnailFlight{},
	}
}

func (r *thumbnailRuntime) setConcurrency(concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 4 {
		concurrency = 4
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.concurrency == concurrency && r.sem != nil {
		return
	}
	r.concurrency = concurrency
	r.sem = make(chan struct{}, concurrency)
	if r.flights == nil {
		r.flights = map[string]*thumbnailFlight{}
	}
}

func (r *thumbnailRuntime) flight(ctx context.Context, key string, fn func() (string, error)) (string, error) {
	r.mu.Lock()
	if r.flights == nil {
		r.flights = map[string]*thumbnailFlight{}
	}
	if flight := r.flights[key]; flight != nil {
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-flight.done:
			return flight.path, flight.err
		}
	}
	flight := &thumbnailFlight{done: make(chan struct{})}
	r.flights[key] = flight
	r.mu.Unlock()

	flight.path, flight.err = fn()

	r.mu.Lock()
	delete(r.flights, key)
	close(flight.done)
	r.mu.Unlock()
	return flight.path, flight.err
}

func (r *thumbnailRuntime) acquireSlot(ctx context.Context) (func(), error) {
	r.mu.Lock()
	if r.concurrency < 1 {
		r.concurrency = 1
	}
	if r.sem == nil {
		r.sem = make(chan struct{}, r.concurrency)
	}
	sem := r.sem
	r.mu.Unlock()

	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
