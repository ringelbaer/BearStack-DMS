package photos

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func insertCatalogDate(t testing.TB, l *Library, path, captured, modified string, private bool) {
	t.Helper()
	stamp, err := time.Parse(time.RFC3339, modified)
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.index.db.Exec(`INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,captured_at,indexed_at,admin_only)
	VALUES(?,?, 'trip','image','image/jpeg',1,?,?, '2026-09-10',?)`, path, path, stamp.UnixNano(), captured, private)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCatalogDateExactGapsTieEdgesAndPageBoundary(t *testing.T) {
	l := routeTestLibrary(t)
	for i := 0; i < 99; i++ {
		insertCatalogDate(t, l, fmt.Sprintf("trip/new-%03d.jpg", i), "2026-06-14T12:00:00Z", "2026-06-14T12:00:00Z", false)
	}
	insertCatalogDate(t, l, "trip/older-first.jpg", "2026-06-10T17:00:00Z", "2026-06-10T12:00:00Z", false)
	insertCatalogDate(t, l, "trip/older-last.jpg", "2026-06-10T12:00:00Z", "2026-06-10T12:00:00Z", false)
	for _, tc := range []struct {
		date, path, day string
		page            int
	}{
		{"2026-06-10", "trip/older-first.jpg", "2026-06-10", 2},
		{"2026-06-12", "trip/older-first.jpg", "2026-06-10", 2},
		{"2026-06-13", "trip/new-098.jpg", "2026-06-14", 1},
		{"0001-01-01", "trip/older-first.jpg", "2026-06-10", 2},
		{"9999-12-31", "trip/new-098.jpg", "2026-06-14", 1},
	} {
		got, err := l.CatalogDate(context.Background(), tc.date, false)
		if err != nil || got.Path != tc.path || got.Date != tc.day || got.Page != tc.page {
			t.Fatalf("%s: %+v %v", tc.date, got, err)
		}
		listing, err := l.List(context.Background(), ListOptions{Recursive: true, Sort: "descending_date", Page: got.Page, PageSize: 96})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, p := range listing.Media {
			found = found || p.Path == got.Path
		}
		if !found {
			t.Fatalf("anchor not in corresponding gallery page: %+v", got)
		}
	}
}

func TestCatalogDateUsesDisplayedSourceDayAndModificationFallback(t *testing.T) {
	l := routeTestLibrary(t)
	insertCatalogDate(t, l, "trip/source.jpg", "2024-01-02T00:15:00+14:00", "2026-01-01T00:00:00Z", false)
	insertCatalogDate(t, l, "trip/modified.jpg", "", "2024-01-03T12:00:00Z", false)
	insertCatalogDate(t, l, "trip/private.jpg", "2024-01-04T12:00:00Z", "2024-01-04T12:00:00Z", true)
	for _, tc := range []struct {
		date, path string
		admin      bool
	}{
		{"2024-01-02", "trip/source.jpg", false},
		{"2024-01-03", "trip/modified.jpg", false},
		{"2024-01-04", "trip/modified.jpg", false},
		{"2024-01-04", "trip/private.jpg", true},
	} {
		got, err := l.CatalogDate(context.Background(), tc.date, tc.admin)
		if err != nil || got.Path != tc.path {
			t.Fatalf("%+v: %+v %v", tc, got, err)
		}
	}
}

func TestCatalogDateEmptyInvalidCancelledAndUnindexed(t *testing.T) {
	l := routeTestLibrary(t)
	got, err := l.CatalogDate(context.Background(), "2026-01-01", false)
	if err != nil || got.Path != "" || got.Date != "" || got.Page != 1 {
		t.Fatalf("empty: %+v %v", got, err)
	}
	for _, date := range []string{"", "2026-02-30", "2026-1-01", "0000-01-01", "2026-01-01T00:00:00Z", "' OR 1=1"} {
		if _, err = l.CatalogDate(context.Background(), date, false); !errors.Is(err, ErrCatalogDate) {
			t.Fatalf("%q: %v", date, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = l.CatalogDate(ctx, "2026-01-01", false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = l.index.db.Exec(`DELETE FROM photo_folder_scan WHERE path=''`); err != nil {
		t.Fatal(err)
	}
	if _, err = l.CatalogDate(context.Background(), "2026-01-01", false); !errors.Is(err, ErrCatalogIndexUnavailable) {
		t.Fatal(err)
	}
}

func TestCatalogDateSeeksAndRankUseCoveringIndexWithoutSort(t *testing.T) {
	l := routeTestLibrary(t)
	for _, condition := range []string{
		"captured_at>'' AND captured_at<'2026-01-01~' ORDER BY captured_at DESC,mod_time_unix_nano DESC,path DESC LIMIT 1",
		"captured_at>='2026-01-01' ORDER BY captured_at ASC,mod_time_unix_nano ASC,path ASC LIMIT 1",
		"captured_at='' AND mod_time_unix_nano>=0 ORDER BY captured_at ASC,mod_time_unix_nano ASC,path ASC LIMIT 1",
		"(captured_at,mod_time_unix_nano,path)>('2026-01-01',0,'a')",
	} {
		rows, err := l.index.db.Query(`EXPLAIN QUERY PLAN SELECT path,captured_at,mod_time_unix_nano FROM media_index INDEXED BY idx_media_index_admin_date WHERE admin_only=0 AND ` + condition)
		if err != nil {
			t.Fatal(err)
		}
		plan := ""
		for rows.Next() {
			var a, b, c int
			var detail string
			if err = rows.Scan(&a, &b, &c, &detail); err != nil {
				t.Fatal(err)
			}
			plan += detail
		}
		rows.Close()
		if !strings.Contains(plan, "COVERING INDEX idx_media_index_admin_date") || strings.Contains(plan, "TEMP B-TREE") {
			t.Fatal(plan)
		}
	}
}

func BenchmarkCatalogDate(b *testing.B) {
	for _, count := range []int{10000, 300000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			l := routeTestLibrary(b)
			_, err := l.index.db.Exec(`WITH RECURSIVE seq(n) AS (VALUES(0) UNION ALL SELECT n+1 FROM seq WHERE n+1<?)
		INSERT INTO media_index(path,name,directory,type,mime_type,size_bytes,mod_time_unix_nano,captured_at,indexed_at)
		SELECT printf('trip/%09d.jpg',n),'image.jpg','trip','image','image/jpeg',1,1700000000000000000+n*1000000000,
		strftime('%Y-%m-%dT%H:%M:%SZ',1700000000+n*3600,'unixepoch'),'2026-09-10' FROM seq`, count)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err = l.CatalogDate(context.Background(), "2025-01-01", false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
