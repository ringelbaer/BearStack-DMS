package photos

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"
)

const routeTimeFormat = "2006-01-02T15:04:05.000000000Z"
const routeTimeSQL = "bearstack_route_time(mi.captured_at, mi.mod_time_unix_nano)"

func ensurePhotoMapIndexes(ctx context.Context, db *sql.DB) error {
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*)=2 FROM sqlite_schema WHERE type='index' AND name IN ('idx_media_index_route_time','idx_media_index_public_gps_directory')`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	conn, release, err := photoFileTempConn(ctx, db)
	if err != nil {
		return err
	}
	defer release()
	_, err = conn.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_media_index_route_time ON media_index(bearstack_route_time(captured_at, mod_time_unix_nano), latitude, longitude, admin_only, directory, type) WHERE latitude IS NOT NULL AND longitude IS NOT NULL`)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_media_index_public_gps_directory ON media_index(directory) WHERE admin_only=0 AND latitude IS NOT NULL AND longitude IS NOT NULL`)
	return err
}

// Fixed-width UTC keys preserve full RFC3339 precision and mixed source offsets.
// The expression index computes them when metadata changes, not on each map pan.
func routeTimeKey(captured string, modified int64) string {
	at, err := time.Parse(time.RFC3339Nano, captured)
	if err != nil {
		at = time.Unix(0, modified)
	}
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(routeTimeFormat)
}

type MapPhotoRoute struct {
	MapTrackGeometry
	TotalMedia   int `json:"total_media"`
	RadiusMeters int `json:"radius_meters"`
}

// Route grouping precedes viewport clipping. Cropping the input would alter
// stays and connect unrelated visits whenever the user moves the map.
func (l *Library) MapPhotoRoute(ctx context.Context, opts ListOptions, viewport MapBounds, maxPoints int) (MapPhotoRoute, error) {
	for attempt := 0; attempt < 3; attempt++ {
		result, err := l.mapPhotoRouteAttempt(ctx, opts, viewport, maxPoints)
		if !errors.Is(err, errPhotoRouteChanged) && !errors.Is(err, errPhotoRouteCacheCorrupt) {
			return result, err
		}
	}
	return MapPhotoRoute{}, ErrMapIndexUnavailable
}

func (l *Library) mapPhotoRouteAttempt(ctx context.Context, opts ListOptions, viewport MapBounds, maxPoints int) (MapPhotoRoute, error) {
	result := MapPhotoRoute{MapTrackGeometry: MapTrackGeometry{Path: opts.Path, Segments: [][]MapCoordinate{}}, RadiusMeters: NormalizeRouteClusterRadiusMeters(opts.RouteClusterRadiusMeters)}
	if !viewport.Valid() {
		return result, ErrMapBounds
	}
	if maxPoints < 32 || maxPoints > 8192 {
		return result, ErrMapPointLimit
	}
	query, err := l.mapQuery(ctx, opts, MapBounds{-90, -180, 90, 180})
	if err != nil {
		return result, err
	}
	geometry := routeGeometry{viewport: viewport, budget: maxPoints}
	bound := MapBounds{90, 180, -90, -180}
	shiftedWest, shiftedEast := math.Inf(1), math.Inf(-1)
	emit := func(c routeCluster) error {
		if c.count > int(^uint(0)>>1)-result.TotalMedia {
			return errPhotoRouteCacheCorrupt
		}
		result.TotalPoints++
		result.TotalMedia += c.count
		bound.South = math.Min(bound.South, c.lat)
		bound.North = math.Max(bound.North, c.lat)
		bound.West = math.Min(bound.West, c.lon)
		bound.East = math.Max(bound.East, c.lon)
		lon := c.lon
		if lon < 0 {
			lon += 360
		}
		shiftedWest = math.Min(shiftedWest, lon)
		shiftedEast = math.Max(shiftedEast, lon)
		geometry.add(GPXPoint{c.lat, c.lon})
		return ctx.Err()
	}
	if err = l.consumePhotoRoute(ctx, query, result.RadiusMeters, emit); err != nil {
		return result, err
	}

	result.Segments, result.Simplified, result.OmittedSegments = geometry.finish()
	if result.TotalPoints > 0 {
		bounds := MapResult{Bounds: &bound}
		bounds.finishBounds(shiftedWest, shiftedEast)
		result.Bounds = bounds.Bounds
	}
	return result, ctx.Err()
}

// Stream the complete grouped route. Both uncached searches and the persistent
// writer use this path; neither viewport nor response point budget affects it.
func (l *Library) scanPhotoRoute(ctx context.Context, query indexMediaOptions, radius int, emit func(routeCluster) error) error {
	stream := newRouteStream(radius, emit)
	var err error

	if query.Plan.PostFilter {
		// Reuse the existing explicit candidate limit and visibility-aware search
		// postfilter. Never silently return the first 10,000 of a larger route.
		items, _, e := l.indexMedia(ctx, query)
		if e != nil {
			return e
		}
		for _, c := range routeClustersFromMedia(items) {
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = stream.add(c); err != nil {
				return err
			}
		}
	} else {
		statement, args := routeIndexQuery(query)
		rows, closeRows, e := l.routeRows(ctx, statement, args, query.Plan.FTSQuery != "")
		if e != nil {
			return e
		}
		defer closeRows()
		for rows.Next() {
			var lat, lon float64
			var key string
			if err = rows.Scan(&lat, &lon, &key); err != nil {
				return err
			}
			if key == "" {
				continue
			}
			at, e := time.Parse(routeTimeFormat, key)
			if e != nil {
				return e
			}
			if err = stream.add(routeCluster{lat: lat, lon: lon, started: at, ended: at, count: 1}); err != nil {
				return err
			}
		}
		if err = rows.Err(); err != nil {
			return err
		}
	}
	return stream.finish()
}

func routeIndexQuery(query indexMediaOptions) (string, []any) {
	where, args, joinSearch := indexWhere(query)
	from := "media_index mi INDEXED BY idx_media_index_route_time"
	if joinSearch {
		// Consume FTS once, then look up matching metadata. Reopening an FTS
		// cursor for every chronological row is quadratic on broad searches.
		from = "media_search CROSS JOIN media_index mi ON mi.rowid=media_search.rowid"
	}
	return "SELECT mi.latitude,mi.longitude," + routeTimeSQL + " FROM " + from + where +
		" ORDER BY " + routeTimeSQL + ",mi.latitude,mi.longitude", args
}

func (l *Library) routeRows(ctx context.Context, statement string, args []any, search bool) (*sql.Rows, func(), error) {
	if !search {
		rows, err := l.index.db.QueryContext(ctx, statement, args...)
		return rows, func() {
			if rows != nil {
				rows.Close()
			}
		}, err
	}
	// Search matches require chronological sorting. Lease one connection and
	// let SQLite spill its sorter to private temporary files instead of growing
	// the index pool's usual in-memory temp store with the collection size.
	conn, closeConn, err := photoFileTempConn(ctx, l.index.db)
	if err != nil {
		return nil, nil, err
	}
	rows, err := conn.QueryContext(ctx, statement, args...)
	if err != nil {
		closeConn()
		return nil, nil, err
	}
	return rows, func() { rows.Close(); closeConn() }, nil
}

// Clip first, then compact a bounded window. Endpoints and disconnected paths
// survive each compaction; even a huge sequence of viewport crossings cannot
// accumulate unbounded geometry. Zooming re-reads the source at the new extent.
type routeGeometry struct {
	viewport    MapBounds
	budget      int
	previous    GPXPoint
	hasPrevious bool
	segments    [][]GPXPoint
	points      int
	join        bool
	simplified  bool
	omitted     int
}

func (g *routeGeometry) add(point GPXPoint) {
	if !g.hasPrevious {
		g.previous = point
		g.hasPrevious = true
		return
	}
	if !clippedMapEdge(g.previous, point, g.viewport, func(a, b GPXPoint) {
		if !g.join || len(g.segments) == 0 || g.segments[len(g.segments)-1][len(g.segments[len(g.segments)-1])-1] != a {
			g.segments = append(g.segments, []GPXPoint{a})
			g.points++
		}
		last := len(g.segments) - 1
		if g.segments[last][len(g.segments[last])-1] != b {
			g.segments[last] = append(g.segments[last], b)
			g.points++
		}
		g.join = true
	}) {
		g.join = false
	}
	g.previous = point
	if g.points > g.budget*4 {
		g.compact(g.budget * 2)
	}
}

func (g *routeGeometry) compact(budget int) {
	var simplified bool
	var omitted int
	g.segments, simplified, omitted = reduceGPXSegments(g.segments, budget)
	g.simplified = g.simplified || simplified
	g.omitted += omitted
	g.points = 0
	for _, s := range g.segments {
		g.points += len(s)
	}
}

func (g *routeGeometry) finish() ([][]MapCoordinate, bool, int) {
	if g.hasPrevious && len(g.segments) == 0 && !g.join {
		// A one-cluster route is still a meaningful place. For a route wholly
		// outside the viewport this degenerate edge emits nothing.
		clippedMapEdge(g.previous, g.previous, g.viewport, func(a, _ GPXPoint) { g.segments = append(g.segments, []GPXPoint{a}) })
	}
	g.compact(g.budget)
	return mapCoordinateSegments(g.segments), g.simplified, g.omitted
}
