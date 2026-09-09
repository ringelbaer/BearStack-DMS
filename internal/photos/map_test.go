package photos

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPhotoMapAggregatesWholeLargeIndex(t *testing.T) {
	l, err := New(t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := os.Mkdir(filepath.Join(l.Root(), "trip"), 0750); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	// Far beyond both a catalog page and the browser's 10,000-image map limit.
	_, err = l.index.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<19999)
	INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at,latitude,longitude)
	SELECT printf('trip/%d.jpg',n), printf('%d.jpg',n),'trip','image','image/jpeg',1,1,'2026-09-09',
	-80.0+(n%160), -179.0+(n%358) FROM seq`)
	if err != nil {
		t.Fatal(err)
	}
	world := MapBounds{-90, -180, 90, 180}
	result, err := l.Map(ctx, ListOptions{}, world)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 20000 || len(result.Markers) > 289 || result.Bounds == nil {
		t.Fatalf("map totals: %d markers %d bounds %v", result.Total, len(result.Markers), result.Bounds)
	}
	total := 0
	for _, marker := range result.Markers {
		total += marker.Count
		if marker.Count > 1 && marker.Path != "" {
			t.Fatal("cluster exposes arbitrary single path")
		}
	}
	if total != 20000 {
		t.Fatalf("silently truncated: %d", total)
	}
	first, mediaTotal, err := l.MapMedia(ctx, ListOptions{}, world)
	if err != nil || len(first) != 96 || mediaTotal != 20000 {
		t.Fatalf("map page: %d/%d %v", len(first), mediaTotal, err)
	}
	last, mediaTotal, err := l.MapMedia(ctx, ListOptions{Page: 209}, world)
	if err != nil || len(last) != 32 || mediaTotal != 20000 {
		t.Fatalf("last map page: %d/%d %v", len(last), mediaTotal, err)
	}
	selection := MapBounds{South: 0, West: 0, North: 2, East: 4}
	cluster, err := l.Map(ctx, ListOptions{}, selection)
	if err != nil {
		t.Fatal(err)
	}
	items, itemTotal, err := l.MapMedia(ctx, ListOptions{}, selection)
	if err != nil || itemTotal != cluster.Total || len(items) > 96 {
		t.Fatalf("selection count: %d/%d %v", itemTotal, cluster.Total, err)
	}
	for _, item := range items {
		if *item.Latitude < 0 || *item.Latitude > 2 || *item.Longitude < 0 || *item.Longitude > 4 {
			t.Fatalf("outside selection: %+v", item)
		}
	}
	if _, err := l.Map(ctx, ListOptions{Query: "2-of:(gps:true,type:image)"}, world); !errors.Is(err, ErrSearchTooBroad()) {
		t.Fatalf("complex search must report its candidate limit: %v", err)
	}
	if _, err := l.index.db.Exec(`UPDATE media_index SET admin_only=1 WHERE latitude<0`); err != nil {
		t.Fatal(err)
	}
	public, err := l.Map(ctx, ListOptions{}, world)
	if err != nil {
		t.Fatal(err)
	}
	if public.Total != 10000 || public.Bounds.South < 0 {
		t.Fatalf("private positions leaked: %+v", public.Bounds)
	}
	filtered, err := l.Map(ctx, ListOptions{MediaType: MediaTypeVideo}, world)
	if err != nil || filtered.Total != 0 || filtered.Bounds != nil {
		t.Fatalf("type filter: %+v %v", filtered, err)
	}
	filtered, err = l.Map(ctx, ListOptions{Query: "gps:false"}, world)
	if err != nil || filtered.Total != 0 {
		t.Fatalf("shared search filter: %+v %v", filtered, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.Map(cancelled, ListOptions{}, world); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestPhotoMapViewportAntimeridianAndPostFilterAgree(t *testing.T) {
	l, err := New(t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "photos.db"), 50)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ctx := context.Background()
	if _, err := l.RebuildIndex(ctx); err != nil {
		t.Fatal(err)
	}
	items := []Media{}
	for i, lon := range []float64{-179, 179, -170, 170, -90, 0, 90} {
		item := Media{Path: string(rune('a'+i)) + ".jpg", Name: "photo.jpg", Type: MediaTypeImage, MIMEType: "image/jpeg", Latitude: float64Ptr(2), Longitude: float64Ptr(lon)}
		if err := l.saveMediaContext(ctx, item); err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	for _, viewport := range []MapBounds{{0, 160, 10, -160}, {0, -100, 10, -150}, {0, -95, 10, 95}, {1.99, 178.9, 2.01, 179.1}} {
		actual, err := l.Map(ctx, ListOptions{}, viewport)
		if err != nil {
			t.Fatal(err)
		}
		expected := aggregateMapMedia(items, viewport)
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("viewport %+v\nSQL: %+v %v\nGo: %+v %v", viewport, actual, actual.Bounds, expected, expected.Bounds)
		}
	}
	// The initial world overview must fit a date-line trip locally too.
	if _, err := l.index.db.Exec(`DELETE FROM media_index WHERE longitude BETWEEN -100 AND 100`); err != nil {
		t.Fatal(err)
	}
	overview, err := l.Map(ctx, ListOptions{}, MapBounds{-90, -180, 90, 180})
	if err != nil || overview.Bounds == nil || overview.Bounds.West != 170 || overview.Bounds.East != -170 {
		t.Fatalf("date-line overview: %+v %v", overview.Bounds, err)
	}
	if err := os.Mkdir(filepath.Join(l.Root(), "private"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(l.Root(), "private", AdminOnlyMarkerName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Map(ctx, ListOptions{Path: "private"}, MapBounds{-90, -180, 90, 180}); !errors.Is(err, errAdminOnly) {
		t.Fatalf("private folder: %v", err)
	}
	if _, err := l.Map(ctx, ListOptions{Path: "../outside"}, MapBounds{-90, -180, 90, 180}); err == nil {
		t.Fatal("traversal accepted")
	}
	for _, b := range []MapBounds{{0, 0, 0, 1}, {-91, 0, 1, 1}, {0, 0, 91, 1}, {0, 0, 1, 181}, {math.NaN(), 0, 1, 1}, {0, 0, math.Inf(1), 1}} {
		if _, err := l.Map(ctx, ListOptions{}, b); !errors.Is(err, ErrMapBounds) {
			t.Fatalf("invalid bounds %+v: %v", b, err)
		}
	}
}
