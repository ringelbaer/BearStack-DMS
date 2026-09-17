package photos

import "errors"

var ErrLabelConflict = errors.New("Personengruppe wurde geändert; bitte erneut prüfen")
var ErrLabelInvalid = errors.New("ungültige Benennungsaktion")
var ErrLabelNameExists = errors.New("Name bereits vorhanden")

type LabelSession struct {
	NamedFaceBatch   bool   `json:"named_face_batch"`
	MergeSideActions bool   `json:"merge_side_actions"`
	ManualMerge      bool   `json:"manual_merge"`
	MergeNaming      bool   `json:"merge_naming"`
	MergeSuggestions bool   `json:"merge_suggestions"`
	NamedSearch      bool   `json:"named_search"`
	NamedPeople      bool   `json:"named_people"`
	FaceFavorites    bool   `json:"face_favorites"`
	Protocol         int    `json:"protocol"`
	Instance         string `json:"instance"`
	Dataset          string `json:"dataset"`
	UpperID          int64  `json:"upper_id"`
}
type LabelPerson struct {
	ID       int64       `json:"id"`
	Name     string      `json:"name"`
	Revision int64       `json:"revision"`
	Count    int64       `json:"count"`
	FaceID   int64       `json:"face_id"`
	Faces    []LabelFace `json:"faces,omitempty"`
	Offset   int         `json:"offset"`
}
type LabelFace struct {
	NeedsReview    bool            `json:"needs_review"`
	SourceRevision int64           `json:"source_revision"`
	OriginalKey    string          `json:"original_key"`
	Favorite       bool            `json:"favorite"`
	ID             int64           `json:"id"`
	DisplayPath    string          `json:"display_path"`
	Bounds         LabelFaceBounds `json:"bounds"`
}

// LabelFaceBounds uses normalized coordinates in the EXIF-oriented original image.
type LabelFaceBounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type LabelCandidates struct {
	People  []LabelPerson `json:"people"`
	Next    int64         `json:"next"`
	HasNext bool          `json:"has_next"`
}
type LabelAction struct {
	FaceIDs        []int64         `json:"face_ids,omitempty"`
	Groups         []LabelGroupRef `json:"groups,omitempty"`
	AssignID       int64           `json:"assign_id,omitempty"`
	AssignRevision int64           `json:"assign_revision,omitempty"`
	SuggestionID   int64           `json:"suggestion_id,omitempty"`
	Favorite       *bool           `json:"favorite,omitempty"`
	OperationID    string          `json:"operation_id"`
	Dataset        string          `json:"dataset"`
	Revision       int64           `json:"revision"`
	Action         string          `json:"action"`
	Name           string          `json:"name,omitempty"`
	AllowDuplicate bool            `json:"allow_duplicate,omitempty"`
	TargetID       int64           `json:"target_id,omitempty"`
	TargetRevision int64           `json:"target_revision,omitempty"`
	FaceID         int64           `json:"face_id,omitempty"`
}
type LabelReceipt struct {
	SourceRevision int64  `json:"source_revision"`
	OperationID    string `json:"operation_id"`
	Action         string `json:"action"`
	SourceID       int64  `json:"source_id"`
	TargetID       int64  `json:"target_id"`
	NewID          int64  `json:"new_id"`
	Faces          int64  `json:"faces"`
	Groups         int    `json:"groups"`
	At             int64  `json:"at"`
}
