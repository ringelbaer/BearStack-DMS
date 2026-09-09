package photos

import (
	"context"
	"errors"
)

const browserRoutePointLimit = 8192

// The browser retains numbered places and their original times/counts. Sample
// only the rendered route, never the persistent source. Compaction shares the
// native/GPX reducer and bounds working memory independently of route length.
type browserRoutePoints struct {
	points []RoutePoint
	total  int
}

func routePointCoordinate(p RoutePoint) GPXPoint { return GPXPoint{p.Lat, p.Lon} }

func (b *browserRoutePoints) add(c routeCluster) error {
	b.total++
	b.points = append(b.points, RoutePoint{Order: b.total, Lat: c.lat, Lon: c.lon,
		StartedAt: c.started, EndedAt: c.ended, Count: c.count})
	if len(b.points) > 2*browserRoutePointLimit {
		b.points = sampleMapValues(b.points, browserRoutePointLimit, routePointCoordinate)
	}
	return nil
}

func (b *browserRoutePoints) finish(listing *Listing) {
	listing.RoutePoints = sampleMapValues(b.points, browserRoutePointLimit, routePointCoordinate)
	listing.RouteTotalPoints = b.total
}

func (l *Library) populateListingRoute(ctx context.Context, opts ListOptions, listing *Listing) error {
	// The browser map is recursive. Explicit filesystem listings and callers
	// requesting only the direct directory preserve their existing selection.
	if opts.Recursive && !opts.FullFilesystem {
		query, err := l.mapQuery(ctx, opts, MapBounds{-90, -180, 90, 180})
		if err == nil {
			for attempt := 0; attempt < 3; attempt++ {
				var points browserRoutePoints
				err = l.consumePhotoRoute(ctx, query, NormalizeRouteClusterRadiusMeters(opts.RouteClusterRadiusMeters), points.add)
				if err == nil {
					points.finish(listing)
					return nil
				}
				if !errors.Is(err, errPhotoRouteChanged) && !errors.Is(err, errPhotoRouteCacheCorrupt) {
					return err
				}
			}
			return ErrMapIndexUnavailable
		}
		if !errors.Is(err, ErrMapIndexUnavailable) {
			return err
		}
	}
	// Before the initial index scan, keep the existing filesystem fallback.
	points := routePointsFromMedia(listing.Media, opts.RouteClusterRadiusMeters)
	listing.RouteTotalPoints = len(points)
	listing.RoutePoints = sampleMapValues(points, browserRoutePointLimit, routePointCoordinate)
	return ctx.Err()
}
