package transfers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bearstack/internal/transfers"
	"bearstack/internal/transfers/contracttest"
)

// This test-only provider uses opaque target IDs, not paths or DAV URLs.
type opaque struct {
	uncertain atomic.Bool
	mu        sync.Mutex
	objects   map[string][]byte
	dirs      map[string]bool
	calls     atomic.Int64
	uploads   atomic.Int64
	active    atomic.Int64
	peak      atomic.Int64
	failure   atomic.Bool
	delay     time.Duration
}

func newOpaque() *opaque { return &opaque{objects: map[string][]byte{}, dirs: map[string]bool{}} }
func key(base transfers.Location, parts []string) string {
	b, _ := json.Marshal(parts)
	return base.ID + ":" + string(b)
}
func (p *opaque) Info() transfers.ProviderInfo {
	return transfers.ProviderInfo{ID: "opaque", Name: "Test only"}
}
func (p *opaque) Validate(raw json.RawMessage) error { return nil }
func (p *opaque) BeginLogin(context.Context, json.RawMessage) (transfers.Login, error) {
	return transfers.Login{State: json.RawMessage(`{}`), Expires: time.Now().Add(time.Minute)}, nil
}
func (p *opaque) PollLogin(_ context.Context, config, state json.RawMessage) (*transfers.Credentials, error) {
	return &transfers.Credentials{Account: string(config), Secret: json.RawMessage(`{"token":"separate-secret-` + string(state) + `"}`)}, nil
}
func (p *opaque) Connect(context.Context, json.RawMessage, json.RawMessage) (transfers.Client, error) {
	return p, nil
}
func (p *opaque) Locations(context.Context, string) ([]transfers.Location, error) {
	return []transfers.Location{{ID: "opaque:root#42", Name: "Archive"}}, nil
}
func (p *opaque) Inventory(ctx context.Context, b transfers.Location, dir []string, emit func(transfers.Entry) error) error {
	p.calls.Add(1)
	p.mu.Lock()
	var entries []transfers.Entry
	for k, data := range p.objects {
		if !strings.HasPrefix(k, b.ID+":") {
			continue
		}
		var parts []string
		json.Unmarshal([]byte(strings.TrimPrefix(k, b.ID+":")), &parts)
		if len(parts) == len(dir)+1 && strings.Join(parts[:len(dir)], "\x00") == strings.Join(dir, "\x00") {
			entries = append(entries, transfers.Entry{Segments: parts, Size: int64(len(data))})
		}
	}
	p.mu.Unlock()
	for _, e := range entries {
		if err := emit(e); err != nil {
			return err
		}
	}
	return ctx.Err()
}
func (p *opaque) Stat(ctx context.Context, b transfers.Location, parts []string) (*transfers.Entry, error) {
	p.calls.Add(1)
	p.mu.Lock()
	defer p.mu.Unlock()
	k := key(b, parts)
	if data, ok := p.objects[k]; ok {
		return &transfers.Entry{Segments: parts, Size: int64(len(data))}, nil
	}
	if p.dirs[k] {
		return &transfers.Entry{Segments: parts, Directory: true}, nil
	}
	return nil, ctx.Err()
}
func (p *opaque) EnsureDirectories(ctx context.Context, b transfers.Location, parts []string) error {
	if err := transfers.ValidateSegments(parts); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range parts {
		k := key(b, parts[:i+1])
		if _, ok := p.objects[k]; ok {
			return transfers.Fail(transfers.Conflict, "file occupies directory")
		}
		p.dirs[k] = true
	}
	return ctx.Err()
}
func (p *opaque) CreateFile(ctx context.Context, b transfers.Location, u transfers.Upload) error {
	if err := transfers.ValidateSegments(u.Segments); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	active := p.active.Add(1)
	defer p.active.Add(-1)
	for old := p.peak.Load(); active > old && !p.peak.CompareAndSwap(old, active); old = p.peak.Load() {
	}
	if p.delay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.delay):
		}
	}
	if p.failure.Load() {
		return transfers.Fail(transfers.Quota, "full")
	}
	data, err := io.ReadAll(io.NewSectionReader(u.Body, 0, u.Size))
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	k := key(b, u.Segments)
	if _, ok := p.objects[k]; ok || p.dirs[k] {
		return transfers.Fail(transfers.Conflict, "exists")
	}
	p.objects[k] = data
	p.uploads.Add(1)
	if p.uncertain.Swap(false) {
		return transfers.Fail(transfers.Temporary, "unknown outcome")
	}
	if u.Progress != nil {
		u.Progress(int64(len(data)))
	}
	return nil
}
func (p *opaque) read(b transfers.Location, parts []string) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]byte(nil), p.objects[key(b, parts)]...)
}
func TestOpaqueProviderContract(t *testing.T) {
	contracttest.Run(t, func(t *testing.T) contracttest.Fixture {
		p := newOpaque()
		b := transfers.Location{ID: "opaque:root#42"}
		return contracttest.Fixture{Client: p, Base: b, Read: func(parts []string) []byte { return p.read(b, parts) }}
	})
}

