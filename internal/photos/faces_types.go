// Face recognition data types; internal vectors are never part of public views.
package photos

import (
	"sync"

	"github.com/coder/hnsw"
)

type RecognizedFace struct {
	nameSource string
	ID         int64   `json:"id"`
	PersonID   int64   `json:"person_id"`
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Manual     bool    `json:"manual"`
	Ignored    bool    `json:"ignored"`
}
type Person struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
	FaceID int64  `json:"face_id"`
}
type PeoplePage struct {
	TotalPages  int              `json:"total_pages"`
	IgnoredOnly bool             `json:"ignored_only"`
	KnownOnly   bool             `json:"known_only"`
	Query       string           `json:"query,omitempty"`
	People      []Person         `json:"people"`
	Faces       []RecognizedFace `json:"faces,omitempty"`
	PersonID    int64            `json:"person_id,omitempty"`
	Name        string           `json:"name,omitempty"`
	Page        int              `json:"page"`
	HasNext     bool             `json:"has_next"`
	HasPrev     bool             `json:"has_prev"`
}
type FaceJob struct {
	Path          string
	Size, ModTime int64
	XMP, Model    string
	Attempts      int
}
type FaceJobError struct {
	Path     string `json:"path"`
	Error    string `json:"error"`
	Attempts int    `json:"attempts"`
}
type FaceStatus struct {
	Errors []FaceJobError `json:"errors,omitempty"`
	Queued int            `json:"queued"`
	Done   int            `json:"done"`
	Failed int            `json:"failed"`
	Faces  int            `json:"faces"`
	People int            `json:"people"`
}
type faceRuntime struct {
	referenceLimit int
	mu             sync.Mutex
	graph          *hnsw.Graph[int64]
	revision       int64
	people         map[int64]int64
	nodes          map[int64][]int64
	model          string
}

func (p *PeoplePage) setTotal(total int) {
	p.TotalPages = max(1, (total+59)/60)
	p.Page = min(max(1, p.Page), p.TotalPages)
	p.HasPrev = p.Page > 1
	p.HasNext = p.Page < p.TotalPages
}
