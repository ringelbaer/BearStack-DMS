package photos

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type cancelGPXReader struct {
	cancel context.CancelFunc
	reader io.Reader
}

func (r cancelGPXReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.cancel()
	return n, err
}

func TestGPXLimitsAndCancellation(t *testing.T) {
	xml := `<gpx><trkpt lat="1" lon="2"/><trkpt lat="1" lon="2"/></gpx>`
	if points, err := decodeGPX(context.Background(), strings.NewReader(xml), int64(len(xml)), 2); err != nil || len(points) != 1 {
		t.Fatalf("at limit: %v %v", points, err)
	}
	for _, input := range []struct {
		bytes  int64
		points int
	}{{int64(len(xml) - 1), 2}, {int64(len(xml)), 1}} {
		if _, err := decodeGPX(context.Background(), strings.NewReader(xml), input.bytes, input.points); !errors.Is(err, errGPXLimit) {
			t.Fatalf("limits %+v: %v", input, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := decodeGPX(ctx, cancelGPXReader{cancel, strings.NewReader(xml)}, 1024, 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-read cancel = %v", err)
	}
	if _, err := decodeGPX(context.Background(), strings.NewReader(`<gpx><trkpt`), 1024, 100); err == nil {
		t.Fatal("accepted malformed XML")
	}
}

func TestGPXOversizedFilesDoNotEnterCache(t *testing.T) {
	l := newTestLibrary(t, t.TempDir())
	defer l.Close()
	path := filepath.Join(l.Root(), "large.gpx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(gpxMaxBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := l.gpxFromPathInfo(context.Background(), "large.gpx", nil); !errors.Is(err, errGPXLimit) {
		t.Fatalf("oversized file = %v", err)
	}
	if len(l.gpxCache) != 0 || l.gpxCacheBytes != 0 {
		t.Fatal("oversized file was cached")
	}
}

func TestGPXWaitingParserCanCancel(t *testing.T) {
	l := newTestLibrary(t, t.TempDir())
	defer l.Close()
	if err := os.WriteFile(filepath.Join(l.Root(), "track.gpx"), []byte(`<gpx/>`), 0600); err != nil {
		t.Fatal(err)
	}
	l.gpxParseGate = make(chan struct{}, 1)
	l.gpxParseGate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan error, 1)
	go func() { _, err := l.gpxFromPathInfo(ctx, "track.gpx", nil); ready <- err }()
	cancel()
	if err := <-ready; !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting parser = %v", err)
	}
}

func TestGPXListingHasIndependentPointAndTrackBudgets(t *testing.T) {
	var listing Listing
	track := GPXTrack{Points: make([]GPXPoint, gpxMaxPoints)}
	for i := 0; i < 10; i++ {
		listing.addGPXTrack(track)
	}
	if len(listing.GPXTracks) != 2 || listing.gpxPointCount > gpxListingMaxPoints {
		t.Fatal("listing point budget exceeded")
	}
	listing = Listing{}
	for i := 0; i < gpxListingMaxTracks+1; i++ {
		listing.addGPXTrack(GPXTrack{Points: []GPXPoint{{Lat: 1}}})
	}
	if len(listing.GPXTracks) != gpxListingMaxTracks {
		t.Fatal("listing track budget exceeded")
	}
}

func TestGPXCacheEvictsLeastRecentlyUsedWithinByteBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "track.gpx")
	if err := os.WriteFile(path, []byte("track"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	l := &Library{}
	track := GPXTrack{Points: make([]GPXPoint, 1, 10)}
	const budget = 900 // two entries including backing-array capacity and overhead
	l.gpxStoreCacheBudget("a", info, track, budget)
	l.gpxStoreCacheBudget("b", info, track, budget)
	if _, ok := l.gpxFromCache("a", info); !ok {
		t.Fatal("missing cache entry")
	}
	l.gpxStoreCacheBudget("c", info, track, budget)
	if _, ok := l.gpxFromCache("b", info); ok {
		t.Fatal("did not evict least recently used entry")
	}
	if l.gpxCacheBytes > budget || len(l.gpxCache) != 2 {
		t.Fatalf("cache cost=%d entries=%d", l.gpxCacheBytes, len(l.gpxCache))
	}
	l.gpxStoreCacheBudget("a", info, GPXTrack{Points: make([]GPXPoint, 100)}, budget)
	if _, ok := l.gpxFromCache("a", info); ok {
		t.Fatal("stored entry larger than budget")
	}
	l.gpxInvalidateCache("c")
	if l.gpxCacheBytes != 0 || l.gpxLRU.Len() != 0 {
		t.Fatal("cache accounting leaked")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 20; n++ {
				l.gpxStoreCacheBudget("shared", info, track, budget)
				l.gpxFromCache("shared", info)
				l.gpxInvalidateCache("shared")
			}
		}()
	}
	wg.Wait()
	if l.gpxCacheBytes != 0 {
		t.Fatal("concurrent cache accounting leaked")
	}
}
