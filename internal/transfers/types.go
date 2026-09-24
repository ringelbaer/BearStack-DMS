// Package transfers owns outbound, create-only transfers independently of source libraries.
package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type ErrorKind string

const (
	Authentication ErrorKind = "authentication"
	Permission     ErrorKind = "permission"
	Conflict       ErrorKind = "conflict"
	Quota          ErrorKind = "quota"
	Throttled      ErrorKind = "throttled"
	Temporary      ErrorKind = "temporary"
	Unsupported    ErrorKind = "unsupported"
	Invalid        ErrorKind = "invalid"
)

type Error struct {
	Kind       ErrorKind
	Message    string
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Message }
func Kind(err error) ErrorKind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return Temporary
}
func Fail(k ErrorKind, message string) error { return &Error{Kind: k, Message: message} }

type ConfigField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
}
type ProviderInfo struct {
	Fields      []ConfigField `json:"fields"`
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	UploadLabel string        `json:"upload_label"`
}
type Connection struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Provider  string          `json:"provider"`
	Enabled   bool            `json:"enabled"`
	Revision  int64           `json:"revision"`
	Config    json.RawMessage `json:"config"`
	Base      Location        `json:"base"`
	Account   string          `json:"account"`
	Connected bool            `json:"connected"`
}
type Location struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Login struct {
	URL     string
	State   json.RawMessage
	Expires time.Time
}
type Credentials struct {
	Account string
	Secret  json.RawMessage
}
type Entry struct {
	Segments  []string `json:"segments"`
	Size      int64    `json:"size"`
	Directory bool     `json:"directory"`
}
type File interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}
type ResumeState struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
}

// ResumeState belongs to the adapter. The core only persists it with the file.
type Upload struct {
	Resume     ResumeState
	SaveResume func(ResumeState) error
	Segments   []string
	Size       int64
	Modified   time.Time
	Body       File
	Progress   func(int64)
}

// Client exposes no delete, overwrite or general-purpose move operation.
// Inventory streams the direct children of one relative directory.
// Stat returns directories for logical prefixes too: nil guarantees that no
// descendant existed when checked. Clients must support concurrent calls.
type Client interface {
	Locations(context.Context, string) ([]Location, error)
	Inventory(context.Context, Location, []string, func(Entry) error) error
	Stat(context.Context, Location, []string) (*Entry, error)
	EnsureDirectories(context.Context, Location, []string) error
	CreateFile(context.Context, Location, Upload) error
}
type Provider interface {
	Info() ProviderInfo
	Validate(json.RawMessage) error
	BeginLogin(context.Context, json.RawMessage) (Login, error)
	PollLogin(context.Context, json.RawMessage, json.RawMessage) (*Credentials, error)
	Connect(context.Context, json.RawMessage, json.RawMessage) (Client, error)
}
type Registry map[string]Provider

type Selection struct {
	Paths            []string `json:"paths,omitempty"`
	Path             string   `json:"path"`
	Query            string   `json:"query"`
	MediaType        string   `json:"media_type"`
	GPSOnly          bool     `json:"gps_only"`
	IncludeAdminOnly bool     `json:"include_admin_only"`
}
type SelectionInfo struct {
	Name    string `json:"name"`
	Target  string `json:"target"`
	Virtual bool   `json:"virtual"`
}
type SourceItem struct {
	Path        string   `json:"path"`
	DisplayPath string   `json:"display_path"`
	Relative    []string `json:"relative"`
	Size        int64    `json:"size"`
	Modified    int64    `json:"modified"`
}
type Source interface {
	Describe(context.Context, Selection) (SelectionInfo, error)
	Walk(context.Context, Selection, func(SourceItem) error) error
	Open(context.Context, Selection, SourceItem) (File, error)
}

type Job struct {
	TargetExists   bool      `json:"target_exists"`
	Submitted      bool      `json:"submitted"`
	ID             string    `json:"id"`
	ConnectionID   string    `json:"connection_id"`
	ConnectionName string    `json:"connection_name"`
	Provider       string    `json:"provider"`
	Revision       int64     `json:"revision"`
	Actor          string    `json:"-"`
	Selection      Selection `json:"selection"`
	SourceName     string    `json:"source_name"`
	Target         string    `json:"target"`
	Base           Location  `json:"base"`
	State          string    `json:"state"`
	Error          string    `json:"error"`
	Created        int64     `json:"created"`
	Expires        int64     `json:"expires"`
	Updated        int64     `json:"updated"`
	Attempts       int       `json:"attempts"`
	RetryAt        int64     `json:"retry_at"`
	Total          int64     `json:"total"`
	Bytes          int64     `json:"bytes"`
	Missing        int64     `json:"missing"`
	MissingBytes   int64     `json:"missing_bytes"`
	Existing       int64     `json:"existing"`
	Conflicts      int64     `json:"conflicts"`
	Done           int64     `json:"done"`
	Failed         int64     `json:"failed"`
	UploadedBytes  int64     `json:"uploaded_bytes"`
	InFlightBytes  int64     `json:"in_flight_bytes"`
}
type Item struct {
	ID int64 `json:"id"`
	SourceItem
	State  string      `json:"state"`
	Error  string      `json:"error"`
	Resume ResumeState `json:"-"`
}

func ValidateSegments(segments []string) error {
	if len(segments) == 0 || len(segments) > 256 {
		return Fail(Invalid, "Ungültiger Zielpfad")
	}
	for _, s := range segments {
		if s == "" || s == "." || s == ".." || len(s) > 255 || !utf8.ValidString(s) || strings.IndexFunc(s, unicode.IsControl) >= 0 || strings.ContainsAny(s, "/\\\x00\r\n") {
			return Fail(Invalid, "Ungültiger Zielname")
		}
	}
	return nil
}
