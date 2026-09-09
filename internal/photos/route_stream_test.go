package photos

import (
	"errors"
	"math"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

// Independent batch oracle for the replaced browser implementation. Compare
// every cluster and time bound, not just the number of output points.
func batchRouteReference(input []routeCluster, radius int) []routeCluster {
	for _, window := range routeAggregationWindows {
		var next []routeCluster
		for _, cluster := range input {
			if len(next) > 0 {
				last := &next[len(next)-1]
				if cluster.started.Sub(last.ended) <= window && routeDistanceMeters(last.lat, last.lon, cluster.lat, cluster.lon) <= float64(radius) {
					mergeRouteCluster(last, cluster)
					continue
				}
			}
			next = append(next, cluster)
		}
		input = next
	}
	return input
}

func TestRouteStreamMatchesAllBatchWindows(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	for trial := 0; trial < 100; trial++ {
		radius := routeClusterRadiusMeterOptions[trial%len(routeClusterRadiusMeterOptions)]
		at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		var input, got []routeCluster
		stream := newRouteStream(radius, func(c routeCluster) error { got = append(got, c); return nil })
		lat, lon := 52.0, 13.0
		for i := 0; i < 1000; i++ {
			lat += (random.Float64() - .5) * .04
			lon += (random.Float64() - .5) * .04
			at = at.Add(time.Duration(random.Intn(31*24*60)) * time.Minute)
			c := routeCluster{lat: lat, lon: lon, started: at, ended: at, count: 1}
			input = append(input, c)
			if err := stream.add(c); err != nil {
				t.Fatal(err)
			}
		}
		if err := stream.finish(); err != nil {
			t.Fatal(err)
		}
		want := batchRouteReference(input, radius)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d: stream %d clusters, batch %d", trial, len(got), len(want))
		}
		count := len(got)
		if err := stream.finish(); err != nil || len(got) != count {
			t.Fatal("finish emitted twice")
		}
	}
}

func TestRouteStreamHasConstantStorageAndPropagatesConsumerErrors(t *testing.T) {
	var emitted, count int
	stream := newRouteStream(500, func(c routeCluster) error { emitted++; count += c.count; return nil })
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	allocations := testing.AllocsPerRun(1, func() {
		emitted, count = 0, 0
		for i := 0; i < 100000; i++ {
			at := start.Add(time.Duration(i) * time.Minute)
			if err := stream.add(routeCluster{lat: float64(i % 2), lon: 13, started: at, ended: at, count: 1}); err != nil {
				t.Fatal(err)
			}
		}
		if err := stream.finish(); err != nil {
			t.Fatal(err)
		}
	})
	if emitted != 100000 || count != 100000 || allocations > 1 {
		t.Fatalf("emitted=%d count=%d allocations=%g", emitted, count, allocations)
	}
	want := errors.New("consumer stopped")
	stream = newRouteStream(500, func(routeCluster) error { return want })
	if err := stream.add(routeCluster{lat: 1, lon: 1, started: start, ended: start, count: 1}); err != nil {
		t.Fatal(err)
	}
	if err := stream.finish(); !errors.Is(err, want) {
		t.Fatalf("consumer error: %v", err)
	}
}

func TestPhotoRouteMergesAcrossDateLineAndRejectsInvalidCoordinates(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	lat := 0.0
	east, west, nan := 179.999, -179.999, math.NaN()
	items := []Media{
		{Latitude: &lat, Longitude: &east, ModTime: start},
		{Latitude: &lat, Longitude: &west, ModTime: start.Add(time.Minute)},
		{Latitude: &nan, Longitude: &west, ModTime: start},
	}
	points := routePointsFromMedia(items, 500)
	if len(points) != 1 || points[0].Count != 2 || math.Abs(math.Abs(points[0].Lon)-180) > 1e-8 {
		t.Fatalf("date line route: %+v", points)
	}
	if d := routeDistanceMeters(1, 1, -1, -179); math.IsNaN(d) || d < 20000000 {
		t.Fatalf("antipodal distance: %g", d)
	}
}