type generatedSource struct {
	items   []transfers.SourceItem
	count   atomic.Int64
	changed atomic.Bool
	blocked chan struct{}
}

func (s *generatedSource) Describe(context.Context, transfers.Selection) (transfers.SelectionInfo, error) {
	return transfers.SelectionInfo{Name: "Selection", Target: "export", Virtual: true}, nil
}
func (s *generatedSource) Walk(ctx context.Context, sel transfers.Selection, emit func(transfers.SourceItem) error) error {
	if s.items != nil {
		for _, item := range s.items {
			if err := emit(item); err != nil {
				return err
			}
		}
		return nil
	}
	if s.blocked != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.blocked:
		}
	}
	for i := int64(0); i < s.count.Load(); i++ {
		name := fmt.Sprintf("%06d.jpg", i)
		if err := emit(transfers.SourceItem{Path: name, DisplayPath: name, Relative: []string{name}, Size: 4, Modified: 1}); err != nil {
			return err
		}
	}
	return nil
}
func (s *generatedSource) Open(ctx context.Context, sel transfers.Selection, i transfers.SourceItem) (transfers.File, error) {
	if s.changed.Load() {
		return nil, transfers.Fail(transfers.Conflict, "source changed")
	}
	return contracttest.Upload(nil, []byte("data")).Body, ctx.Err()
}

type rig struct {
	e       *transfers.Engine
	p       *opaque
	s       *generatedSource
	allowed atomic.Bool
	dir     string
}

