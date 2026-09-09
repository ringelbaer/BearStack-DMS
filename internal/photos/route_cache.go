package photos

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	photoRouteFormatVersion          = 1
	photoRouteAlgorithmVersion       = 1
	photoRouteCacheBytes       int64 = 512 << 20
	photoRouteCacheEntries           = 256
)

var (
	errPhotoRouteChanged          = errors.New("photo route index changed")
	errPhotoRouteCacheCorrupt     = errors.New("invalid photo route cache")
	errPhotoRouteCacheUnavailable = errors.New("photo route cache unavailable")
)

type photoRouteScope struct {
	Library          string `json:"library"`
	Path             string `json:"path"`
	MediaType        string `json:"media_type"`
	IncludeAdminOnly bool   `json:"include_admin_only"`
	Radius           int    `json:"radius_meters"`
}

type photoRouteCacheHeader struct {
	Format    int                `json:"format_version"`
	Algorithm int                `json:"algorithm_version"`
	Revision  photoRouteRevision `json:"index_revision"`
	Scope     photoRouteScope    `json:"scope"`
}

type cachedPhotoRoutePoint struct {
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Started   time.Time `json:"started_at"`
	Ended     time.Time `json:"ended_at"`
	Count     int       `json:"count"`
}

type photoRouteCacheFile struct {
	*os.File
	decoder *json.Decoder
	input   *routeCacheReader
}

// Bound a damaged header or individual JSON value before Decoder can grow its
// buffer with the whole cache file. Valid points contain only five small fields.
type routeCacheReader struct {
	reader      io.Reader
	read, limit int64
	ctx         context.Context
}

func (r *routeCacheReader) Read(p []byte) (int, error) {
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
	}
	if r.read >= r.limit {
		return 0, io.EOF
	}
	if int64(len(p)) > r.limit-r.read {
		p = p[:r.limit-r.read]
	}
	n, err := r.reader.Read(p)
	r.read += int64(n)
	return n, err
}

type photoRouteFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	err     error
}

type photoRouteCacheState struct {
	mu      sync.Mutex
	workers sync.WaitGroup
	flights map[string]*photoRouteFlight
	closed  bool
}

// A cancelled subscriber leaves the shared computation running for other
// subscribers. Once the last leaves, its SQL query and temporary write stop.
func (s *photoRouteCacheState) run(ctx context.Context, key string, build func(context.Context) error) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return context.Canceled
	}
	f := s.flights[key]
	if f == nil {
		if len(s.flights) >= 32 {
			s.mu.Unlock()
			return errPhotoRouteCacheUnavailable
		}
		if s.flights == nil {
			s.flights = make(map[string]*photoRouteFlight)
		}
		work, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		f = &photoRouteFlight{done: make(chan struct{}), cancel: cancel}
		s.flights[key] = f
		s.workers.Add(1)
		go func() {
			defer s.workers.Done()
			err := build(work)
			cancel()
			s.mu.Lock()
			f.err = err
			if s.flights[key] == f {
				delete(s.flights, key)
			}
			close(f.done)
			s.mu.Unlock()
		}()
	}
	f.waiters++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		f.waiters--
		if f.waiters == 0 {
			f.cancel()
			if s.flights[key] == f {
				delete(s.flights, key)
			}
		}
		s.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.done:
		return f.err
	}
}

func (s *photoRouteCacheState) close() {
	s.mu.Lock()
	s.closed = true
	flights := make([]*photoRouteFlight, 0, len(s.flights))
	for _, f := range s.flights {
		f.cancel()
		flights = append(flights, f)
	}
	s.mu.Unlock()
	for _, f := range flights {
		<-f.done
	}
	s.workers.Wait()
}

func (l *Library) photoRouteCacheKey(query indexMediaOptions, radius int, revision photoRouteRevision) (string, photoRouteCacheHeader) {
	root := sha256.Sum256([]byte(l.root + "\x00" + l.dbPath))
	header := photoRouteCacheHeader{Format: photoRouteFormatVersion, Algorithm: photoRouteAlgorithmVersion,
		Scope: photoRouteScope{Library: hex.EncodeToString(root[:]), Path: query.Directory, MediaType: query.MediaType, IncludeAdminOnly: query.IncludeAdminOnly, Radius: radius}}
	// Revision lives inside the atomically replaced file, never in its name.
	// Changes therefore do not accumulate another file for every index update.
	keyBytes, _ := json.Marshal(header)
	key := sha256.Sum256(keyBytes)
	header.Revision = revision
	return filepath.Join(l.cacheDir, "photo-routes", "v1", hex.EncodeToString(key[:])+".json"), header
}

