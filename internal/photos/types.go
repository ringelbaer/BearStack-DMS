// Datei definiert zentrale Foto-, Medien-, Ordner-, Blog- und Statistiktypen.
package photos

import (
	"html/template"
	"sync"
	"time"
)

const (
	MediaTypeImage = "image"
	MediaTypeVideo = "video"
	MediaTypeAudio = "audio"
	MediaTypeBlog  = "blog"

	MinFolderPreviewCount = 1
	MaxFolderPreviewCount = 4
)

func NormalizeFolderPreviewCount(value int) int {
	if value < MinFolderPreviewCount {
		return MinFolderPreviewCount
	}
	if value > MaxFolderPreviewCount {
		return MaxFolderPreviewCount
	}
	return value
}

type Library struct {
	faceImageGate chan struct{}
	faceRuntime   faceRuntime
	root          string
	cacheDir      string
	dbPath        string
	index         *photoIndexStore
	pageSize      int
	thumbnail     thumbnailRuntime
	gpxMu         sync.RWMutex
	gpxCache      map[string]cachedGPXTrack
	statsMu       sync.Mutex
	statsCache    thumbnailCacheStatsEntry
	telemetryMu   sync.RWMutex
	telemetry     IndexTelemetry
}

type IndexStats struct {
	Media   int
	Folders int
	Blogs   int
}

type IndexTelemetry struct {
	Running        bool
	StartedAt      time.Time
	FinishedAt     time.Time
	Duration       time.Duration
	ScannedFolders int
	SkippedFolders int
	Files          int
	FilesPerSecond float64
	DBWrites       int
	Stats          IndexStats
	LastErrors     []string
}

type IndexOptions struct {
	EntryDelay  time.Duration
	LowPriority bool
}

type ListOptions struct {
	Path                     string
	Query                    string
	MediaType                string
	GPSOnly                  bool
	Sort                     string
	Page                     int
	PageSize                 int
	Recursive                bool
	LeanMetadata             bool
	IncludeMapData           bool
	IncludeAdminOnly         bool
	FullFilesystem           bool
	RouteClusterRadiusMeters int
	FolderPreviewSize        int
}

type Listing struct {
	Path        string
	ParentPath  string
	Breadcrumbs []Crumb
	Folders     []Folder
	Media       []Media
	Blogs       []BlogPost
	GPXTracks   []GPXTrack
	RoutePoints []RoutePoint
	Query       string
	MediaType   string
	GPSOnly     bool
	Sort        string
	Order       string
	Page        int
	PageSize    int
	Total       int
	HasPrev     bool
	HasNext     bool
}

type Tag struct {
	Name  string
	Color string
	Count int
}

type Crumb struct {
	Name        string
	Path        string
	DisplayName string
	DisplayDate *time.Time
}

type Folder struct {
	Name                  string
	Path                  string
	DisplayName           string
	DisplayDate           *time.Time
	Tags                  []string
	AdminOnly             bool
	MediaCount            int
	DirectMediaCount      int
	MediaCountApproximate bool
	DirCount              int
	ModTime               time.Time
	Previews              []Media
	previewScanned        bool
}

type Media struct {
	Name           string
	Path           string
	Directory      string
	Type           string
	MIMEType       string
	SizeBytes      int64
	ModTime        time.Time
	CapturedAt     *time.Time
	Width          int
	Height         int
	Orientation    string
	Camera         string
	Lens           string
	Rating         *float64
	Latitude       *float64
	Longitude      *float64
	Keywords       []string
	Tags           []string
	AutomaticFaces []RecognizedFace
	Faces          []Face
	XMPFingerprint string
	AdminOnly      bool
}

type Face struct {
	Name   string
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type BlogPost struct {
	Name      string
	Path      string
	Tags      []string
	AdminOnly bool
	Date      *time.Time
	Text      string
	HTML      template.HTML
	ModTime   time.Time
}

type GPXTrack struct {
	Name   string
	Path   string
	Label  string
	Color  string
	Points []GPXPoint
}

type GPXPoint struct {
	Lat float64
	Lon float64
}

type RoutePoint struct {
	Order     int
	Lat       float64
	Lon       float64
	StartedAt time.Time
	EndedAt   time.Time
	Count     int
}

type Metadata struct {
	CapturedAt  *time.Time
	Width       int
	Height      int
	Orientation int
	Camera      string
	Lens        string
	Rating      *float64
	Latitude    *float64
	Longitude   *float64
	Keywords    []string
	Faces       []Face
}
