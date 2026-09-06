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
	Query    string           `json:"query,omitempty"`
	People   []Person         `json:"people"`
	Faces    []RecognizedFace `json:"faces,omitempty"`
	PersonID int64            `json:"person_id,omitempty"`
	Name     string           `json:"name,omitempty"`
	Page     int              `json:"page"`
	HasNext  bool             `json:"has_next"`
	HasPrev  bool             `json:"has_prev"`
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
	mu       sync.Mutex
	graph    *hnsw.Graph[int64]
	revision int64
	people   map[int64]int64
	nodes    map[int64][]int64
	model    string
}
