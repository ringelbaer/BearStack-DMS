package photos

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"

	"bearstack/internal/photos/photopath"
)

var ErrMapPointLimit = errors.New("invalid map point budget")
var ErrGPXInvalid = errors.New("invalid GPX track")

func ErrGPXTooLarge() error { return errGPXLimit }

// Coordinates use compact [latitude,longitude] pairs. The bounds describe the
// complete track; segments describe only the requested viewport and point budget.
type MapCoordinate [2]float64
type MapTrackGeometry struct {
	Path            string            `json:"path"`
	Name            string            `json:"name"`
	Bounds          *MapBounds        `json:"bounds,omitempty"`
	Segments        [][]MapCoordinate `json:"segments"`
	TotalPoints     int               `json:"total_points"`
	Simplified      bool              `json:"simplified"`
	OmittedSegments int               `json:"omitted_segments"`
}

func (l *Library) GPXGeometry(ctx context.Context, path string, viewport MapBounds, maxPoints int, includeAdminOnly bool) (MapTrackGeometry, error) {
	result := MapTrackGeometry{Segments: [][]MapCoordinate{}}
	if !viewport.Valid() {
		return result, ErrMapBounds
	}
	if maxPoints < 32 || maxPoints > 8192 {
		return result, ErrMapPointLimit
	}
	rel, err := CleanPath(path)
	if err != nil {
		return result, err
	}
	if rel == "" || !strings.EqualFold(filepath.Ext(rel), ".gpx") {
		return result, os.ErrNotExist
	}
	hidden, err := l.MediaAdminOnly(rel)
	if err != nil {
		return result, err
	}
	if hidden && !includeAdminOnly {
		return result, errAdminOnly
	}
	// Authorize the folder before probing the leaf, so private filenames cannot
	// be distinguished through missing-file versus access-denied responses.
	// Match the scanner's symlink policy, including links within the library.
	if err := photopath.RejectSymlinkPath(l.root, rel); err != nil {
		return result, err
	}
	abs, err := l.Resolve(rel)
	if err != nil {
		return result, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, os.ErrNotExist
	}
	release, err := l.acquireMapGeometry(ctx)
	if err != nil {
		return result, err
	}
	defer release()
	track, err := l.gpxFromPathInfo(ctx, rel, info)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, errGPXLimit) || errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
			return result, err
		}
		return result, ErrGPXInvalid
	}
	result.Path, result.Name = track.Path, track.Name
	result.TotalPoints = len(track.Points)
	result.Bounds = trackBounds(track.Points)
	clipped, err := clipMapSegments(ctx, track.Segments, viewport)
	if err != nil {
		return result, err
	}
	result.Segments, result.Simplified, result.OmittedSegments = reduceMapSegments(clipped, maxPoints)
	return result, ctx.Err()
}

