package photos

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGPXSegmentsPreserveRecordedGapsAndNearbySegmentStarts(t *testing.T) {
	xml := `<gpx><trk><trkseg><trkpt lat="1" lon="2"/><trkpt lat="1" lon="2.1"/></trkseg><trkseg><trkpt lat="1" lon="2.1"/><trkpt lat="NaN" lon="3"/><trkpt lat="4" lon="5"/></trkseg></trk><rte><rtept lat="6" lon="7"/></rte></gpx>`
	points, segments, err := decodeGPXSegments(context.Background(), strings.NewReader(xml), 4096, 10)
	if err != nil || len(points) != 5 || len(segments) != 4 {
		t.Fatalf("segments: %v %v %v", points, segments, err)
	}
	if len(segments[0]) != 2 || segments[1][0] != segments[0][1] {
		t.Fatal("adjacent track segments were joined or their shared start was thinned away")
	}
	if &segments[0][0] != &points[0] {
		t.Fatal("segments copied the point backing array")
	}
}

func TestMapTrackClippingPreservesViewportCrossingsAndAntimeridian(t *testing.T) {
	world := MapBounds{-90, -180, 90, 180}
	crossing := [][]GPXPoint{{{0, 179}, {0, -179}}}
	pieces, err := clipMapSegments(context.Background(), crossing, world)
	if err != nil || len(pieces) != 2 {
		t.Fatalf("world wrap: %v %v", pieces, err)
	}
	for _, piece := range pieces {
		if math.Abs(piece[1].Lon-piece[0].Lon) > 1.001 {
			t.Fatalf("long line across world: %v", piece)
		}
	}
	pieces, err = clipMapSegments(context.Background(), crossing, MapBounds{-1, 170, 1, -170})
	if err != nil || len(pieces) != 1 || len(pieces[0]) != 2 || pieces[0][1].Lon-pieces[0][0].Lon != 2 {
		t.Fatalf("dateline viewport: %v %v", pieces, err)
	}
	pieces, err = clipMapSegments(context.Background(), [][]GPXPoint{{{0, -5}, {0, 5}}}, MapBounds{-1, -1, 1, 1})
	if err != nil || len(pieces) != 1 || pieces[0][0].Lon != -1 || pieces[0][1].Lon != 1 {
		t.Fatalf("both endpoints outside: %v %v", pieces, err)
	}
	b := trackBounds(crossing[0])
	if b == nil || b.West != 179 || b.East != -179 {
		t.Fatalf("wide track fit: %+v", b)
	}
}

func TestMapTrackReductionIsBoundedAndNeverJoinsSegments(t *testing.T) {
	points := make([]GPXPoint, 100000)
	for i := range points {
		points[i] = GPXPoint{math.Sin(float64(i) / 30), float64(i) / 10000}
	}
	for _, budget := range []int{32, 128, 2048, 8192} {
		out, simplified, omitted := reduceMapSegments([][]GPXPoint{points}, budget)
		if len(out) != 1 || len(out[0]) != budget || !simplified || omitted != 0 {
			t.Fatalf("budget %d: %d %v %d", budget, len(out[0]), simplified, omitted)
		}
		if out[0][0] != (MapCoordinate{points[0].Lat, wrapMapLongitude(points[0].Lon)}) || out[0][budget-1] != (MapCoordinate{points[len(points)-1].Lat, wrapMapLongitude(points[len(points)-1].Lon)}) {
			t.Fatal("lost endpoints")
		}
	}
	short := make([][]GPXPoint, 20)
	for i := range short {
		short[i] = []GPXPoint{{float64(i), 0}}
	}
	out, simplified, omitted := reduceMapSegments(short, 32)
	if len(out) != 20 || simplified || omitted != 0 {
		t.Fatal("unnecessary segment loss")
	}
	many := make([][]GPXPoint, 1000)
	for i := range many {
		many[i] = []GPXPoint{{float64(i % 80), 1}, {float64(i % 80), 2}}
	}
	out, simplified, omitted = reduceMapSegments(many, 32)
	if len(out) != 16 || !simplified || omitted != 984 {
		t.Fatalf("segment budget: %d %v %d", len(out), simplified, omitted)
	}
	for _, line := range out {
		if len(line) != 2 || line[0][0] != line[1][0] {
			t.Fatal("joined independent segments")
		}
	}
}

func TestGPXGeometryRechecksVisibilityLimitsAndCancellation(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "trip"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "trip", "track.gpx")
	if err := os.WriteFile(path, []byte(`<gpx><trkseg><trkpt lat="1" lon="2"/><trkpt lat="2" lon="3"/></trkseg></gpx>`), 0600); err != nil {
		t.Fatal(err)
	}
	l := newTestLibrary(t, root)
	defer l.Close()
	ctx := context.Background()
	world := MapBounds{-90, -180, 90, 180}
	result, err := l.GPXGeometry(ctx, "trip/track.gpx", world, 32, false)
	if err != nil || result.TotalPoints != 2 || len(result.Segments) != 1 || len(l.gpxCache) != 1 {
		t.Fatalf("geometry: %+v %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(root, "trip", AdminOnlyMarkerName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.GPXGeometry(ctx, "trip/track.gpx", world, 32, false); !errors.Is(err, errAdminOnly) {
		t.Fatalf("cached private track leaked: %v", err)
	}
	if _, err := l.GPXGeometry(ctx, "trip/missing.gpx", world, 32, false); !errors.Is(err, errAdminOnly) {
		t.Fatalf("private filename existence leaked: %v", err)
	}
	if _, err := l.GPXGeometry(ctx, "trip/track.gpx", world, 32, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(root, "alias.gpx")); err != nil {
		t.Fatal(err)
	}
	if _, err := l.GPXGeometry(ctx, "alias.gpx", world, 32, false); err == nil {
		t.Fatal("symlink bypassed folder privacy")
	}
	if _, err := l.GPXGeometry(ctx, "trip/track.gpx", world, 8193, true); !errors.Is(err, ErrMapPointLimit) {
		t.Fatalf("point budget: %v", err)
	}
	l.gpxGeometryGate = make(chan struct{}, 1)
	l.gpxGeometryGate <- struct{}{}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := l.GPXGeometry(cancelled, "trip/track.gpx", world, 32, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting geometry: %v", err)
	}
}
