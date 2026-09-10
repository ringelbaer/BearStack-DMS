package photos

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"
	"testing"
	"time"

	"bearstack/internal/facerec"
	"bearstack/internal/sqlutil"
)

func TestFaceSuggestionPerformanceStages(t *testing.T) {
	if os.Getenv("BEARSTACK_FACE_SUGGESTION_PERF") != "1" {
		t.Skip("set BEARSTACK_FACE_SUGGESTION_PERF=1 for stage measurements")
	}
	l := faceSuggestionPerfFixture(t, 1000)
	ctx := context.Background()
	if profilePath := os.Getenv("BEARSTACK_FACE_SUGGESTION_CPU"); profilePath != "" {
		f, err := os.Create(profilePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			f.Close()
			t.Fatal(err)
		}
		defer func() { pprof.StopCPUProfile(); f.Close() }()
	}
	l.faceRuntime.mu.Lock()
	defer l.faceRuntime.mu.Unlock()
	start := time.Now()
	if err := l.ensureFaceGraph(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	t.Logf("load_cache=%s", time.Since(start))
	start = time.Now()
	ranked, err := l.faceRuntime.rankFacePersons(ctx, faceDetection(0).Embedding, map[int64]bool{1: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rank=%s", time.Since(start))
	start = time.Now()
	candidates, err := l.validateFacePersonCandidates(ctx, l.index.db, faceDetection(0).Embedding, ranked, 20)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("validate=%s", time.Since(start))
	start = time.Now()
	ordered, err := l.validateFacePersonCandidates(ctx, faceSuggestionUnorderedQuery{l.index.db}, faceDetection(0).Embedding, ranked, 20)
	if err != nil || !slices.Equal(ordered, candidates) {
		t.Fatalf("historical query changed results: %v", err)
	}
	t.Logf("validate_without_forced_order=%s", time.Since(start))
	start = time.Now()
	if _, err := l.faceSuggestionPeople(ctx, 1, 1, candidates, DefaultFaceThresholds()); err != nil {
		t.Fatal(err)
	}
	t.Logf("assemble=%s", time.Since(start))
	args := []any{}
	for _, candidate := range candidates {
		for _, id := range l.faceRuntime.nodes[candidate.person] {
			args = append(args, id)
		}
	}
	args = args[:min(512, len(args))]
	q := `SELECT f.id,f.person_id,f.path,p.name_source FROM photo_faces f
 JOIN photo_people p ON p.id=f.person_id JOIN media_index m ON m.path=f.path
 WHERE f.id IN (` + sqlutil.Placeholders(len(args)) + `) AND f.ignored=0 AND m.admin_only=0
 AND (f.favorite=1 OR coalesce(f.reference_eligible,1)=1)`
	rows, err := l.index.db.Query("EXPLAIN QUERY PLAN "+q, args...)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b, c int
		var detail string
		if err := rows.Scan(&a, &b, &c, &detail); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		t.Log(detail)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// Historical query-plan comparison; only this test removes the forced order.
type faceSuggestionUnorderedQuery struct{ db *sql.DB }

func (q faceSuggestionUnorderedQuery) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query = strings.ReplaceAll(query, " CROSS JOIN ", " JOIN ")
	return q.db.QueryContext(ctx, query, args...)
}

// Opt-in audit of the actual modal search, including SQLite, live directory
// checks, streaming snapshots and the shared reference cache. Only the source
// image is real; candidate metadata/vectors and directories are synthetic.
// No inference, network, thumbnail rendering or production data is involved.
func TestFaceSuggestionPerformance(t *testing.T) {
	if os.Getenv("BEARSTACK_FACE_SUGGESTION_PERF") != "1" {
		t.Skip("set BEARSTACK_FACE_SUGGESTION_PERF=1 for the modal performance audit")
	}
	for _, groups := range []int{1000, 10000} {
		t.Run(fmt.Sprintf("groups_%d", groups), func(t *testing.T) {
			l := faceSuggestionPerfFixture(t, groups)
			ctx := context.Background()
			measure := func(name string, stream bool, repeats int) {
				t.Helper()
				want := 20
				if strings.Contains(name, "no_hits") || strings.Contains(name, "positive_margin") {
					want = 0
				}
				var elapsed, first []float64
				var frames, size, alloc uint64
				completed := 0
				for range repeats {
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					start := time.Now()
					requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					var firstAt time.Duration
					var out PeopleSuggestions
					var err error
					if stream {
						out, err = l.SuggestPeopleForFaceStream(requestCtx, 1, func(p PeopleSuggestions) error {
							if firstAt == 0 {
								firstAt = time.Since(start)
							}
							data, e := json.Marshal(p)
							size += uint64(len(data))
							frames++
							return e
						})
					} else {
						out, err = l.SuggestPeopleForFace(requestCtx, 1)
					}
					cancel()
					if (err != nil && !errors.Is(err, context.DeadlineExceeded)) || len(out.People) > 20 {
						t.Fatalf("%s: %+v %v", name, out, err)
					}
					if err == nil {
						if len(out.People) != want {
							t.Fatalf("%s: got %d suggestions, want %d", name, len(out.People), want)
						}
						completed++
					}
					data, err := json.Marshal(out)
					if err != nil {
						t.Fatal(err)
					}
					size += uint64(len(data))
					frames++
					total := time.Since(start)
					if firstAt == 0 {
						firstAt = total
					}
					runtime.ReadMemStats(&after)
					alloc += after.TotalAlloc - before.TotalAlloc
					elapsed = append(elapsed, float64(total)/float64(time.Millisecond))
					first = append(first, float64(firstAt)/float64(time.Millisecond))
					if completed == 0 {
						break
					}
				}
				slices.Sort(elapsed)
				slices.Sort(first)
				n := len(elapsed)
				t.Logf("%s n=%d completed=%d total_p50=%.2fms total_max=%.2fms first_p50=%.2fms frames=%.1f bytes=%.0f alloc=%.2fMiB",
					name, n, completed, elapsed[n/2], elapsed[n-1], first[n/2], float64(frames)/float64(n), float64(size)/float64(n), float64(alloc)/float64(n)/(1<<20))
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			measure("cold_stream", true, 1)
			runtime.GC()
			runtime.ReadMemStats(&after)
			t.Logf("cache_references=%d retained_heap=%.2fMiB", faceSuggestionCachedReferences(l), float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/(1<<20))
			measure("warm_stream_early_hits", true, 3)
			measure("warm_json_early_hits", false, 3)
			setFaceSuggestionPerfSource(t, l, 2)
			measure("warm_stream_no_hits", true, 10)
			setFaceSuggestionPerfSource(t, l, 0)
			// Separately measure the existing code after optimizer statistics.
			// This changes only the synthetic database, never a user database.
			if _, err := l.index.db.Exec(`ANALYZE`); err != nil {
				t.Fatal(err)
			}
			measure("analyzed_warm_stream_early_hits", true, 10)
			measure("analyzed_warm_json_early_hits", false, 10)

			// Later IDs steadily improve the top 20: bound each event's size while
			// exposing repeated validation and rendering work across many events.
			tx, err := l.index.db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			for i := 1; i <= groups; i++ {
				if _, err = tx.Exec(`UPDATE perf_vectors SET vector=? WHERE id=?`, encodeVector(matchingVector(.46+.44*float32(i)/float32(groups))), i); err != nil {
					tx.Rollback()
					t.Fatal(err)
				}
			}
			if _, err = tx.Exec(`UPDATE photo_faces SET embedding=(SELECT vector FROM perf_vectors WHERE id=photo_faces.person_id-1) WHERE id<>1; UPDATE photo_face_state SET revision=revision+1`); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if _, err = l.SuggestPeopleForFace(ctx, 1); err != nil {
				t.Fatal(err)
			}
			var profile *os.File
			if profilePath := os.Getenv("BEARSTACK_FACE_SUGGESTION_CPU"); profilePath != "" && groups == 10000 {
				profile, err = os.Create(profilePath)
				if err != nil {
					t.Fatal(err)
				}
				if err = pprof.StartCPUProfile(profile); err != nil {
					profile.Close()
					t.Fatal(err)
				}
			}
			measure("warm_stream_improving_hits", true, 5)
			measure("warm_json_improving_hits", false, 5)
			if profile != nil {
				pprof.StopCPUProfile()
				profile.Close()
			}
			thresholds := DefaultFaceThresholds()
			thresholds.SuggestionMargin = .05
			if err := l.SetFaceThresholds(ctx, thresholds); err != nil {
				t.Fatal(err)
			}
			measure("warm_stream_positive_margin", true, 5)
			if err := l.SetFaceThresholds(ctx, DefaultFaceThresholds()); err != nil {
				t.Fatal(err)
			}

			if _, err := l.index.db.Exec(`UPDATE photo_people SET name='',name_fold='' WHERE id>1 AND id%10<>0`); err != nil {
				t.Fatal(err)
			}
			measure("warm_stream_90pct_unnamed", true, 5)
			l.faceSuggestions.mu.Lock()
			l.faceSuggestions.snapshot = nil
			l.faceSuggestions.mu.Unlock()
			measure("cold_stream_90pct_unnamed", true, 1)

			if _, err := l.index.db.Exec(`UPDATE photo_face_reference_settings SET pending=1,cursor=0`); err != nil {
				t.Fatal(err)
			}
			measure("pending_named_stream_90pct_unnamed", true, 1)
			start := time.Now()
			if err := l.rebuildFaceReferences(ctx); err != nil {
				t.Fatal(err)
			}
			t.Logf("pending_reference_rebuild=%s", time.Since(start))
			faceSuggestionPerfContention(t, l)
		})
	}
}

func setFaceSuggestionPerfSource(t *testing.T, l *Library, axis int) {
	t.Helper()
	// The source group is excluded from matching, so its cached vector need not
	// be rebuilt to measure a different query against the same warm references.
	if _, err := l.index.db.Exec(`UPDATE photo_faces SET embedding=? WHERE id=1`, encodeVector(faceDetection(axis).Embedding)); err != nil {
		t.Fatal(err)
	}
}

func faceSuggestionPerfFixture(t *testing.T, groups int) *Library {
	t.Helper()
	root, data := t.TempDir(), t.TempDir()
	f, err := os.Create(filepath.Join(root, "source.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	err = jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 64, 64)), nil)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("source: %v %v", err, closeErr)
	}
	l, err := New(root, filepath.Join(data, "cache"), filepath.Join(data, "photos.db"), 60)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	if _, err = l.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := range min(1000, groups+1) {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("album%04d", i)), 0750); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := l.index.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, q := range []string{
		`CREATE TABLE perf_vectors(id INTEGER PRIMARY KEY,vector BLOB)`,
		`INSERT INTO photo_people(id) VALUES(1)`,
	} {
		if _, err = tx.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(`INSERT INTO photo_faces(id,path,directory,person_id,x,y,width,height,confidence,embedding,model) VALUES(1,'source.jpg','',1,.1,.1,.2,.2,.99,?,?)`, encodeVector(faceDetection(0).Embedding), facerec.Model); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= groups; i++ {
		score := float32(.1)
		if i <= 32 {
			score = .9 - float32(i)*.005
		}
		if _, err := tx.Exec(`INSERT INTO perf_vectors VALUES(?,?)`, i, encodeVector(matchingVector(score))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(`INSERT INTO photo_people(id,name,name_fold,manual_name) SELECT id+1,printf('Person %05d',id),printf('person %05d',id),1 FROM perf_vectors`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10)
 INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at)
 SELECT printf('album%04d/%05d-%02d.jpg',v.id%1000,v.id,x),printf('%05d-%02d.jpg',v.id,x),printf('album%04d',v.id%1000),'image','image/jpeg',1,1,'' FROM perf_vectors v CROSS JOIN n`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`WITH n(x) AS (VALUES(1),(2),(3))
 INSERT INTO photo_faces(path,directory,person_id,x,y,width,height,confidence,embedding,model)
 SELECT m.path,m.directory,v.id+1,.1,.1,.2,.2,.99,v.vector,? FROM media_index m
 JOIN perf_vectors v ON v.id=CAST(substr(m.name,1,5) AS INTEGER) CROSS JOIN n WHERE m.path<>'source.jpg'`, facerec.Model); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO photo_face_references SELECT id,person_id FROM photo_faces;
 UPDATE photo_face_reference_settings SET pending=0,cursor=0;
 UPDATE photo_face_state SET model=?,revision=revision+1`, facerec.Model); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var refs int
	if err = l.index.db.QueryRow(`SELECT count(*) FROM photo_face_references`).Scan(&refs); err != nil || refs != groups*30+1 {
		t.Fatalf("refs=%d %v", refs, err)
	}
	return l
}

func faceSuggestionPerfContention(t *testing.T, l *Library) {
	t.Helper()
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		once := false
		_, err := l.SuggestPeopleForFaceStream(context.Background(), 1, func(PeopleSuggestions) error {
			if !once {
				once = true
				close(entered)
				<-release
			}
			return nil
		})
		finished <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("no stream event")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	waiting := make(chan error, 1)
	start := time.Now()
	go func() { _, err := l.SuggestPeopleForFace(ctx, 1); waiting <- err }()
	<-ctx.Done()
	var returned bool
	var secondErr error
	select {
	case secondErr = <-waiting:
		returned = true
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if !returned {
		secondErr = <-waiting
	}
	if secondErr != nil && !errors.Is(secondErr, context.DeadlineExceeded) {
		t.Fatalf("cancel: %v", secondErr)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	t.Logf("second_request_returned_while_other_stream_blocked=%t wait=%s", returned, time.Since(start))
}

func faceSuggestionCachedReferences(l *Library) int {
	l.faceSuggestions.mu.Lock()
	defer l.faceSuggestions.mu.Unlock()
	count := 0
	if s := l.faceSuggestions.snapshot; s != nil {
		for _, refs := range s.faces {
			count += len(refs)
		}
	}
	return count
}
