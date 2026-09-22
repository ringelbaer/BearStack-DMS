package photos

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestSearchSemanticsAcrossMediaRowsAndSQL(t *testing.T) {
	row := cachedMediaRow{
		Path: "album/Äpfel.jpg", Name: "Äpfel.jpg", Directory: "album", Type: MediaTypeImage,
		Camera: "Sony", Lens: "Prime", Orientation: "landscape", Width: 6000, Height: 4000,
		CapturedAt: "2024-01-01T00:15:00+02:00", ModTimeUnixNano: time.Date(2023, 12, 31, 22, 15, 0, 0, time.UTC).UnixNano(),
		Latitude: sql.NullFloat64{Float64: 48, Valid: true}, Longitude: sql.NullFloat64{Float64: 11, Valid: true},
		Tags: `["Urlaub","Familie"]`, Keywords: `["Sommer"]`, Faces: `[{"name":"Jürgen"}]`,
	}
	lib := routeTestLibrary(t)
	media := mediaFromCachedRow(row)
	if err := lib.saveMediaContext(context.Background(), media); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.index.db.Exec(`UPDATE photo_stats SET value=1 WHERE key IN ('media_count','gps_media_count')`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		want  bool
	}{
		{"", true}, {"tag:urlaub", true}, {"-tag:urlaub", false}, {"-tag:winter", true},
		{"tag:winter or person:Juergen", true}, {"tag:winter or person:Anna", false},
		{"camera:sony and keyword:sommer", true}, {"file_name:äpf*", true}, {"directory:album", true},
		{"person:Juergen", true}, {"-person:Juergen", false}, {"date:2024", true}, {"date:2023", false},
		{"date:2024-01", true}, {"date:2024-01-01", true}, {"date:2023-12-31", false},
		{"gps:true", true}, {"gps:off", false}, {"resolution:>=24", true}, {"resolution:>24", false},
		{"2-of:(camera:sony,person:Juergen,tag:winter)", true}, {"3-of:(camera:sony,person:Juergen,tag:winter)", false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			query := compileMediaQuery(tc.query)
			if got := query.matches(media); got != tc.want {
				t.Fatalf("Media=%v want=%v", got, tc.want)
			}
			if got := query.matchesRow(row); got != tc.want {
				t.Fatalf("row=%v want=%v", got, tc.want)
			}
			got, _, err := lib.indexMedia(context.Background(), indexMediaOptions{Query: tc.query, Plan: indexQueryPlanFor(tc.query), Limit: 10, RequestSort: "ascending_name", IncludeAdminOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			if (len(got) == 1) != tc.want {
				t.Fatalf("SQL matches=%d want=%v", len(got), tc.want)
			}
		})
	}
}

func TestSearchDateFallbackAndAbsentGPSAgree(t *testing.T) {
	row := cachedMediaRow{Type: MediaTypeImage, ModTimeUnixNano: time.Date(2022, 3, 4, 12, 0, 0, 0, time.UTC).UnixNano()}
	for _, query := range []string{"date:2022", "date:2022-03", "date:2022-03-04", "gps:false", "-gps:true"} {
		compiled := compileMediaQuery(query)
		if !compiled.matchesRow(row) || !compiled.matches(mediaFromCachedRow(row)) {
			t.Fatalf("fallback mismatch for %s", query)
		}
	}
}