func (l *Library) photoRouteSource(ctx context.Context, query indexMediaOptions, radius int, revision photoRouteRevision) (*photoRouteCacheFile, error) {
	if query.Query != "" || l.cacheDir == "" {
		return nil, nil
	}
	name, header := l.photoRouteCacheKey(query, radius, revision)
	if cached, err := openPhotoRouteCache(name, header); err == nil {
		return cached, nil
	}
	flightKey := fmt.Sprintf("%s:%s:%d", name, revision.Instance, revision.Number)
	err := l.photoRoutes.run(ctx, flightKey, func(work context.Context) error {
		if cached, e := openPhotoRouteCache(name, header); e == nil {
			cached.Close()
			return nil
		}
		return l.buildPhotoRouteCache(work, name, header, query)
	})
	if errors.Is(err, errPhotoRouteCacheUnavailable) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cached, err := openPhotoRouteCache(name, header)
	// A concurrent cache eviction or an unavailable filesystem must not make
	// the map unavailable. The caller can still stream the complete SQL route.
	if err != nil {
		return nil, nil
	}
	return cached, nil
}

// Shared by native geometry and browser route markers. Consumers must discard
// their accumulated output and retry when the revision changes or a file is
// repaired; partial geometry must never escape a failed attempt.
func (l *Library) consumePhotoRoute(ctx context.Context, query indexMediaOptions, radius int, emit func(routeCluster) error) error {
	revision, err := l.scopedPhotoRouteRevision(ctx, query)
	if err != nil {
		return err
	}
	source, err := l.photoRouteSource(ctx, query, radius, revision)
	if err != nil {
		return err
	}
	if source != nil {
		defer source.Close()
	}
	release, err := l.acquireMapGeometry(ctx)
	if err != nil {
		return err
	}
	defer release()
	if source != nil {
		err = readPhotoRouteCache(ctx, source, emit)
		if errors.Is(err, errPhotoRouteCacheCorrupt) {
			l.removePhotoRouteCache(source)
		}
	} else {
		err = l.scanPhotoRoute(ctx, query, radius, emit)
	}
	if err != nil {
		return err
	}
	current, err := l.scopedPhotoRouteRevision(ctx, query)
	if err != nil {
		return err
	}
	if current != revision {
		return errPhotoRouteChanged
	}
	return ctx.Err()
}

func cacheToken(decoder *json.Decoder, want any) error {
	token, err := decoder.Token()
	if err != nil || token != want {
		return errPhotoRouteCacheCorrupt
	}
	return nil
}

func openPhotoRouteCache(name string, want photoRouteCacheHeader) (*photoRouteCacheFile, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > photoRouteCacheBytes {
		return nil, errPhotoRouteCacheCorrupt
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	input := &routeCacheReader{reader: file, limit: 64 << 10}
	decoder := json.NewDecoder(bufio.NewReaderSize(input, 64<<10))
	decoder.DisallowUnknownFields()
	var got photoRouteCacheHeader
	if cacheToken(decoder, json.Delim('{')) != nil || cacheToken(decoder, "header") != nil || decoder.Decode(&got) != nil || got != want ||
		cacheToken(decoder, "points") != nil || cacheToken(decoder, json.Delim('[')) != nil {
		file.Close()
		return nil, errPhotoRouteCacheCorrupt
	}
	input.limit = photoRouteCacheBytes + 1
	return &photoRouteCacheFile{File: file, decoder: decoder, input: input}, nil
}

func readPhotoRouteCache(ctx context.Context, file *photoRouteCacheFile, emit func(routeCluster) error) error {
	d := file.decoder
	file.input.ctx = ctx
	var previous time.Time
	for d.More() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var point cachedPhotoRoutePoint
		file.input.limit = file.input.read + 4096
		if d.Decode(&point) != nil || point.Count <= 0 || point.Started.IsZero() || point.Ended.Before(point.Started) || point.Started.Before(previous) ||
			math.IsNaN(point.Latitude) || math.IsNaN(point.Longitude) || math.Abs(point.Latitude) > 90 || math.Abs(point.Longitude) > 180 {
			if err := ctx.Err(); err != nil {
				return err
			}
			return errPhotoRouteCacheCorrupt
		}
		previous = point.Started
		file.input.limit = photoRouteCacheBytes + 1
		if err := emit(routeCluster{point.Latitude, point.Longitude, point.Started, point.Ended, point.Count}); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if cacheToken(d, json.Delim(']')) != nil || cacheToken(d, json.Delim('}')) != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		return errPhotoRouteCacheCorrupt
	}
	if _, err := d.Token(); err != io.EOF {
		if err := ctx.Err(); err != nil {
			return err
		}
		return errPhotoRouteCacheCorrupt
	}
	info, err := file.Stat()
	if err == nil && time.Since(info.ModTime()) > time.Minute {
		_ = os.Chtimes(file.Name(), time.Now(), time.Now())
	}
	return ctx.Err()
}

