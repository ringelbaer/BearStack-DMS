package photos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func seedCachedPhotoRoute(t testing.TB, l *Library, count int) {
	t.Helper()
	_, err := l.index.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<?)
	INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at,latitude,longitude)
	SELECT printf('trip/%d.jpg',n),printf('%d.jpg',n),'trip','image','image/jpeg',1,1700000000000000000+n*1000000000,'2026-09-09',n%2,13 FROM seq`, count-1)
	if err != nil {
		t.Fatal(err)
	}
}

func BenchmarkPhotoRouteCache(b *testing.B) {
	l := routeTestLibrary(b)
	seedCachedPhotoRoute(b, l, 100000)
	if _, err := l.index.db.Exec(`UPDATE media_index SET latitude=(rowid/1000)%2`); err != nil {
		b.Fatal(err)
	}
	world := MapBounds{-90, -180, 90, 180}
	if _, err := l.MapPhotoRoute(context.Background(), ListOptions{}, world, 4096); err != nil {
		b.Fatal(err)
	}
	for _, query := range []string{"", "gps:true"} {
		b.Run(map[bool]string{true: "cached", false: "uncached"}[query == ""], func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				route, err := l.MapPhotoRoute(context.Background(), ListOptions{Query: query}, world, 4096)
				if err != nil || route.TotalMedia != 100000 {
					b.Fatalf("route: %d %v", route.TotalMedia, err)
				}
			}
		})
	}
}

func cachedRouteFiles(t testing.TB, l *Library) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(l.cacheDir, "photo-routes", "v1", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestPhotoRouteCachePersistsCompleteRouteAndReusesDifferentViewports(t *testing.T) {
	l := routeTestLibrary(t)
	seedCachedPhotoRoute(t, l, 200)
	ctx := context.Background()
	opts := ListOptions{Path: "trip"}
	first, err := l.MapPhotoRoute(ctx, opts, MapBounds{-90, -180, 90, 180}, 32)
	if err != nil || first.TotalPoints != 200 || !first.Simplified {
		t.Fatalf("first: %+v %v", first, err)
	}
	files := cachedRouteFiles(t, l)
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}
	bytes, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Header photoRouteCacheHeader   `json:"header"`
		Points []cachedPhotoRoutePoint `json:"points"`
	}
	if err = json.Unmarshal(bytes, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Points) != 200 || stored.Header.Format != 1 || stored.Header.Algorithm != 1 {
		t.Fatalf("partial cache: %d %+v", len(stored.Points), stored.Header)
	}
	before, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != 0600 {
		t.Fatalf("permissions: %v", before.Mode())
	}
	// A cache hit must not execute the GPS source query or group it again.
	if _, err = l.index.db.Exec(`DROP INDEX idx_media_index_route_time`); err != nil {
		t.Fatal(err)
	}
	second, err := l.MapPhotoRoute(ctx, opts, MapBounds{-.1, 12.9, .1, 13.1}, 128)
	if err != nil || second.TotalPoints != 200 || second.TotalMedia != 200 {
		t.Fatalf("cached viewport: %+v %v", second, err)
	}
	after, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("cache unnecessarily replaced")
	}
	// Reopening the library retains the persistent index revision and cache.
	root, cache, db := l.root, l.cacheDir, l.dbPath
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root, cache, db, 50)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	third, err := reopened.MapPhotoRoute(ctx, opts, MapBounds{-90, -180, 90, 180}, 32)
	if err != nil || !reflect.DeepEqual(first, third) {
		t.Fatalf("restart: %+v %v", third, err)
	}
	final, _ := os.Stat(files[0])
	if !os.SameFile(before, final) {
		t.Fatal("cache lost on restart")
	}
}

func TestPhotoRouteRevisionTracksMetadataAndRecursiveVisibility(t *testing.T) {
	l := routeTestLibrary(t)
	seedCachedPhotoRoute(t, l, 2)
	ctx := context.Background()
	if err := os.Mkdir(filepath.Join(l.root, "trip", "sub"), 0750); err != nil {
		t.Fatal(err)
	}
	beforeFolder, _ := os.Stat(filepath.Join(l.root, "trip"))
	for _, statement := range []string{
		`UPDATE media_index SET latitude=2 WHERE path='trip/0.jpg'`,
		`UPDATE media_index SET longitude=14 WHERE path='trip/0.jpg'`,
		`UPDATE media_index SET captured_at='2020-01-01T00:00:00Z' WHERE path='trip/0.jpg'`,
		`UPDATE media_index SET mod_time_unix_nano=mod_time_unix_nano+1 WHERE path='trip/0.jpg'`,
		`UPDATE media_index SET type='video' WHERE path='trip/0.jpg'`,
		`UPDATE media_index SET admin_only=1 WHERE path='trip/0.jpg'`,
		`UPDATE media_index SET directory='trip/sub' WHERE path='trip/0.jpg'`,
		`DELETE FROM media_index WHERE path='trip/0.jpg'`,
	} {
		before, err := l.photoRouteRevision(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = l.index.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
		after, err := l.photoRouteRevision(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if after.Number != before.Number+1 || after.Instance != before.Instance {
			t.Fatalf("revision for %s: %+v -> %+v", statement, before, after)
		}
	}
	afterFolder, _ := os.Stat(filepath.Join(l.root, "trip"))
	if !beforeFolder.ModTime().Equal(afterFolder.ModTime()) {
		t.Fatal("test changed directory time")
	}
	before, _ := l.photoRouteRevision(ctx)
	if _, err := l.index.db.Exec(`UPDATE media_index SET latitude=latitude,tags='["new"]',rating=4`); err != nil {
		t.Fatal(err)
	}
	after, _ := l.photoRouteRevision(ctx)
	if before != after {
		t.Fatal("unrelated metadata invalidates cache")
	}
	tx, err := l.index.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE media_index SET longitude=77`); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	after, _ = l.photoRouteRevision(ctx)
	if before != after {
		t.Fatal("rolled back write invalidates cache")
	}
	world := MapBounds{-90, -180, 90, 180}
	if _, err := l.MapPhotoRoute(ctx, ListOptions{}, world, 32); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "trip", ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	public, err := l.MapPhotoRoute(ctx, ListOptions{}, world, 32)
	if err != nil || public.TotalMedia != 0 {
		t.Fatalf("new private folder in cached route: %+v %v", public, err)
	}
	admin, err := l.MapPhotoRoute(ctx, ListOptions{IncludeAdminOnly: true}, world, 32)
	if err != nil || admin.TotalMedia != 1 {
		t.Fatalf("admin scope: %+v %v", admin, err)
	}
	if len(cachedRouteFiles(t, l)) != 2 {
		t.Fatal("permission scopes not separated")
	}
}

