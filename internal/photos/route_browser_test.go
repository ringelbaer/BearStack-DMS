package photos

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBrowserPhotoRouteSharesNativeCacheBeyondMediaPage(t *testing.T) {
	l := routeTestLibrary(t)
	seedCachedPhotoRoute(t, l, 200)
	ctx := context.Background()
	opts := ListOptions{Path: "trip", Recursive: true, GPSOnly: true, IncludeMapData: true, PageSize: 7}
	first, err := l.List(ctx, opts)
	if err != nil || len(first.Media) != 7 || len(first.RoutePoints) != 200 || first.RouteTotalPoints != 200 {
		t.Fatalf("browser route/media: %d/%d total=%d: %v", len(first.RoutePoints), len(first.Media), first.RouteTotalPoints, err)
	}
	files := cachedRouteFiles(t, l)
	if len(files) != 1 {
		t.Fatalf("browser did not populate the cache: %v", files)
	}
	before, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	// Neither the next browser page nor the native API may regroup source GPS
	// rows. The explicit chronological query would fail without this index.
	if _, err = l.index.db.Exec(`DROP INDEX idx_media_index_route_time`); err != nil {
		t.Fatal(err)
	}
	opts.Page, opts.Sort = 2, "descending_name"
	second, err := l.List(ctx, opts)
	if err != nil || !reflect.DeepEqual(first.RoutePoints, second.RoutePoints) {
		t.Fatalf("browser cache reuse: %v", err)
	}
	native, err := l.MapPhotoRoute(ctx, ListOptions{Path: "trip"}, MapBounds{-90, -180, 90, 180}, 32)
	if err != nil || native.TotalPoints != 200 || native.TotalMedia != 200 {
		t.Fatalf("shared native cache: %+v %v", native, err)
	}
	after, err := os.Stat(files[0])
	if err != nil || !os.SameFile(before, after) || len(cachedRouteFiles(t, l)) != 1 {
		t.Fatalf("cache replaced: %v", err)
	}
}

func TestBrowserPhotoRouteInvalidatesAndSeparatesVisibilityAndSearch(t *testing.T) {
	l := routeTestLibrary(t)
	seedCachedPhotoRoute(t, l, 100)
	ctx := context.Background()
	opts := ListOptions{Path: "trip", Recursive: true, GPSOnly: true, IncludeMapData: true, PageSize: 7}
	if _, err := l.List(ctx, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := l.index.db.Exec(`UPDATE media_index SET latitude=5`); err != nil {
		t.Fatal(err)
	}
	changed, err := l.List(ctx, opts)
	if err != nil || len(changed.RoutePoints) != 1 || changed.RoutePoints[0].Lat != 5 || changed.RoutePoints[0].Count != 100 {
		t.Fatalf("stale browser route: %+v %v", changed.RoutePoints, err)
	}
	// A search gets the full selection, but does not create another cache file.
	search := opts
	search.Query = "gps:true"
	queried, err := l.List(ctx, search)
	if err != nil || !reflect.DeepEqual(changed.RoutePoints, queried.RoutePoints) || len(cachedRouteFiles(t, l)) != 1 {
		t.Fatalf("search cache policy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(l.root, "trip", ".adminonly"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	opts.Path = ""
	public, err := l.List(ctx, opts)
	if err != nil || len(public.RoutePoints) != 0 || len(public.Media) != 0 || public.Total != 0 {
		t.Fatalf("private route/markers exposed: %+v %v", public, err)
	}
	if _, err := l.index.db.Exec(`UPDATE media_index SET admin_only=0`); err != nil {
		t.Fatal(err)
	}
	opts.Query = "gps:true"
	public, err = l.List(ctx, opts)
	if err != nil || len(public.Media) != 0 || len(public.RoutePoints) != 0 {
		t.Fatalf("private search map exposed: %+v %v", public, err)
	}
	opts.Query = ""
	opts.IncludeAdminOnly = true
	admin, err := l.List(ctx, opts)
	if err != nil || len(admin.RoutePoints) != 1 || admin.RoutePoints[0].Count != 100 {
		t.Fatalf("admin route: %+v %v", admin.RoutePoints, err)
	}
}

func TestBrowserPhotoRouteSamplesWholeCachedRouteWithBoundedStorage(t *testing.T) {
	l := routeTestLibrary(t)
	const count = 20000
	seedCachedPhotoRoute(t, l, count)
	listing, err := l.List(context.Background(), ListOptions{Path: "trip", Recursive: true, GPSOnly: true, IncludeMapData: true, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	points := listing.RoutePoints
	if listing.RouteTotalPoints != count || len(points) != browserRoutePointLimit || points[0].Order != 1 || points[len(points)-1].Order != count {
		t.Fatalf("incomplete/unbounded route: %d/%d", len(points), listing.RouteTotalPoints)
	}
	for i, p := range points {
		if p.Count != 1 || p.StartedAt != p.EndedAt || (i > 0 && p.Order <= points[i-1].Order) {
			t.Fatalf("lost source metadata: %+v", p)
		}
	}
	query, err := l.mapQuery(context.Background(), ListOptions{Path: "trip"}, MapBounds{-90, -180, 90, 180})
	if err != nil {
		t.Fatal(err)
	}
	stored := 0
	if err = l.consumePhotoRoute(context.Background(), query, 1000, func(routeCluster) error { stored++; return nil }); err != nil || stored != count {
		t.Fatalf("cache was sampled: %d %v", stored, err)
	}
	var window browserRoutePoints
	for i := 0; i < 100000; i++ {
		at := time.Unix(int64(i), 0)
		_ = window.add(routeCluster{lat: float64(i % 2), lon: 13, started: at, ended: at, count: 1})
		if len(window.points) > 2*browserRoutePointLimit {
			t.Fatal("unbounded working window")
		}
	}
}
