package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"bearstack/internal/facerec"
)

// Opt-in integration benchmark: ~1 GB temporary disk, no model downloads.
func TestFaceScaleMillion(t *testing.T) {
	if os.Getenv("BEARSTACK_FACE_SCALE_TEST") != "1" {
		t.Skip("set BEARSTACK_FACE_SCALE_TEST=1 for 100k photos / 1m faces")
	}
	ctx := context.Background()
	l, err := New(t.TempDir(), filepath.Join(t.TempDir(), "cache"), filepath.Join(t.TempDir(), "photos.db"), 60)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	db := l.index.db
	start := time.Now()
	_, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<100000) INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at) SELECT printf('%06d.jpg',x),printf('%06d.jpg',x),'','image','image/jpeg',1,1,'' FROM n`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<10000) INSERT INTO photo_people(id,name,name_fold,manual_name) SELECT x,printf('Person %d',x),printf('person %d',x),1 FROM n`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE fixture_vectors(id INTEGER PRIMARY KEY,vector BLOB)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 10000; id++ {
		v := make([]float32, 128)
		seed := uint32(id)
		for i := range v {
			seed = 1664525*seed + 1013904223
			v[i] = float32(int32(seed)) / float32(1<<31)
		}
		d := facerec.Detection{Width: .2, Height: .2, Confidence: .99, Embedding: v}
		if err = facerec.Validate(&d); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(`INSERT INTO fixture_vectors VALUES(?,?)`, id, encodeVector(v)); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1000000) INSERT INTO photo_faces(path,person_id,x,y,width,height,confidence,embedding,model) SELECT printf('%06d.jpg',1+(x-1)/10),1+(x-1)%10000,.1,.1,.2,.2,.99,v.vector,? FROM n JOIN fixture_vectors v ON v.id=1+(x-1)%10000`, facerec.Model)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO photo_face_references SELECT id,person_id FROM photo_faces WHERE id<=50000`); err != nil {
		t.Fatal(err)
	}
	t.Logf("fixture: %s", time.Since(start))
	start = time.Now()
	if err = l.PrepareFaceQueue(ctx, facerec.Model); err != nil {
		t.Fatal(err)
	}
	t.Logf("queue backfill: %s", time.Since(start))
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start = time.Now()
	l.faceRuntime.mu.Lock()
	err = l.ensureFaceGraph(ctx, facerec.Model)
	l.faceRuntime.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("50k reference graph: %s, retained heap delta %.1f MiB", time.Since(start), float64(after.HeapAlloc-before.HeapAlloc)/(1<<20))
	if after.HeapAlloc > before.HeapAlloc+(512<<20) {
		t.Fatal("reference index exceeds 512 MiB budget")
	}
	// Background queue writes run concurrently with the same gallery SQL path.
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for workCtx.Err() == nil {
			j, e := l.NextFaceJob(workCtx)
			if e != nil {
				return
			}
			_, _ = db.ExecContext(workCtx, `UPDATE photo_face_jobs SET status='done' WHERE path=?`, j.Path)
		}
	}()
	defer func() { cancel(); <-done }()
	start = time.Now()
	for range 20 {
		_, total, e := l.indexMedia(ctx, indexMediaOptions{Query: "person:Person 1", Plan: indexQueryPlanFor(`person:"Person 1"`), Limit: 60})
		if e != nil || total == 0 {
			t.Fatalf("gallery query %d %v", total, e)
		}
	}
	elapsed := time.Since(start)
	t.Logf("20 person gallery requests under queue writes: %s (mean %s)", elapsed, elapsed/20)
	if elapsed > 20*time.Second {
		t.Fatal("gallery requests exceeded 1s mean budget")
	}
	start = time.Now()
	p, e := l.People(ctx, 0, 1, "")
	if e != nil || len(p.People) != 60 {
		t.Fatalf("people %d %v", len(p.People), e)
	}
	t.Logf("people page: %s", time.Since(start))
	rows, e := db.Query(`EXPLAIN QUERY PLAN SELECT path FROM photo_face_jobs WHERE status='queued' AND retry_at<=0 ORDER BY retry_at,path LIMIT 1`)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var a, b, c int
		var d string
		if e = rows.Scan(&a, &b, &c, &d); e != nil {
			t.Fatal(e)
		}
		fmt.Fprintln(&plan, d)
	}
	if !strings.Contains(plan.String(), "idx_face_jobs_ready") {
		t.Fatalf("queue lacks index: %s", plan.String())
	}
}