func TestPhotoRouteCacheSeparatesParametersSkipsSearchAndRepairsCorruption(t *testing.T) {
	l := routeTestLibrary(t)
	seedCachedPhotoRoute(t, l, 10)
	ctx := context.Background()
	world := MapBounds{-90, -180, 90, 180}
	for _, opts := range []ListOptions{{}, {RouteClusterRadiusMeters: 500}, {MediaType: "video"}, {Path: "trip"}} {
		if _, err := l.MapPhotoRoute(ctx, opts, world, 32); err != nil {
			t.Fatal(err)
		}
	}
	if len(cachedRouteFiles(t, l)) != 4 {
		t.Fatal("parameter scopes not separated")
	}
	if _, err := l.MapPhotoRoute(ctx, ListOptions{Query: "type:image"}, world, 32); err != nil {
		t.Fatal(err)
	}
	if len(cachedRouteFiles(t, l)) != 4 {
		t.Fatal("search created a persistent file")
	}
	query, err := l.mapQuery(ctx, ListOptions{}, world)
	if err != nil {
		t.Fatal(err)
	}
	revision, _ := l.photoRouteRevision(ctx)
	name, _ := l.photoRouteCacheKey(query, 1000, revision)
	bytes, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err = json.Unmarshal(bytes, &envelope); err != nil {
		t.Fatal(err)
	}
	var header photoRouteCacheHeader
	if err = json.Unmarshal(envelope["header"], &header); err != nil {
		t.Fatal(err)
	}
	header.Algorithm++
	envelope["header"], _ = json.Marshal(header)
	obsolete, _ := json.Marshal(envelope)
	if err = os.WriteFile(name, obsolete, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = l.MapPhotoRoute(ctx, ListOptions{}, world, 32); err != nil {
		t.Fatal(err)
	}
	if file, e := openPhotoRouteCache(name, photoRouteCacheHeader{Format: photoRouteFormatVersion, Algorithm: photoRouteAlgorithmVersion, Revision: revision, Scope: header.Scope}); e != nil {
		t.Fatal("algorithm version not replaced", e)
	} else {
		file.Close()
	}
	if err = os.WriteFile(name, bytes[:len(bytes)-10], 0600); err != nil {
		t.Fatal(err)
	}
	route, err := l.MapPhotoRoute(ctx, ListOptions{}, world, 32)
	if err != nil || route.TotalMedia != 10 {
		t.Fatalf("corrupt cache: %+v %v", route, err)
	}
	// A failed cache write preserves the complete uncached route.
	blocked := filepath.Join(t.TempDir(), "file")
	if err = os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	l.cacheDir = blocked
	uncached, err := l.MapPhotoRoute(ctx, ListOptions{}, world, 32)
	if err != nil || !reflect.DeepEqual(route, uncached) {
		t.Fatalf("unwritable cache: %+v %v", uncached, err)
	}
}

func TestPhotoRouteCacheSharesWorkAndCancelsOnlyAfterLastSubscriber(t *testing.T) {
	var state photoRouteCacheState
	defer state.close()
	var starts atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	stopped := make(chan struct{})
	build := func(ctx context.Context) error {
		starts.Add(1)
		close(started)
		defer close(stopped)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() { first <- state.run(ctx, "same", build) }()
	<-started
	go func() { second <- state.run(context.Background(), "same", build) }()
	deadline := time.After(5 * time.Second)
	for {
		state.mu.Lock()
		n := state.flights["same"].waiters
		state.mu.Unlock()
		if n == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("second subscriber missing")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-stopped:
		t.Fatal("cancelled another subscriber's work")
	default:
	}
	close(release)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if starts.Load() != 1 {
		t.Fatal("duplicate computation")
	}
	last, cancelLast := context.WithCancel(context.Background())
	lastStarted := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- state.run(last, "last", func(ctx context.Context) error { close(lastStarted); <-ctx.Done(); return ctx.Err() })
	}()
	<-lastStarted
	cancelLast()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPhotoRouteCacheDoesNotPublishStaleRevision(t *testing.T) {
	l := routeTestLibrary(t)
	seedCachedPhotoRoute(t, l, 200)
	ctx := context.Background()
	one, _ := l.acquireMapGeometry(ctx)
	two, _ := l.acquireMapGeometry(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	var result MapPhotoRoute
	var err error
	go func() {
		defer wg.Done()
		result, err = l.MapPhotoRoute(ctx, ListOptions{}, MapBounds{-90, -180, 90, 180}, 32)
	}()
	deadline := time.After(5 * time.Second)
	for {
		l.photoRoutes.mu.Lock()
		n := len(l.photoRoutes.flights)
		l.photoRoutes.mu.Unlock()
		if n == 1 {
			break
		}
		select {
		case <-deadline:
			one()
			two()
			t.Fatal("cache computation missing")
		case <-time.After(time.Millisecond):
		}
	}
	if _, e := l.index.db.Exec(`UPDATE media_index SET admin_only=1`); e != nil {
		one()
		two()
		t.Fatal(e)
	}
	one()
	two()
	wg.Wait()
	if err != nil || result.TotalMedia != 0 {
		t.Fatalf("stale result: %+v %v", result, err)
	}
	files := cachedRouteFiles(t, l)
	if len(files) != 1 {
		t.Fatalf("files: %v", files)
	}
	bytes, _ := os.ReadFile(files[0])
	var stored struct {
		Header photoRouteCacheHeader `json:"header"`
	}
	if e := json.Unmarshal(bytes, &stored); e != nil {
		t.Fatal(e)
	}
	current, _ := l.photoRouteRevision(ctx)
	if stored.Header.Revision != current {
		t.Fatal("published stale revision")
	}
}

func TestPhotoRouteCacheEvictsOldFilesWithinDiskAndEntryBudget(t *testing.T) {
	l := routeTestLibrary(t)
	dir := filepath.Join(l.cacheDir, "photo-routes", "v1")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	var keep string
	for i := 0; i < photoRouteCacheEntries+2; i++ {
		keep = filepath.Join(dir, fmt.Sprintf("%064x.json", i))
		if err := os.WriteFile(keep, []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	l.prunePhotoRouteCache(keep)
	if len(cachedRouteFiles(t, l)) != photoRouteCacheEntries {
		t.Fatal("entry budget")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("evicted current file")
	}
	large := filepath.Join(dir, fmt.Sprintf("%064x.json", 9999))
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(photoRouteCacheBytes); err != nil {
		t.Fatal(err)
	}
	file.Close()
	l.prunePhotoRouteCache(large)
	if files := cachedRouteFiles(t, l); len(files) != 1 || files[0] != large {
		t.Fatalf("byte budget: %v", files)
	}
}

func TestPhotoRouteCacheCancellationAtEndDoesNotInvalidateValidFile(t *testing.T) {
	l := routeTestLibrary(t)
	ctx := context.Background()
	world := MapBounds{-90, -180, 90, 180}
	if _, err := l.MapPhotoRoute(ctx, ListOptions{}, world, 32); err != nil {
		t.Fatal(err)
	}
	query, err := l.mapQuery(ctx, ListOptions{}, world)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := l.photoRouteRevision(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name, header := l.photoRouteCacheKey(query, 1000, revision)
	file, err := openPhotoRouteCache(name, header)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	err = readPhotoRouteCache(cancelled, file, func(routeCluster) error { return nil })
	if !errors.Is(err, context.Canceled) || errors.Is(err, errPhotoRouteCacheCorrupt) {
		t.Fatalf("cancelled cache mistaken for corruption: %v", err)
	}
}
