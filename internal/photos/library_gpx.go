// Datei importiert GPX-Tracks und verknuepft GPS-Daten mit passenden Fotozeitpunkten.
package photos

import (
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
const gpxTrackCacheMaxEntries = 4096

var (
	gpxDateYMDPattern = regexp.MustCompile(`(?:^|[^0-9])(\d{4})[-_. ]?(\d{2})[-_. ]?(\d{2})(?:[^0-9]|$)`)
	gpxDateDMYPattern = regexp.MustCompile(`(?:^|[^0-9])(\d{2})[-_. ](\d{2})[-_. ](\d{4})(?:[^0-9]|$)`)
)

type cachedGPXTrack struct {
	modTimeUnixNano int64
	sizeBytes       int64
	track           GPXTrack
}

func (l *Library) gpxFromPath(rel string) (GPXTrack, error) {
	abs, err := l.Resolve(rel)
	if err != nil {
		return GPXTrack{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		l.gpxInvalidateCache(rel)
		return GPXTrack{}, err
	}
	return l.gpxFromResolvedPath(rel, abs, info)
}

func (l *Library) gpxFromPathInfo(rel string, info os.FileInfo) (GPXTrack, error) {
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
	return l.gpxFromResolvedPath(rel, abs, info)
}

func (l *Library) gpxFromResolvedPath(rel, abs string, info os.FileInfo) (GPXTrack, error) {
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

	decoder := xml.NewDecoder(file)
	name := filepath.Base(filepath.FromSlash(rel))
	track := GPXTrack{Name: name, Path: rel, Label: gpxTrackLabel(name)}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return GPXTrack{}, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || (start.Name.Local != "trkpt" && start.Name.Local != "rtept") {
			continue
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
			track.Points = appendGPXPoint(track.Points, point)
		}
	}
	l.gpxStoreCache(rel, info, track)
	return track, nil
}

func (l *Library) gpxFromCache(rel string, info os.FileInfo) (GPXTrack, bool) {
	if l == nil || info == nil {
		return GPXTrack{}, false
	}
	modUnix := info.ModTime().UnixNano()
	sizeBytes := info.Size()
	l.gpxMu.RLock()
	cached, ok := l.gpxCache[rel]
	l.gpxMu.RUnlock()
	if !ok || cached.modTimeUnixNano != modUnix || cached.sizeBytes != sizeBytes {
		return GPXTrack{}, false
	}
	return cached.track, true
}

func (l *Library) gpxStoreCache(rel string, info os.FileInfo, track GPXTrack) {
	if l == nil || info == nil {
		return
	}
	l.gpxMu.Lock()
	if l.gpxCache == nil {
		l.gpxCache = map[string]cachedGPXTrack{}
	}
	l.gpxCache[rel] = cachedGPXTrack{
		modTimeUnixNano: info.ModTime().UnixNano(),
		sizeBytes:       info.Size(),
		track:           track,
	}
	if len(l.gpxCache) > gpxTrackCacheMaxEntries {
		clear(l.gpxCache)
	}
	l.gpxMu.Unlock()
}

func (l *Library) gpxInvalidateCache(rel string) {
	if l == nil {
		return
	}
	l.gpxMu.Lock()
	if l.gpxCache != nil {
		delete(l.gpxCache, rel)
	}
	l.gpxMu.Unlock()
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
