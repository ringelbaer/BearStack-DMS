package photos

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
)

// MapBounds can cross the antimeridian (West > East). Coordinates remain
// geographic; projecting and choosing the visible viewport is a client concern.
type MapBounds struct {
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}

func (b MapBounds) Valid() bool {
	return b.South >= -90 && b.North <= 90 && b.South < b.North &&
		b.West >= -180 && b.West <= 180 && b.East >= -180 && b.East <= 180 && b.West != b.East
}

var ErrMapIndexUnavailable = errors.New("photo map index is not ready")
var ErrMapBounds = errors.New("invalid photo map bounds")

type MapMarker struct {
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Count     int       `json:"count"`
	Bounds    MapBounds `json:"bounds"`
	// A single-item marker can be opened through the regular media/info API.
	Path string `json:"path,omitempty"`
}

type MapResult struct {
	Total   int         `json:"total"`
	Bounds  *MapBounds  `json:"bounds,omitempty"`
	Markers []MapMarker `json:"markers"`
}

// Map aggregates the complete indexed selection, not a truncated media page.
// There are at most 17*17 cells, independent of the library's file count.
// Search and visibility use the same query plan as the gallery. Complex search
// retains the existing explicit candidate limit instead of returning partial data.
func (l *Library) Map(ctx context.Context, opts ListOptions, viewport MapBounds) (MapResult, error) {
	result := MapResult{Markers: []MapMarker{}}
	query, err := l.mapQuery(ctx, opts, viewport)
	if err != nil {
		return result, err
	}
	if query.Plan.PostFilter {
		items, _, err := l.indexMedia(ctx, query)
		if err != nil {
			return result, err
		}
		return aggregateMapMedia(items, viewport), nil
	}
	where, args, joinSearch := indexWhere(query)
	longitude := "mi.longitude"
	if viewport.West > viewport.East {
		longitude = "CASE WHEN mi.longitude < ? THEN mi.longitude + 360 ELSE mi.longitude END"
		args = append([]any{viewport.West}, args...)
	}
	from := "media_index mi"
	if joinSearch {
		from += " JOIN media_search ON media_search.rowid = mi.rowid"
	}
	latStep, lonStep := viewport.steps()
	args = append(args, viewport.South, latStep, viewport.West, lonStep)
	rows, err := l.index.db.QueryContext(ctx, `WITH points AS (
		SELECT mi.path, mi.latitude AS lat, mi.longitude AS raw_lon, `+longitude+` AS lon FROM `+from+where+`
	) SELECT COUNT(*), AVG(lat), AVG(lon), MIN(path), MIN(lat), MIN(lon), MAX(lat), MAX(lon),
	MIN(CASE WHEN raw_lon < 0 THEN raw_lon+360 ELSE raw_lon END), MAX(CASE WHEN raw_lon < 0 THEN raw_lon+360 ELSE raw_lon END)
	FROM points GROUP BY CAST((lat - ?) / ? AS INTEGER), CAST((lon - ?) / ? AS INTEGER)
	ORDER BY MIN(lat), MIN(lon)`, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	shiftedWest, shiftedEast := math.Inf(1), math.Inf(-1)
	for rows.Next() {
		var marker MapMarker
		var bounds MapBounds
		var west, east float64
		if err := rows.Scan(&marker.Count, &marker.Latitude, &marker.Longitude, &marker.Path,
			&bounds.South, &bounds.West, &bounds.North, &bounds.East, &west, &east); err != nil {
			return result, err
		}
		if marker.Count != 1 {
			marker.Path = ""
		}
		marker.Longitude = mapLongitude(marker.Longitude)
		result.add(marker, bounds)
		shiftedWest = math.Min(shiftedWest, west)
		shiftedEast = math.Max(shiftedEast, east)
	}
	result.finishBounds(shiftedWest, shiftedEast)
	result.sortMarkers()
	return result, rows.Err()
}

func (b MapBounds) steps() (float64, float64) {
	east := b.East
	if b.West > east {
		east += 360
	}
	return math.Max((b.North-b.South)/16, 1e-9), math.Max((east-b.West)/16, 1e-9)
}

func mapLongitude(lon float64) float64 {
	if lon > 180 {
		return lon - 360
	}
	return lon
}

func (r *MapResult) add(marker MapMarker, b MapBounds) {
	marker.Bounds = b
	marker.Bounds.West = mapLongitude(b.West)
	marker.Bounds.East = mapLongitude(b.East)
	r.Markers = append(r.Markers, marker)
	r.Total += marker.Count
	if r.Bounds == nil {
		r.Bounds = &b
		return
	}
	r.Bounds.South = math.Min(r.Bounds.South, b.South)
	r.Bounds.West = math.Min(r.Bounds.West, b.West)
	r.Bounds.North = math.Max(r.Bounds.North, b.North)
	r.Bounds.East = math.Max(r.Bounds.East, b.East)
}

func aggregateMapMedia(items []Media, viewport MapBounds) MapResult {
	type cell struct {
		marker MapMarker
		bounds MapBounds
	}
	cells := map[[2]int]*cell{}
	latStep, lonStep := viewport.steps()
	shiftedWest, shiftedEast := math.Inf(1), math.Inf(-1)
	for _, item := range items {
		if item.Latitude == nil || item.Longitude == nil {
			continue
		}
		lat, lon := *item.Latitude, *item.Longitude
		if lat < viewport.South || lat > viewport.North || lon < -180 || lon > 180 {
			continue
		}
		if viewport.West > viewport.East {
			if lon < viewport.West && lon > viewport.East {
				continue
			}
			if lon < viewport.West {
				lon += 360
			}
		} else if lon < viewport.West || lon > viewport.East {
			continue
		}
		shifted := *item.Longitude
		if shifted < 0 {
			shifted += 360
		}
		shiftedWest = math.Min(shiftedWest, shifted)
		shiftedEast = math.Max(shiftedEast, shifted)
		key := [2]int{int((lat - viewport.South) / latStep), int((lon - viewport.West) / lonStep)}
		c := cells[key]
		if c == nil {
			cells[key] = &cell{MapMarker{Latitude: lat, Longitude: lon, Count: 1, Path: item.Path}, MapBounds{lat, lon, lat, lon}}
			continue
		}
		c.marker.Count++
		c.marker.Latitude += lat
		c.marker.Longitude += lon
		c.marker.Path = ""
		c.bounds.South = math.Min(c.bounds.South, lat)
		c.bounds.North = math.Max(c.bounds.North, lat)
		c.bounds.West = math.Min(c.bounds.West, lon)
		c.bounds.East = math.Max(c.bounds.East, lon)
	}
	result := MapResult{Markers: []MapMarker{}}
	for _, c := range cells {
		c.marker.Latitude /= float64(c.marker.Count)
		c.marker.Longitude = mapLongitude(c.marker.Longitude / float64(c.marker.Count))
		result.add(c.marker, c.bounds)
	}
	result.sortMarkers()
	result.finishBounds(shiftedWest, shiftedEast)
	return result
}

// Compare the ordinary and Pacific-centred extents. A trip straddling the date
// line should initially fit its surroundings instead of almost the whole world.
func (r *MapResult) finishBounds(west, east float64) {
	if r.Bounds == nil {
		return
	}
	if east-west < r.Bounds.East-r.Bounds.West {
		r.Bounds.West = west
		r.Bounds.East = east
	}
	r.Bounds.West = mapLongitude(r.Bounds.West)
	r.Bounds.East = mapLongitude(r.Bounds.East)
}

func (r *MapResult) sortMarkers() {
	sort.Slice(r.Markers, func(i, j int) bool {
		a, b := r.Markers[i], r.Markers[j]
		if a.Latitude != b.Latitude {
			return a.Latitude < b.Latitude
		}
		return a.Longitude < b.Longitude
	})
}

func (l *Library) mapQuery(ctx context.Context, opts ListOptions, viewport MapBounds) (indexMediaOptions, error) {
	if !viewport.Valid() {
		return indexMediaOptions{}, ErrMapBounds
	}
	rel, err := CleanPath(opts.Path)
	if err != nil {
		return indexMediaOptions{}, err
	}
	opts.Query = strings.TrimSpace(opts.Query)
	if opts.Query != "" {
		rel = ""
	}
	private, err := l.FolderAdminOnly(rel)
	if err != nil {
		return indexMediaOptions{}, err
	}
	if private && !opts.IncludeAdminOnly {
		return indexMediaOptions{}, errAdminOnly
	}
	if !l.index.available() {
		return indexMediaOptions{}, ErrMapIndexUnavailable
	}
	if err := l.index.waitMapIndexes(ctx); err != nil {
		return indexMediaOptions{}, err
	}
	state, err := l.indexedListingPathState(ctx, rel, opts.IncludeAdminOnly, false)
	if err != nil {
		return indexMediaOptions{}, err
	}
	if !state.Covered {
		return indexMediaOptions{}, ErrMapIndexUnavailable
	}
	if !opts.IncludeAdminOnly && !opts.mapVisibilityChecked {
		if err := l.refreshMapVisibility(ctx, rel); err != nil {
			return indexMediaOptions{}, err
		}
	}
	if queryHasPerson(opts.Query) {
		if err := l.refreshPeopleVisibility(ctx, ""); err != nil {
			return indexMediaOptions{}, err
		}
	}
	return indexMediaOptions{Directory: rel, Subtree: true, Query: opts.Query,
		Plan: indexQueryPlanFor(opts.Query), MediaType: opts.MediaType, GPSOnly: true,
		IncludeAdminOnly: opts.IncludeAdminOnly, LeanMetadata: true, Limit: indexPostFilterCandidateMax, MapBounds: &viewport}, nil
}

// MapMedia uses the same spatial and search predicates, returning bounded pages
// for dense locations that cannot be separated by further zooming.
func (l *Library) MapMedia(ctx context.Context, opts ListOptions, viewport MapBounds) ([]Media, int, error) {
	query, err := l.mapQuery(ctx, opts, viewport)
	if err != nil {
		return nil, 0, err
	}
	page := opts.Page
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	query.Limit = 96
	query.Offset = (page - 1) * 96
	query.RequestSort = "descending_date"
	return l.indexMedia(ctx, query)
}

func mapViewportWhere(b MapBounds) (string, []any) {
	where := "mi.latitude BETWEEN ? AND ? AND "
	args := []any{b.South, b.North, b.West, b.East}
	if b.West > b.East {
		where += "(mi.longitude >= ? OR mi.longitude <= ?) AND mi.longitude BETWEEN -180 AND 180"
	} else {
		where += "mi.longitude BETWEEN ? AND ?"
	}
	return where, args
}
