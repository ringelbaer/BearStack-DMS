package photos

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func routeTestLibrary(t testing.TB) *Library {
	t.Helper()
	l, err := New(t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	if err := os.Mkdir(filepath.Join(l.Root(), "trip"), 0750); err != nil {
		t.Fatal(err)
	}
	if _, err = l.RebuildIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
	return l
}

func BenchmarkMapPhotoRoute(b *testing.B) {
	l := routeTestLibrary(b)
	_, err := l.index.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<99999)
	INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at,latitude,longitude)
	SELECT printf('trip/%d.jpg',n),printf('%d.jpg',n),'trip','image','image/jpeg',1,1700000000000000000+n*1000000000,'2026-09-09',n%2,13 FROM seq`)
	if err != nil {
		b.Fatal(err)
	}
	if _, err = l.index.db.Exec(`INSERT INTO media_search(rowid,path,search_text) SELECT rowid,path,'trip' FROM media_index`); err != nil {
		b.Fatal(err)
	}
	for _, search := range []string{"", "trip"} {
		b.Run("query="+search, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				route, e := l.MapPhotoRoute(context.Background(), ListOptions{Query: search}, MapBounds{-90, -180, 90, 180}, 4096)
				if e != nil || route.TotalMedia != 100000 {
					b.Fatalf("route: %d %v", route.TotalMedia, e)
				}
			}
		})
	}
}

func TestMapPhotoRouteOrdersOffsetsFractionsAndFallbackExactly(t *testing.T) {
	l := routeTestLibrary(t)
	times := []string{"2024-01-01T10:00:00+02:00", "2024-01-01T07:59:00Z", "2024-01-01T08:00:00.1234Z", "2024-01-01T08:00:00.123Z", ""}
	for i, captured := range times {
		_, err := l.index.db.Exec(`INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,captured_at,indexed_at,latitude,longitude)
		VALUES(?,?,'trip','image','image/jpeg',1,?,?,'2026-09-09',1,?)`, string(rune('a'+i))+".jpg", "trip.jpg",
			time.Date(2024, 1, 1, 8, 1, 0, 0, time.UTC).UnixNano(), captured, float64(i+1)*10)
		if err != nil {
			t.Fatal(err)
		}
	}
	world := MapBounds{-90, -180, 90, 180}
	if _, err := l.index.db.Exec(`INSERT INTO media_search(rowid,path,search_text) SELECT rowid,path,'trip' FROM media_index`); err != nil {
		t.Fatal(err)
	}
	result, err := l.MapPhotoRoute(context.Background(), ListOptions{}, world, 32)
	want := [][]MapCoordinate{{{1, 20}, {1, 10}, {1, 40}, {1, 30}, {1, 50}}}
	if err != nil || result.TotalMedia != 5 || result.TotalPoints != 5 || result.Simplified || !reflect.DeepEqual(result.Segments, want) {
		t.Fatalf("route: %+v %v", result, err)
	}
	for _, search := range []string{"", "trip", "gps:true OR type:audio"} {
		filtered, e := l.MapPhotoRoute(context.Background(), ListOptions{Query: search}, world, 32)
		if e != nil || !reflect.DeepEqual(filtered.Segments, want) {
			t.Fatalf("query %q: %+v %v", search, filtered, e)
		}
	}
	for _, search := range []string{"", "trip", "type:image"} {
		query, e := l.mapQuery(context.Background(), ListOptions{Query: search}, world)
		if e != nil {
			t.Fatal(e)
		}
		sql, args := routeIndexQuery(query)
		rows, e := l.index.db.Query("EXPLAIN QUERY PLAN "+sql, args...)
		if e != nil {
			t.Fatal(e)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if e = rows.Scan(&id, &parent, &unused, &detail); e != nil {
				t.Fatal(e)
			}
			plan.WriteString(detail)
		}
		rows.Close()
		if search != "trip" && (!strings.Contains(plan.String(), "idx_media_index_route_time") || strings.Contains(plan.String(), "TEMP B-TREE")) {
			t.Fatalf("unbounded route sort for %q: %s", search, plan.String())
		}
		if search == "trip" && !strings.HasPrefix(plan.String(), "SCAN media_search") {
			t.Fatalf("FTS must be scanned once: %s", plan.String())
		}
	}
}

func TestMapPhotoRouteIncludesWholeLargeIndexAndSharesFilters(t *testing.T) {
	l := routeTestLibrary(t)
	_, err := l.index.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<19999)
	INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at,latitude,longitude)
	SELECT printf('trip/%d.jpg',n),printf('%d.jpg',n),'trip','image','image/jpeg',1,1700000000000000000+n*1000000000,'2026-09-09',n%2,13 FROM seq`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	world := MapBounds{-90, -180, 90, 180}
	result, err := l.MapPhotoRoute(ctx, ListOptions{}, world, 64)
	if err != nil || result.TotalMedia != 20000 || result.TotalPoints != 20000 || !result.Simplified || result.Bounds == nil {
		t.Fatalf("large route %+v: %v", result, err)
	}
	points := 0
	for _, s := range result.Segments {
		points += len(s)
	}
	if points > 64 {
		t.Fatalf("geometry budget: %d", points)
	}
	_, err = l.MapPhotoRoute(ctx, ListOptions{Query: "gps:true OR type:audio"}, world, 64)
	if !errors.Is(err, errPhotoSearchTooBroad) {
		t.Fatalf("silent postfilter truncation: %v", err)
	}
	if _, err = l.index.db.Exec(`UPDATE media_index SET admin_only=1 WHERE latitude=1`); err != nil {
		t.Fatal(err)
	}
	public, err := l.MapPhotoRoute(ctx, ListOptions{}, world, 64)
	if err != nil || public.TotalMedia != 10000 || public.TotalPoints != 1 || public.Bounds.North != 0 {
		t.Fatalf("private route %+v: %v", public, err)
	}
	for _, opts := range []ListOptions{{MediaType: MediaTypeVideo}, {Query: "gps:false"}} {
		empty, e := l.MapPhotoRoute(ctx, opts, world, 64)
		if e != nil || empty.TotalMedia != 0 || len(empty.Segments) != 0 {
			t.Fatalf("filters: %+v %v", empty, e)
		}
	}
	_, err = l.MapPhotoRoute(ctx, ListOptions{}, world, 31)
	if !errors.Is(err, ErrMapPointLimit) {
		t.Fatalf("point budget: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = l.MapPhotoRoute(cancelled, ListOptions{}, world, 64)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestRouteGeometryCompactionPreservesGapsAndBoundedMemory(t *testing.T) {
	g := routeGeometry{viewport: MapBounds{0, 0, 1, 1}, budget: 32}
	for i := 0; i < 10000; i++ {
		for _, p := range []GPXPoint{{.25, -1}, {.25, 2}, {2, 2}, {2, -1}} {
			g.add(p)
			if g.points > 128 {
				t.Fatalf("working geometry grew: %d", g.points)
			}
		}
	}
	lines, simplified, omitted := g.finish()
	if !simplified || len(lines) != 16 || omitted != 10000-16 {
		t.Fatalf("segments=%d omitted=%d simplified=%v", len(lines), omitted, simplified)
	}
	for _, line := range lines {
		if !reflect.DeepEqual(line, []MapCoordinate{{.25, 0}, {.25, 1}}) {
			t.Fatalf("invented crossing: %v", line)
		}
	}
	dateLine := routeGeometry{viewport: MapBounds{0, 170, 3, -170}, budget: 32}
	dateLine.add(GPXPoint{1, 179})
	dateLine.add(GPXPoint{2, -179})
	segments, _, _ := dateLine.finish()
	if len(segments) != 1 || len(segments[0]) != 2 || math.Abs(segments[0][0][1]) != 179 || math.Abs(segments[0][1][1]) != 179 {
		t.Fatalf("date line: %v", segments)
	}
}

func TestRouteSearchRestoresConnectionAfterCancellationAndQueryError(t *testing.T) {
	l := routeTestLibrary(t)
	l.index.db.SetMaxOpenConns(1)
	var before, after, during int
	if err := l.index.db.QueryRow("PRAGMA temp_store").Scan(&before); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rows, closeRows, err := l.routeRows(ctx, "PRAGMA temp_store", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("missing temp mode")
	}
	if err = rows.Scan(&during); err != nil {
		t.Fatal(err)
	}
	cancel()
	closeRows()
	if err = l.index.db.QueryRow("PRAGMA temp_store").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if during != 1 || after != before {
		t.Fatalf("temp modes before=%d during=%d after=%d", before, during, after)
	}
	if _, _, err = l.routeRows(context.Background(), "SELECT * FROM nonexistent_route_test_table", nil, true); err == nil {
		t.Fatal("query error missing")
	}
	if err = l.index.db.QueryRow("PRAGMA temp_store").Scan(&after); err != nil || after != before {
		t.Fatalf("failed request changed pool: %d %v", after, err)
	}
}
