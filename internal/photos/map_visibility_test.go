package photos

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeMapsHideNewPrivateAncestorsBeforeRescanAcrossBatches(t *testing.T) {
	l := routeTestLibrary(t)
	for i := 0; i < 300; i++ {
		if err := os.Mkdir(filepath.Join(l.Root(), "trip", fmt.Sprint(i)), 0750); err != nil {
			t.Fatal(err)
		}
	}
	_, err := l.index.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n<299)
	INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,indexed_at,latitude,longitude)
	SELECT printf('trip/%d/a.jpg',n),'a.jpg',printf('trip/%d',n),'image','image/jpeg',1,1700000000000000000+n*1000000000,'2026-09-09',1,13 FROM seq`)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	world := MapBounds{-90, -180, 90, 180}
	before, err := l.Map(ctx, ListOptions{}, world)
	if err != nil || before.Total != 300 {
		t.Fatalf("initial map: %+v %v", before, err)
	}
	if err = os.WriteFile(filepath.Join(l.Root(), "trip", AdminOnlyMarkerName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"markers", "media", "route"} {
		// Give each endpoint the same stale public metadata, without a rescan.
		if _, err = l.index.db.Exec(`UPDATE media_index SET admin_only=0`); err != nil {
			t.Fatal(err)
		}
		total := 0
		switch kind {
		case "markers":
			result, e := l.Map(ctx, ListOptions{}, world)
			err = e
			total = result.Total
		case "media":
			_, total, err = l.MapMedia(ctx, ListOptions{}, world)
		case "route":
			result, e := l.MapPhotoRoute(ctx, ListOptions{}, world, 32)
			err = e
			total = result.TotalMedia
		}
		if err != nil || total != 0 {
			t.Fatalf("%s leaked positions: %d %v", kind, total, err)
		}
	}
}
