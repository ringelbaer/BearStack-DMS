package photos

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestMediaBatchMatchesSinglesAndRefreshesFiles(t *testing.T) {
	ctx := context.Background()
	l := photoTagFixture(t)
	finishFace(t, l, 0)
	paths := []string{"album/second.jpg", "album/photo.jpg", "album/second.jpg"}
	batch, err := l.MediaBatchContext(ctx, paths)
	if err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		single, err := l.MediaContext(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		// The single reader historically returns [] for no faces, batch decoration nil.
		if len(single.AutomaticFaces) == 0 {
			single.AutomaticFaces = nil
		}
		if !reflect.DeepEqual(single, batch[i]) {
			t.Fatalf("batch differs for %s:\nsingle %+v\nbatch %+v", path, single, batch[i])
		}
	}
	path := filepath.Join(l.Root(), "album/photo.jpg")
	writeJPEG(t, path, color.RGBA{G: 255, A: 255})
	changedAt := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, changedAt, changedAt); err != nil {
		t.Fatal(err)
	}
	batch, err = l.MediaBatchContext(ctx, []string{"album/photo.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if !batch[0].ModTime.Equal(changedAt) || len(batch[0].AutomaticFaces) != 0 || !reflect.DeepEqual(batch[0].Tags, []string{"original"}) {
		t.Fatalf("stale metadata/faces or lost manual tags: %+v", batch[0])
	}
	writeJPEG(t, filepath.Join(l.Root(), "album/new.jpg"), color.White)
	batch, err = l.MediaBatchContext(ctx, []string{"album/new.jpg"})
	if err != nil || len(batch) != 1 || batch[0].Width == 0 {
		t.Fatalf("new media = %+v, %v", batch, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := l.MediaBatchContext(ctx, []string{"album/photo.jpg"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted file = %v", err)
	}
}

func TestMediaBatchHonorsCurrentPrivacyAndPaths(t *testing.T) {
	ctx := context.Background()
	l := faceLibrary(t, "public/photo.jpg", "secret/photo.jpg")
	finishFace(t, l, 0)
	finishFace(t, l, 0)
	// Name provenance must be rechecked even if it is outside the requested batch.
	if _, err := l.index.db.Exec(`UPDATE photo_people SET name='Private Name', name_source='secret/photo.jpg'`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "secret/.adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := l.MediaBatchContext(ctx, []string{"public/photo.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch[0].AutomaticFaces) != 1 || batch[0].AutomaticFaces[0].Name != "" {
		t.Fatalf("private name leaked: %+v", batch)
	}
	batch, err = l.MediaBatchContext(ctx, []string{"secret/photo.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if !batch[0].AdminOnly || len(batch[0].AutomaticFaces) != 0 {
		t.Fatalf("private faces leaked: %+v", batch)
	}
	if err := os.Symlink(filepath.Join(l.Root(), "public/photo.jpg"), filepath.Join(l.Root(), "link.jpg")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../outside.jpg", "link.jpg", "", "unknown.txt"} {
		if _, err := l.MediaBatchContext(ctx, []string{path}); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.MediaBatchContext(cancelled, []string{"public/photo.jpg"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
}

// Wrap the existing driver, retaining its registered SQLite functions. Counting
// actual queries catches accidental per-photo fallbacks, even for cache hits.
type mediaQueryConnector struct {
	driver  driver.Driver
	dsn     string
	queries *atomic.Int64
}

func (c mediaQueryConnector) Driver() driver.Driver { return c.driver }
func (c mediaQueryConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.driver.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return mediaQueryConn{Conn: conn, queries: c.queries}, nil
}

type mediaQueryConn struct {
	driver.Conn
	queries *atomic.Int64
}

func (c mediaQueryConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.queries.Add(1)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}

func (c mediaQueryConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func TestMediaBatchBoundsDatabaseQueries(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 201)
	for i := range paths {
		paths[i] = fmt.Sprintf("photo-%03d.jpg", i)
		writeJPEG(t, filepath.Join(root, paths[i]), color.White)
	}
	l := newTestLibrary(t, root)
	t.Cleanup(func() { _ = l.Close() })
	ctx := context.Background()
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	dsn, err := indexSQLiteDSN(l.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	var queries atomic.Int64
	db := sql.OpenDB(mediaQueryConnector{l.index.db.Driver(), dsn, &queries})
	if err := l.index.db.Close(); err != nil {
		t.Fatal(err)
	}
	l.index.db = db
	for _, count := range []int{0, 1, 100, 201} {
		queries.Store(0)
		items, err := l.MediaBatchContext(ctx, paths[:count])
		if err != nil || len(items) != count {
			t.Fatalf("batch %d = %d items, %v", count, len(items), err)
		}
		want := int64(2 * ((count + 199) / 200))
		if got := queries.Load(); got != want {
			t.Fatalf("batch %d: %d queries; want %d", count, got, want)
		}
	}
}

func BenchmarkMediaInfoBatch(b *testing.B) {
	root := b.TempDir()
	paths := make([]string, 100)
	for i := range paths {
		paths[i] = fmt.Sprintf("photo-%03d.mp4", i)
		if err := os.WriteFile(filepath.Join(root, paths[i]), []byte("media"), 0600); err != nil {
			b.Fatal(err)
		}
	}
	l, err := New(root, filepath.Join(b.TempDir(), "cache"), filepath.Join(b.TempDir(), "photos.db"), 100)
	if err != nil {
		b.Fatal(err)
	}
	defer l.Close()
	ctx := context.Background()
	if _, err := l.RebuildIndex(ctx); err != nil {
		b.Fatal(err)
	}
	b.Run("single-loop", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			for _, path := range paths {
				if _, err := l.MediaContext(ctx, path); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("batch", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := l.MediaBatchContext(ctx, paths); err != nil {
				b.Fatal(err)
			}
		}
	})
}
