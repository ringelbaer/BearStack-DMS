package photos

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPopulatedMapIndexUpgradeIsDeferredCancellableAndResumable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "photos.db")
	db, _, err := openIndexDB(path)
	if err != nil {
		t.Fatal(err)
	}
	seedCachedPhotoRoute(t, &Library{index: &photoIndexStore{db: db}}, 10000)
	if _, err = db.Exec(`DROP INDEX idx_media_index_route_time; DROP INDEX idx_media_index_public_gps_directory`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, _, err = openIndexDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name IN ('idx_media_index_route_time','idx_media_index_public_gps_directory')`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("startup built optional populated indexes: %d %v", count, err)
	}
	// Hold the only connection to make a slow build deterministic. Opening the
	// store and serving an ordinary read must not wait for that optional work.
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	store := &photoIndexStore{db: db}
	store.startMapIndexes()
	if err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_index`).Scan(&count); err != nil || count != 10000 {
		t.Fatalf("gallery metadata unavailable: %d %v", count, err)
	}
	wait, cancel := context.WithCancel(ctx)
	cancel()
	if err = store.waitMapIndexes(wait); !errors.Is(err, context.Canceled) {
		t.Fatalf("map waiter: %v", err)
	}
	store.mapIndexesCancel()
	select {
	case <-store.mapIndexesDone:
	case <-time.After(5 * time.Second):
		t.Fatal("index build did not cancel")
	}
	conn.Close()
	if err = store.close(); err != nil {
		t.Fatal(err)
	}
	// A fresh process retries interrupted work and keeps all existing metadata.
	db, _, err = openIndexDB(path)
	if err != nil {
		t.Fatal(err)
	}
	store = &photoIndexStore{db: db}
	store.startMapIndexes()
	defer store.close()
	if err = store.waitMapIndexes(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM media_index`).Scan(&count); err != nil || count != 10000 {
		t.Fatalf("upgrade lost photos: %d %v", count, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name IN ('idx_media_index_route_time','idx_media_index_public_gps_directory')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("incomplete indexes: %d %v", count, err)
	}
}

func BenchmarkPopulatedPhotoMapIndexUpgrade(b *testing.B) {
	l := routeTestLibrary(b)
	seedCachedPhotoRoute(b, l, 100000)
	if err := l.index.waitMapIndexes(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if _, err := l.index.db.Exec(`DROP INDEX idx_media_index_route_time; DROP INDEX idx_media_index_public_gps_directory`); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err := ensurePhotoMapIndexes(context.Background(), l.index.db); err != nil {
			b.Fatal(err)
		}
	}
}

func TestMapIndexUpgradeRetriesConcurrentWriter(t *testing.T) {
	ctx := context.Background()
	db, _, err := openIndexDB(filepath.Join(t.TempDir(), "photos.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`DROP INDEX idx_media_index_route_time`); err != nil {
		t.Fatal(err)
	}
	writer, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	worker, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = worker.ExecContext(ctx, `PRAGMA busy_timeout=1`); err != nil {
		t.Fatal(err)
	}
	worker.Close()
	if _, err = writer.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	defer writer.ExecContext(ctx, `ROLLBACK`)
	if err = ensurePhotoMapIndexes(ctx, db); err == nil {
		t.Fatal("fixture did not block index creation")
	}
	store := &photoIndexStore{db: db}
	store.startMapIndexes()
	defer func() { store.mapIndexesCancel(); <-store.mapIndexesDone }()
	select {
	case <-store.mapIndexesDone:
		t.Fatalf("writer contention ended upgrade: %v", store.mapIndexesErr)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err = writer.ExecContext(ctx, `ROLLBACK`); err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = store.waitMapIndexes(wait); err != nil {
		t.Fatal(err)
	}
}
