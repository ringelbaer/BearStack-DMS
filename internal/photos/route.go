// Datei erzeugt URL- und Routeninformationen fuer Foto-, Ordner- und Medienpfade.
package photos

import (
	"math"
	"sort"
	"time"
)

const DefaultRouteClusterRadiusMeters = 1000

var routeClusterRadiusMeterOptions = []int{500, 1000, 1500, 2000, 3000, 5000, 7500, 10000}

var routeAggregationWindows = []time.Duration{
	30 * time.Minute,
	time.Hour,
	2 * time.Hour,
	4 * time.Hour,
	8 * time.Hour,
	12 * time.Hour,
	24 * time.Hour,
	48 * time.Hour,
	7 * 24 * time.Hour,
	30 * 24 * time.Hour,
}

type routeCluster struct {
	lat     float64
	lon     float64
	started time.Time
	ended   time.Time
	count   int
}

func NormalizeRouteClusterRadiusMeters(value int) int {
	if value <= 0 {
		return DefaultRouteClusterRadiusMeters
	}
	best := routeClusterRadiusMeterOptions[0]
	bestDiff := absInt(value - best)
	for _, candidate := range routeClusterRadiusMeterOptions[1:] {
		if diff := absInt(value - candidate); diff < bestDiff {
			best = candidate
			bestDiff = diff
		}
	}
	return best
}

func routePointsFromMedia(items []Media, radiusMeters int) []RoutePoint {
	radiusMeters = NormalizeRouteClusterRadiusMeters(radiusMeters)
	clusters := routeClustersFromMedia(items)
	if len(clusters) == 0 {
		return nil
	}
	for _, window := range routeAggregationWindows {
		clusters = mergeRouteClusters(clusters, window, radiusMeters)
		if !hasNearbyAdjacentRouteClusters(clusters, radiusMeters) {
			break
		}
	}
	points := make([]RoutePoint, 0, len(clusters))
	for i, cluster := range clusters {
		points = append(points, RoutePoint{
			Order:     i + 1,
			Lat:       cluster.lat,
			Lon:       cluster.lon,
			StartedAt: cluster.started,
			EndedAt:   cluster.ended,
			Count:     cluster.count,
		})
	}
	return points
}

func routeClustersFromMedia(items []Media) []routeCluster {
	clusters := make([]routeCluster, 0, len(items))
	for _, item := range items {
		if item.Latitude == nil || item.Longitude == nil {
			continue
		}
		lat, lon := *item.Latitude, *item.Longitude
		if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			continue
		}
		at := mediaDate(item)
		if at.IsZero() {
			continue
		}
		clusters = append(clusters, routeCluster{
			lat:     lat,
			lon:     lon,
			started: at,
			ended:   at,
			count:   1,
		})
	}
	sort.SliceStable(clusters, func(i, j int) bool {
		if clusters[i].started.Equal(clusters[j].started) {
			if clusters[i].lat == clusters[j].lat {
				return clusters[i].lon < clusters[j].lon
			}
			return clusters[i].lat < clusters[j].lat
		}
		return clusters[i].started.Before(clusters[j].started)
	})
	return clusters
}

func mergeRouteClusters(clusters []routeCluster, window time.Duration, radiusMeters int) []routeCluster {
	if len(clusters) < 2 {
		return clusters
	}
	merged := make([]routeCluster, 0, len(clusters))
	for _, cluster := range clusters {
		if len(merged) == 0 {
			merged = append(merged, cluster)
			continue
		}
		last := &merged[len(merged)-1]
		if routeDistanceMeters(last.lat, last.lon, cluster.lat, cluster.lon) <= float64(radiusMeters) && cluster.started.Sub(last.ended) <= window {
			mergeRouteCluster(last, cluster)
			continue
		}
		merged = append(merged, cluster)
	}
	return merged
}

func mergeRouteCluster(dst *routeCluster, src routeCluster) {
	total := dst.count + src.count
	if total <= 0 {
		return
	}
	dst.lat = (dst.lat*float64(dst.count) + src.lat*float64(src.count)) / float64(total)
	dst.lon = (dst.lon*float64(dst.count) + src.lon*float64(src.count)) / float64(total)
	if src.started.Before(dst.started) {
		dst.started = src.started
	}
	if src.ended.After(dst.ended) {
		dst.ended = src.ended
	}
	dst.count = total
}

func hasNearbyAdjacentRouteClusters(clusters []routeCluster, radiusMeters int) bool {
	for i := 1; i < len(clusters); i++ {
		if routeDistanceMeters(clusters[i-1].lat, clusters[i-1].lon, clusters[i].lat, clusters[i].lon) <= float64(radiusMeters) {
			return true
		}
	}
	return false
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func routeDistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusMeters = 6371000
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	deltaPhi := (lat2 - lat1) * math.Pi / 180
	deltaLambda := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	return earthRadiusMeters * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