// GPX and photo routes share the same two geometry slots per library.
func (l *Library) acquireMapGeometry(ctx context.Context) (func(), error) {
	l.gpxMu.Lock()
	if l.gpxGeometryGate == nil {
		l.gpxGeometryGate = make(chan struct{}, 2)
	}
	gate := l.gpxGeometryGate
	l.gpxMu.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func trackBounds(points []GPXPoint) *MapBounds {
	if len(points) == 0 {
		return nil
	}
	b := MapBounds{90, 180, -90, -180}
	shiftedWest, shiftedEast := math.Inf(1), math.Inf(-1)
	for _, p := range points {
		b.South = math.Min(b.South, p.Lat)
		b.North = math.Max(b.North, p.Lat)
		b.West = math.Min(b.West, p.Lon)
		b.East = math.Max(b.East, p.Lon)
		lon := p.Lon
		if lon < 0 {
			lon += 360
		}
		shiftedWest = math.Min(shiftedWest, lon)
		shiftedEast = math.Max(shiftedEast, lon)
	}
	result := MapResult{Bounds: &b}
	result.finishBounds(shiftedWest, shiftedEast)
	return result.Bounds
}

func wrapMapLongitude(lon float64) float64 { return math.Mod(math.Mod(lon+180, 360)+360, 360) - 180 }

// Clip individual shortest-longitude edges and include their visible world
// copies. This produces two separate segments at +/-180 for a world viewport,
// and a continuous short segment for an antimeridian-crossing viewport.
func clipMapSegments(ctx context.Context, segments [][]GPXPoint, bounds MapBounds) ([][]GPXPoint, error) {
	east := bounds.East
	if east < bounds.West {
		east += 360
	}
	center := (bounds.West + east) / 2
	var result [][]GPXPoint
	for _, segment := range segments {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var current []GPXPoint
		flush := func() {
			if len(current) > 0 {
				result = append(result, current)
				current = nil
			}
		}
		if len(segment) == 1 {
			p := segment[0]
			p.Lon = center + wrapMapLongitude(p.Lon-center)
			if p.Lat >= bounds.South && p.Lat <= bounds.North && p.Lon >= bounds.West && p.Lon <= east {
				result = append(result, []GPXPoint{p})
			}
		}
		for i := 1; i < len(segment); i++ {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			visible := clippedMapEdge(segment[i-1], segment[i], bounds, func(x, y GPXPoint) {
				if len(current) == 0 || current[len(current)-1] != x {
					flush()
					current = append(current, x)
				}
				if current[len(current)-1] != y {
					current = append(current, y)
				}
			})
			if !visible {
				flush()
			}
		}
		flush()
	}
	return result, nil
}

// Emit visible copies of one shortest-longitude edge without allocating.
func clippedMapEdge(a, b GPXPoint, bounds MapBounds, emit func(GPXPoint, GPXPoint)) bool {
	east := bounds.East
	if east < bounds.West {
		east += 360
	}
	center := (bounds.West + east) / 2
	a.Lon = center + wrapMapLongitude(a.Lon-center)
	b.Lon = a.Lon + wrapMapLongitude(b.Lon-a.Lon)
	visible := false
	for _, shift := range [...]float64{-360, 0, 360} {
		x, y := a, b
		x.Lon += shift
		y.Lon += shift
		x, y, ok := clipMapEdge(x, y, MapBounds{bounds.South, bounds.West, bounds.North, east})
		if ok {
			visible = true
			emit(x, y)
		}
	}
	return visible
}

// Liang-Barsky clipping uses constant memory and preserves boundary crossings
// even when both recorded endpoints are outside the viewport.
func clipMapEdge(a, b GPXPoint, v MapBounds) (GPXPoint, GPXPoint, bool) {
	dx, dy := b.Lon-a.Lon, b.Lat-a.Lat
	lo, hi := 0.0, 1.0
	for _, edge := range [][2]float64{{-dx, a.Lon - v.West}, {dx, v.East - a.Lon}, {-dy, a.Lat - v.South}, {dy, v.North - a.Lat}} {
		p, q := edge[0], edge[1]
		if p == 0 {
			if q < 0 {
				return a, b, false
			}
			continue
		}
		t := q / p
		if p < 0 {
			lo = math.Max(lo, t)
		} else {
			hi = math.Min(hi, t)
		}
		if lo > hi {
			return a, b, false
		}
	}
	return GPXPoint{a.Lat + lo*dy, a.Lon + lo*dx}, GPXPoint{a.Lat + hi*dy, a.Lon + hi*dx}, true
}

func reduceMapSegments(segments [][]GPXPoint, budget int) ([][]MapCoordinate, bool, int) {
	reduced, simplified, omitted := reduceGPXSegments(segments, budget)
	return mapCoordinateSegments(reduced), simplified, omitted
}

func mapCoordinateSegments(segments [][]GPXPoint) [][]MapCoordinate {
	out := make([][]MapCoordinate, 0, len(segments))
	for _, segment := range segments {
		line := make([]MapCoordinate, 0, len(segment))
		for _, p := range segment {
			line = append(line, MapCoordinate{p.Lat, wrapMapLongitude(p.Lon)})
		}
		out = append(out, line)
	}
	return out
}

func reduceGPXSegments(segments [][]GPXPoint, budget int) ([][]GPXPoint, bool, int) {
	out := [][]GPXPoint{}
	total := 0
	for _, segment := range segments {
		total += len(segment)
	}
	simplified := total > budget
	omitted := 0
	// Preserve gaps if even segment endpoints exceed the budget. Sample whole
	// segments throughout the track and report omissions explicitly.
	endpoints := 0
	for _, segment := range segments {
		endpoints += min(2, len(segment))
	}
	if endpoints > budget {
		retained := make([][]GPXPoint, 0, budget/2)
		for i := 0; i < budget/2; i++ {
			retained = append(retained, segments[i*(len(segments)-1)/(budget/2-1)])
		}
		omitted = len(segments) - len(retained)
		segments = retained
	}
	base, extra := 0, 0
	for _, s := range segments {
		base += min(2, len(s))
		extra += max(0, len(s)-2)
	}
	remaining := min(budget-base, extra)
	used, allocated := 0, 0
	for _, s := range segments {
		used += max(0, len(s)-2)
		share := 0
		if extra > 0 {
			share = remaining*used/extra - allocated
		}
		allocated += share
		sampled := sampleMapLine(s, min(2, len(s))+share)
		if len(sampled) > 0 {
			out = append(out, sampled)
		}
	}
	return out, simplified, omitted
}

// Largest-triangle buckets retain bends and endpoints in O(n) work; no recursive
// Douglas-Peucker worst case or unbounded geometry allocation on the server.
func sampleMapLine(points []GPXPoint, budget int) []GPXPoint {
	return sampleMapValues(points, budget, func(p GPXPoint) GPXPoint { return p })
}

// Retain the source values so browser route samples keep their timestamps and
// counts while sharing the same geometry reduction as native maps and GPX.
func sampleMapValues[T any](points []T, budget int, coordinate func(T) GPXPoint) []T {
	if len(points) <= budget {
		return points
	}
	if budget < 3 {
		return []T{points[0], points[len(points)-1]}
	}
	out := make([]T, 0, budget)
	out = append(out, points[0])
	step := float64(len(points)-2) / float64(budget-2)
	chosen := 0
	for i := 0; i < budget-2; i++ {
		avgStart := int(math.Floor(float64(i+1)*step)) + 1
		avgEnd := min(int(math.Floor(float64(i+2)*step))+1, len(points))
		var avg GPXPoint
		for j := avgStart; j < avgEnd; j++ {
			p := coordinate(points[j])
			avg.Lat += p.Lat
			avg.Lon += p.Lon
		}
		count := float64(avgEnd - avgStart)
		avg.Lat /= count
		avg.Lon /= count
		start := int(math.Floor(float64(i)*step)) + 1
		end := min(int(math.Floor(float64(i+1)*step))+1, len(points)-1)
		area, best := -1.0, start
		a := coordinate(points[chosen])
		for j := start; j < end; j++ {
			b := coordinate(points[j])
			triangle := math.Abs((a.Lon-avg.Lon)*(b.Lat-a.Lat) - (a.Lon-b.Lon)*(avg.Lat-a.Lat))
			if triangle > area {
				area = triangle
				best = j
			}
		}
		out = append(out, points[best])
		chosen = best
	}
	return append(out, points[len(points)-1])
}