func newRig(t *testing.T, n int64) *rig {
	t.Helper()
	r := &rig{p: newOpaque(), s: &generatedSource{}, dir: t.TempDir()}
	r.s.count.Store(n)
	r.allowed.Store(true)
	r.open(t)
	t.Cleanup(func() { r.e.Close() })
	return r
}
func (r *rig) open(t *testing.T) {
	t.Helper()
	var err error
	r.e, err = transfers.Open(r.dir, transfers.Registry{"opaque": r.p}, r.s, func(context.Context, string) bool { return r.allowed.Load() })
	if err != nil {
		t.Fatal(err)
	}
}
func (r *rig) connection(t *testing.T, name string) transfers.Connection {
	t.Helper()
	ctx := context.Background()
	c, err := r.e.SaveConnection(ctx, transfers.Connection{Name: name, Provider: "opaque", Enabled: true, Config: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	done, err := r.e.PollLogin(ctx, c.ID, c.Revision, json.RawMessage(`1`))
	if err != nil || !done {
		t.Fatalf("login %v %v", done, err)
	}
	c, _ = r.e.Connection(ctx, c.ID)
	c, err = r.e.SetBase(ctx, c.ID, c.Revision, transfers.Location{ID: "opaque:root#42"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func (r *rig) run(t *testing.T) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.e.Run(ctx); close(done) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("worker did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}
func waitJob(t *testing.T, e *transfers.Engine, id string, states ...string) transfers.Job {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		j, err := e.Job(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		for _, state := range states {
			if j.State == state {
				return j
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := e.Job(context.Background(), id)
	t.Fatalf("job never reached %v: %+v", states, j)
	return j
}
func (r *rig) preview(t *testing.T, c transfers.Connection, target string) transfers.Job {
	t.Helper()
	j, err := r.e.NewPreview(context.Background(), c.ID, "admin", transfers.Selection{}, target)
	if err != nil {
		t.Fatal(err)
	}
	return waitJob(t, r.e, j.ID, "ready")
}
func action(t *testing.T, e *transfers.Engine, id, a string) {
	t.Helper()
	if err := e.Action(context.Background(), id, a); err != nil {
		t.Fatal(err)
	}
}
func TestManifestConflictsIdempotencyAndRestart(t *testing.T) {
	r := newRig(t, 4)
	c := r.connection(t, "one")
	r.p.dirs[key(c.Base, []string{"export"})] = true
	r.p.objects[key(c.Base, []string{"export", "000000.jpg"})] = []byte("same")
	r.p.objects[key(c.Base, []string{"export", "000001.jpg"})] = []byte("different")
	stop := r.run(t)
	j := r.preview(t, c, "")
	if j.Total != 4 || j.Existing != 1 || j.Conflicts != 1 || j.Missing != 2 {
		t.Fatalf("preview %+v", j)
	}
	r.s.count.Store(5)
	stop()
	action(t, r.e, j.ID, "start")
	action(t, r.e, j.ID, "start")
	r.e.Close()
	r.open(t)
	r.run(t)
	final := waitJob(t, r.e, j.ID, "partial")
	if final.Total != 4 || final.Done != 2 || r.p.uploads.Load() != 2 {
		t.Fatalf("manifest changed or duplicate uploads %+v", final)
	}
	if !bytes.Equal(r.p.read(c.Base, []string{"export", "000000.jpg"}), []byte("same")) {
		t.Fatal("overwrote existing content")
	}
	jobs, err := r.e.Jobs(context.Background(), "", 1)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs %v %v", jobs, err)
	}
}
func TestConnectionsPauseIndependentlyAndRequireNewPreview(t *testing.T) {
	r := newRig(t, 2)
	a, b := r.connection(t, "one"), r.connection(t, "two")
	stop := r.run(t)
	ja, jb := r.preview(t, a, "a"), r.preview(t, b, "b")
	stop()
	action(t, r.e, ja.ID, "start")
	action(t, r.e, jb.ID, "start")
	a.Config = json.RawMessage(`{"changed":true}`)
	if _, err := r.e.SaveConnection(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if j, _ := r.e.Job(context.Background(), ja.ID); j.State != "paused" {
		t.Fatalf("first job not paused %+v", j)
	}
	if err := r.e.Action(context.Background(), ja.ID, "resume"); transfers.Kind(err) != transfers.Conflict {
		t.Fatalf("accepted old revision %v", err)
	}
	r.run(t)
	waitJob(t, r.e, jb.ID, "complete")
	if j, _ := r.e.Job(context.Background(), ja.ID); j.Done != 0 {
		t.Fatal("changed connection uploaded")
	}
}
func TestPreviewCannotBeResumedOrRetriedBeforeConfirmation(t *testing.T) {
	r := newRig(t, 1)
	c := r.connection(t, "one")
	r.s.blocked = make(chan struct{})
	r.run(t)
	j, err := r.e.NewPreview(context.Background(), c.ID, "admin", transfers.Selection{}, "")
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, r.e, j.ID, "planning")
	action(t, r.e, j.ID, "pause")
	if err := r.e.Action(context.Background(), j.ID, "resume"); err == nil {
		t.Fatal("resumed unconfirmed preview")
	}
	if err := r.e.Action(context.Background(), j.ID, "start"); err == nil {
		t.Fatal("started partial manifest")
	}
	close(r.s.blocked)
	if r.p.uploads.Load() != 0 {
		t.Fatal("uploaded without confirmation")
	}
}
func TestDisabledConnectionNoRemoteAndRevokedActor(t *testing.T) {
	r := newRig(t, 1)
	c := r.connection(t, "one")
	stop := r.run(t)
	j := r.preview(t, c, "")
	stop()
	action(t, r.e, j.ID, "start")
	calls := r.p.calls.Load()
	c.Enabled = false
	var err error
	c, err = r.e.SaveConnection(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.e.NewPreview(context.Background(), c.ID, "admin", transfers.Selection{}, ""); err == nil {
		t.Fatal("disabled preview accepted")
	}
	if _, err = r.e.Locations(context.Background(), c.ID, ""); err == nil {
		t.Fatal("disabled browsing accepted")
	}
	if r.p.calls.Load() != calls {
		t.Fatal("disabled remote request")
	}
	c.Enabled = true
	c, err = r.e.SaveConnection(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	r.allowed.Store(false)
	if err = r.e.Action(context.Background(), j.ID, "resume"); transfers.Kind(err) != transfers.Permission {
		t.Fatalf("revoked actor: %v", err)
	}
}
func TestChangedSourceAndConcurrency(t *testing.T) {
	r := newRig(t, 20)
	c := r.connection(t, "one")
	r.p.delay = 5 * time.Millisecond
	r.run(t)
	j := r.preview(t, c, "")
	action(t, r.e, j.ID, "start")
	waitJob(t, r.e, j.ID, "complete")
	if r.p.peak.Load() > 2 || r.p.peak.Load() < 1 {
		t.Fatalf("concurrency %d", r.p.peak.Load())
	}
	j = r.preview(t, c, "changed")
	r.s.changed.Store(true)
	action(t, r.e, j.ID, "start")
	j = waitJob(t, r.e, j.ID, "partial")
	if j.Failed != 20 {
		t.Fatalf("changed files %+v", j)
	}
}
func TestEncryptedCredentialsAndMissingKey(t *testing.T) {
	r := newRig(t, 0)
	r.connection(t, "one")
	for _, name := range []string{"transfers.db", "transfers.db-wal"} {
		b, err := os.ReadFile(filepath.Join(r.dir, name))
		if err == nil && bytes.Contains(b, []byte("separate-secret")) {
			t.Fatal("plaintext secret on disk")
		}
	}
	info, err := os.Stat(filepath.Join(r.dir, "credentials.key"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("key permissions %v %v", info, err)
	}
	r.e.Close()
	os.Remove(filepath.Join(r.dir, "credentials.key"))
	if e, err := transfers.Open(r.dir, transfers.Registry{}, r.s, func(context.Context, string) bool { return true }); err == nil {
		e.Close()
		t.Fatal("replaced missing key")
	}
}
func TestFiftyThousandManifestIsPaged(t *testing.T) {
	if testing.Short() {
		t.Skip("large manifest")
	}
	r := newRig(t, 50000)
	c := r.connection(t, "many")
	r.run(t)
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	var peak atomic.Uint64
	sampleCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-sampleCtx.Done():
				return
			case <-ticker.C:
				var current runtime.MemStats
				runtime.ReadMemStats(&current)
				if current.HeapAlloc > peak.Load() {
					peak.Store(current.HeapAlloc)
				}
			}
		}
	}()
	start := time.Now()
	j := r.preview(t, c, "")
	cancel()
	<-sampled
	delta := int64(peak.Load()) - int64(before.HeapAlloc)
	if delta > 128<<20 {
		t.Fatalf("unbounded manifest heap growth: %d bytes", delta)
	}
	t.Logf("50,000-file peak Go heap growth: %d KiB", delta/1024)
	if j.Total != 50000 || j.Bytes != 200000 {
		t.Fatalf("large preview %+v", j)
	}
	if r.p.calls.Load() > 3 {
		t.Fatalf("unbatched inventory: %d calls", r.p.calls.Load())
	}
	items, err := r.e.Items(context.Background(), j.ID, 0)
	if err != nil || len(items) != 200 {
		t.Fatalf("unbounded page %d %v", len(items), err)
	}
	t.Logf("50,000-file preview: %s", time.Since(start))
}

func TestPauseCancelAndOtherConnectionContinues(t *testing.T) {
	r := newRig(t, 8)
	a, b := r.connection(t, "pause"), r.connection(t, "continue")
	r.p.delay = 80 * time.Millisecond
	stop := r.run(t)
	ja, jb := r.preview(t, a, "pause"), r.preview(t, b, "continue")
	stop()
	action(t, r.e, ja.ID, "start")
	action(t, r.e, jb.ID, "start")
	r.run(t)
	waitJob(t, r.e, ja.ID, "running")
	action(t, r.e, ja.ID, "pause")
	waitJob(t, r.e, jb.ID, "complete")
	ja = waitJob(t, r.e, ja.ID, "paused")
	if ja.Done >= 8 {
		t.Fatal("pause ignored")
	}
	action(t, r.e, ja.ID, "resume")
	waitJob(t, r.e, ja.ID, "running")
	action(t, r.e, ja.ID, "cancel")
	waitJob(t, r.e, ja.ID, "cancelled")
	if err := r.e.Action(context.Background(), ja.ID, "resume"); err == nil {
		t.Fatal("cancelled job resumed")
	}
}
func TestAuthenticationReplacementPausesOnlyItsConnection(t *testing.T) {
	r := newRig(t, 1)
	a, b := r.connection(t, "a"), r.connection(t, "b")
	stop := r.run(t)
	ja, jb := r.preview(t, a, "a"), r.preview(t, b, "b")
	stop()
	action(t, r.e, ja.ID, "start")
	action(t, r.e, jb.ID, "start")
	done, err := r.e.PollLogin(context.Background(), a.ID, a.Revision, json.RawMessage(`2`))
	if err != nil || !done {
		t.Fatalf("login %v %v", done, err)
	}
	ja = waitJob(t, r.e, ja.ID, "paused")
	if _, err := r.e.Job(context.Background(), jb.ID); err != nil {
		t.Fatal(err)
	}
	r.run(t)
	waitJob(t, r.e, jb.ID, "complete")
	if err := r.e.Action(context.Background(), ja.ID, "resume"); err == nil {
		t.Fatal("old account job resumed")
	}
}

func TestAncestorCollisionAndCaseSensitiveSubtrees(t *testing.T) {
	r := newRig(t, 0)
	r.s.items = []transfers.SourceItem{
		{Path: "A/inside/a.jpg", Relative: []string{"A", "inside", "a.jpg"}, Size: 4},
		{Path: "a/inside/a.jpg", Relative: []string{"a", "inside", "a.jpg"}, Size: 4},
	}
	c := r.connection(t, "one")
	r.p.dirs[key(c.Base, []string{"export"})] = true
	r.p.objects[key(c.Base, []string{"export", "A"})] = []byte("existing")
	r.run(t)
	j := r.preview(t, c, "")
	if j.Conflicts != 1 || j.Missing != 1 {
		t.Fatalf("ancestor conflict: %+v", j)
	}
	action(t, r.e, j.ID, "start")
	j = waitJob(t, r.e, j.ID, "partial")
	if j.Done != 1 || j.Conflicts != 1 {
		t.Fatalf("case-sensitive upload: %+v", j)
	}
}
func TestUncertainResponseIsCheckedBeforeRetry(t *testing.T) {
	r := newRig(t, 1)
	c := r.connection(t, "one")
	r.run(t)
	j := r.preview(t, c, "")
	r.p.uncertain.Store(true)
	action(t, r.e, j.ID, "start")
	j = waitJob(t, r.e, j.ID, "waiting")
	if j.Attempts != 1 || j.RetryAt < time.Now().Unix() {
		t.Fatalf("retry policy %+v", j)
	}
	action(t, r.e, j.ID, "resume")
	j = waitJob(t, r.e, j.ID, "complete")
	if r.p.uploads.Load() != 1 || j.Existing != 1 {
		t.Fatalf("uncertain result duplicated %+v", j)
	}
}
