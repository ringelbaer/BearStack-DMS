// Datei importiert GPX-Tracks und verknuepft GPS-Daten mit passenden Fotozeitpunkten.
package photos

import (
	"container/list"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const gpxMinPointDistanceMeters = 5
const (
	gpxTrackCacheMaxEntries       = 4096
	gpxMaxBytes             int64 = 16 << 20
	gpxMaxPoints                  = 100_000
	gpxCacheMaxBytes        int64 = 32 << 20
	gpxListingMaxPoints           = 250_000
	gpxListingMaxTracks           = 256
)

var errGPXLimit = errors.New("GPX überschreitet das Byte- oder Punktlimit")

var (
	gpxDateYMDPattern = regexp.MustCompile(`(?:^|[^0-9])(\d{4})[-_. ]?(\d{2})[-_. ]?(\d{2})(?:[^0-9]|$)`)
	gpxDateDMYPattern = regexp.MustCompile(`(?:^|[^0-9])(\d{2})[-_. ](\d{2})[-_. ](\d{4})(?:[^0-9]|$)`)
)

type cachedGPXTrack struct {
	modTimeUnixNano int64
	sizeBytes       int64
	track           GPXTrack
	cost            int64
	element         *list.Element
}

func (l *Library) gpxFromPathInfo(ctx context.Context, rel string, info os.FileInfo) (GPXTrack, error) {
	abs, err := l.Resolve(rel)
	if err != nil {
		return GPXTrack{}, err
	}
	if info == nil {
		info, err = os.Stat(abs)
		if err != nil {
			l.gpxInvalidateCache(rel)
			return GPXTrack{}, err
		}
	}
	return l.gpxFromResolvedPath(ctx, rel, abs, info)
}

func (l *Library) gpxFromResolvedPath(ctx context.Context, rel, abs string, info os.FileInfo) (GPXTrack, error) {
	if err := ctx.Err(); err != nil {
		return GPXTrack{}, err
	}
	if info.Size() > gpxMaxBytes {
		l.gpxInvalidateCache(rel)
		return GPXTrack{}, errGPXLimit
	}
	if cached, ok := l.gpxFromCache(rel, info); ok {
		return cached, nil
	}
	// Bound concurrent parser allocations and let waiting requests cancel.
	l.gpxMu.Lock()
	if l.gpxParseGate == nil {
		l.gpxParseGate = make(chan struct{}, 1)
	}
	gate := l.gpxParseGate
	l.gpxMu.Unlock()
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return GPXTrack{}, ctx.Err()
	}
	if cached, ok := l.gpxFromCache(rel, info); ok {
		return cached, nil
	}
	file, err := os.Open(abs)
	if err != nil {
		return GPXTrack{}, err
	}
	defer file.Close()
	if fileInfo, statErr := file.Stat(); statErr == nil {
		info = fileInfo
		if cached, ok := l.gpxFromCache(rel, info); ok {
			return cached, nil
		}
	}

	points, err := decodeGPX(ctx, file, gpxMaxBytes, gpxMaxPoints)
	if err != nil {
		return GPXTrack{}, err
	}
	name := filepath.Base(filepath.FromSlash(rel))
	track := GPXTrack{Name: name, Path: rel, Label: gpxTrackLabel(name), Points: points}
	l.gpxStoreCache(rel, info, track)
	return track, nil
}

type gpxContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r gpxContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func decodeGPX(ctx context.Context, input io.Reader, maxBytes int64, maxPoints int) ([]GPXPoint, error) {
	limited := &io.LimitedReader{R: input, N: maxBytes + 1}
	decoder := xml.NewDecoder(gpxContextReader{ctx, limited})
	var points []GPXPoint
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if limited.N == 0 {
			return nil, errGPXLimit
		}
		if errors.Is(err, io.EOF) {
			return points, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || (start.Name.Local != "trkpt" && start.Name.Local != "rtept") {
			continue
		}
		count++
		if count > maxPoints {
			return nil, errGPXLimit
		}
		var point GPXPoint
		var hasLat, hasLon bool
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "lat":
				point.Lat, hasLat = parseGPXCoord(attr.Value)
			case "lon":
				point.Lon, hasLon = parseGPXCoord(attr.Value)
			}
		}
		if hasLat && hasLon && validGPXPoint(point) {
			points = appendGPXPoint(points, point)
		}
	}
}

func (l *Library) gpxFromCache(rel string, info os.FileInfo) (GPXTrack, bool) {
	if l == nil || info == nil {
		return GPXTrack{}, false
	}
	l.gpxMu.Lock()
	defer l.gpxMu.Unlock()
	cached, ok := l.gpxCache[rel]
	if !ok {
		return GPXTrack{}, false
	}
	if cached.modTimeUnixNano != info.ModTime().UnixNano() || cached.sizeBytes != info.Size() {
		l.removeGPXCacheEntry(rel)
		return GPXTrack{}, false
	}
	l.gpxLRU.MoveToFront(cached.element)
	return cached.track, true
}