func (l *Library) removePhotoRouteCache(file *photoRouteCacheFile) {
	l.photoRoutes.mu.Lock()
	defer l.photoRoutes.mu.Unlock()
	old, err := file.Stat()
	current, e := os.Lstat(file.Name())
	if err == nil && e == nil && os.SameFile(old, current) {
		_ = os.Remove(file.Name())
	}
}

type routeCacheWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *routeCacheWriter) Write(bytes []byte) (int, error) {
	if int64(len(bytes)) > w.remaining {
		return 0, errPhotoRouteCacheUnavailable
	}
	n, err := w.writer.Write(bytes)
	w.remaining -= int64(n)
	return n, err
}

func (l *Library) buildPhotoRouteCache(ctx context.Context, name string, header photoRouteCacheHeader, query indexMediaOptions) error {
	release, err := l.acquireMapGeometry(ctx)
	if err != nil {
		return err
	}
	defer release()
	unavailable := func(err error) error { return errors.Join(errPhotoRouteCacheUnavailable, err) }
	if err = os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return unavailable(err)
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".route-*")
	if err != nil {
		return unavailable(err)
	}
	defer func() { file.Close(); os.Remove(file.Name()) }()
	writer := bufio.NewWriterSize(&routeCacheWriter{file, photoRouteCacheBytes}, 64<<10)
	encoder := json.NewEncoder(writer)
	if _, err = writer.WriteString(`{"header":`); err != nil {
		return unavailable(err)
	}
	if err = encoder.Encode(header); err != nil {
		return unavailable(err)
	}
	if _, err = writer.WriteString(`,"points":[`); err != nil {
		return unavailable(err)
	}
	first := true
	err = l.scanPhotoRoute(ctx, query, header.Scope.Radius, func(c routeCluster) error {
		if !first {
			if e := writer.WriteByte(','); e != nil {
				return unavailable(e)
			}
		}
		first = false
		if e := encoder.Encode(cachedPhotoRoutePoint{c.lat, c.lon, c.started, c.ended, c.count}); e != nil {
			return unavailable(e)
		}
		return ctx.Err()
	})
	if err != nil {
		return err
	}
	if _, err = writer.WriteString(`]}`); err != nil {
		return unavailable(err)
	}
	if err = writer.Flush(); err != nil {
		return unavailable(err)
	}
	if err = file.Sync(); err != nil {
		return unavailable(err)
	}
	if err = file.Close(); err != nil {
		return unavailable(err)
	}
	current, err := l.scopedPhotoRouteRevision(ctx, query)
	if err != nil {
		return err
	}
	if current != header.Revision {
		return errPhotoRouteChanged
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	l.photoRoutes.mu.Lock()
	defer l.photoRoutes.mu.Unlock()
	if err = os.Rename(file.Name(), name); err != nil {
		return unavailable(err)
	}
	l.prunePhotoRouteCache(name)
	return nil
}

// Separate from thumbnails: bounded file count and disk use, no search keys and
// no per-revision filenames. A route larger than the budget uses the SQL stream.
func (l *Library) prunePhotoRouteCache(keep string) {
	entries, err := os.ReadDir(filepath.Dir(keep))
	if err != nil {
		return
	}
	type entry struct {
		name string
		info os.FileInfo
	}
	files := make([]entry, 0, len(entries))
	var bytes int64
	for _, item := range entries {
		info, e := item.Info()
		if e != nil || !info.Mode().IsRegular() {
			continue
		}
		name := filepath.Join(filepath.Dir(keep), item.Name())
		if strings.HasPrefix(item.Name(), ".route-") && time.Since(info.ModTime()) > time.Hour {
			_ = os.Remove(name)
			continue
		}
		if len(item.Name()) != 69 || !strings.HasSuffix(item.Name(), ".json") {
			continue
		}
		bytes += info.Size()
		files = append(files, entry{name, info})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].info.ModTime().Before(files[j].info.ModTime()) })
	count := len(files)
	for _, file := range files {
		if bytes <= photoRouteCacheBytes && count <= photoRouteCacheEntries {
			break
		}
		if file.name != keep && os.Remove(file.name) == nil {
			bytes -= file.info.Size()
			count--
		}
	}
}
