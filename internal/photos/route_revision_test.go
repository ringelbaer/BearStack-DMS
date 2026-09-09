package photos

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScopedRouteRevisionsPreserveOtherFoldersTypesAndVisibility(t *testing.T) {
	l := routeTestLibrary(t)
	ctx := context.Background()
	seedCachedPhotoRoute(t, l, 4)
	for _, dir := range []string{"trip/child", "trip-other"} {
		if err := os.MkdirAll(filepath.Join(l.root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(statement string) {
		t.Helper()
		if _, err := l.index.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE media_index SET directory='trip/child' WHERE name='0.jpg'; UPDATE media_index SET directory='trip-other' WHERE name='1.jpg'; UPDATE media_index SET type='video' WHERE name='2.jpg'; UPDATE media_index SET admin_only=1 WHERE name='3.jpg'`)
	queries := []indexMediaOptions{
		{}, {Directory: "trip"}, {Directory: "trip/child"}, {Directory: "trip-other"},
		{Directory: "trip", MediaType: "image"}, {Directory: "trip", MediaType: "video"}, {Directory: "trip", IncludeAdminOnly: true},
	}
	revisions := func() []photoRouteRevision {
		t.Helper()
		out := make([]photoRouteRevision, len(queries))
		for i, q := range queries {
			var err error
			out[i], err = l.scopedPhotoRouteRevision(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	check := func(statement string, changed ...int) {
		t.Helper()
		before := revisions()
		exec(statement)
		after := revisions()
		for i := range queries {
			want := false
			for _, j := range changed {
				want = want || i == j
			}
			if (before[i] != after[i]) != want {
				t.Fatalf("scope %+v invalidation=%v want=%v after %s", queries[i], before[i] != after[i], want, statement)
			}
		}
	}
	check(`UPDATE media_index SET latitude=5 WHERE name='0.jpg'`, 0, 1, 2, 4, 6)
	check(`UPDATE media_index SET captured_at='2026-09-09T10:00:00Z' WHERE name='2.jpg'`, 0, 1, 5, 6)
	check(`UPDATE media_index SET longitude=15 WHERE name='3.jpg'`, 6)
	check(`UPDATE media_index SET admin_only=0 WHERE name='3.jpg'`, 0, 1, 4, 6)
	check(`UPDATE media_index SET directory='trip-other' WHERE name='0.jpg'`, 0, 1, 2, 3, 4, 6)
	check(`DELETE FROM media_index WHERE name='0.jpg'`, 0, 3)
	check(`UPDATE media_index SET tags='["holiday"]'`)
	check(`BEGIN; UPDATE media_index SET latitude=18; ROLLBACK`)
	// A real warm route remains usable even if regrouping would now fail.
	world := MapBounds{-90, -180, 90, 180}
	if _, err := l.MapPhotoRoute(ctx, ListOptions{Path: "trip"}, world, 32); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE media_index SET latitude=6 WHERE name='1.jpg'; DROP INDEX idx_media_index_route_time`)
	if _, err := l.MapPhotoRoute(ctx, ListOptions{Path: "trip"}, world, 32); err != nil {
		t.Fatalf("unrelated folder invalidated warm cache: %v", err)
	}
}