func (l *Library) gpxStoreCache(rel string, info os.FileInfo, track GPXTrack) {
	l.gpxStoreCacheBudget(rel, info, track, gpxCacheMaxBytes)
}

func (l *Library) gpxStoreCacheBudget(rel string, info os.FileInfo, track GPXTrack, budget int64) {
	if l == nil || info == nil {
		return
	}
	// Include backing-array capacity, strings and conservative per-entry overhead.
	cost := int64(cap(track.Points))*16 + int64(len(rel)+len(track.Name)+len(track.Path)+len(track.Label)+len(track.Color)) + 256
	l.gpxMu.Lock()
	defer l.gpxMu.Unlock()
	l.removeGPXCacheEntry(rel)
	if cost > budget {
		return
	}
	if l.gpxCache == nil {
		l.gpxCache = map[string]cachedGPXTrack{}
	}
	for l.gpxCacheBytes+cost > budget || len(l.gpxCache) >= gpxTrackCacheMaxEntries {
		oldest := l.gpxLRU.Back()
		if oldest == nil {
			break
		}
		l.removeGPXCacheEntry(oldest.Value.(string))
	}
	l.gpxCache[rel] = cachedGPXTrack{modTimeUnixNano: info.ModTime().UnixNano(), sizeBytes: info.Size(), track: track, cost: cost, element: l.gpxLRU.PushFront(rel)}
	l.gpxCacheBytes += cost
}

// Caller holds gpxMu.
func (l *Library) removeGPXCacheEntry(rel string) {
	if cached, ok := l.gpxCache[rel]; ok {
		l.gpxCacheBytes -= cached.cost
		l.gpxLRU.Remove(cached.element)
		delete(l.gpxCache, rel)
	}
}

func (l *Library) gpxInvalidateCache(rel string) {
	if l == nil {
		return
	}
	l.gpxMu.Lock()
	defer l.gpxMu.Unlock()
	l.removeGPXCacheEntry(rel)
}

func parseGPXCoord(value string) (float64, bool) {
	coord, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return coord, err == nil
}

func validGPXPoint(point GPXPoint) bool {
	return point.Lat >= -90 && point.Lat <= 90 && point.Lon >= -180 && point.Lon <= 180
}

func appendGPXPoint(points []GPXPoint, point GPXPoint) []GPXPoint {
	if len(points) > 0 {
		last := points[len(points)-1]
		if routeDistanceMeters(last.Lat, last.Lon, point.Lat, point.Lon) < gpxMinPointDistanceMeters {
			return points
		}
	}
	return append(points, point)
}

func gpxTrackLabel(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if date, ok := gpxDateFromName(base); ok {
		return date.Format("02.01.2006")
	}
	return name
}

func gpxDateFromName(name string) (time.Time, bool) {
	if match := gpxDateYMDPattern.FindStringSubmatch(name); len(match) == 4 {
		return gpxDateFromParts(match[1], match[2], match[3])
	}
	if match := gpxDateDMYPattern.FindStringSubmatch(name); len(match) == 4 {
		return gpxDateFromParts(match[3], match[2], match[1])
	}
	return time.Time{}, false
}

func gpxDateFromParts(year, month, day string) (time.Time, bool) {
	date, err := time.Parse("2006-01-02", year+"-"+month+"-"+day)
	return date, err == nil
}

func decorateGPXTracks(tracks []GPXTrack) {
	for i := range tracks {
		if tracks[i].Label == "" {
			tracks[i].Label = gpxTrackLabel(tracks[i].Name)
		}
		tracks[i].Color = gpxTrackColor(i)
	}
}

func gpxTrackColor(index int) string {
	hue := math.Mod(float64(index)*137.508, 360)
	return hslToHex(hue, 0.72, 0.42)
}

func hslToHex(hue, saturation, lightness float64) string {
	c := (1 - math.Abs(2*lightness-1)) * saturation
	x := c * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	m := lightness - c/2
	var r, g, b float64
	switch {
	case hue < 60:
		r, g, b = c, x, 0
	case hue < 120:
		r, g, b = x, c, 0
	case hue < 180:
		r, g, b = 0, c, x
	case hue < 240:
		r, g, b = 0, x, c
	case hue < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return fmt.Sprintf("#%02x%02x%02x", colorByte(r+m), colorByte(g+m), colorByte(b+m))
}

func colorByte(value float64) int {
	return int(math.Round(clampFloat(value, 0, 1) * 255))
}

func clampFloat(value, min, max float64) float64 {
	return math.Max(min, math.Min(max, value))
}

// Map responses have a separate budget: evicting the cache cannot release
// tracks that are still referenced by a response under construction.
func (listing *Listing) addGPXTrack(track GPXTrack) {
	if len(track.Points) == 0 || len(listing.GPXTracks) >= gpxListingMaxTracks || listing.gpxPointCount+len(track.Points) > gpxListingMaxPoints {
		return
	}
	listing.GPXTracks = append(listing.GPXTracks, track)
	listing.gpxPointCount += len(track.Points)
}
