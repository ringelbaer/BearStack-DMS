// Datei definiert zentrale Dokumenttypen und Metadatenmodelle fuer die Dokumentverwaltung.
package document

import (
	"strings"
	"time"
)

const CurrentSearchVersion = 6

const (
	UploadWayWeb    = "web"
	UploadWayAPI    = "api"
	UploadWayMail   = "mail"
	UploadWayWebDAV = "webdav"
)

const (
	ContentTextSourcePDF     = "pdf"
	ContentTextSourceFile    = "file"
	ContentTextSourceRaw     = "raw"
	ContentTextSourceOCR     = "ocr"
	ContentTextSourceNone    = "none"
	ContentTextSourceUnknown = "unknown"
)

type Document struct {
	ID                int64
	OriginalName      string
	StoredPath        string
	ThumbnailPath     string
	UploadWay         string
	Title             string
	Description       string
	Tags              []string
	CustomValues      map[int64]string
	MIMEType          string
	SizeBytes         int64
	SHA256            string
	DocumentDate      *time.Time
	UploadedAt        time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
	ContentText       string
	ContentTextSource string
	SearchVersion     int
	DuplicateCount    int
	LinkedCount       int
	DeleteProtected   bool
}

func NormalizeUploadWay(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case UploadWayAPI:
		return UploadWayAPI
	case UploadWayMail:
		return UploadWayMail
	case UploadWayWebDAV:
		return UploadWayWebDAV
	default:
		return UploadWayWeb
	}
}

func (d Document) UploadWayLabel() string {
	switch NormalizeUploadWay(d.UploadWay) {
	case UploadWayAPI:
		return "API"
	case UploadWayMail:
		return "E-Mail"
	case UploadWayWebDAV:
		return "WebDAV"
	default:
		return "Web"
	}
}

func NormalizeContentTextSource(value, contentText string) string {
	if strings.TrimSpace(contentText) == "" {
		return ContentTextSourceNone
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ContentTextSourcePDF:
		return ContentTextSourcePDF
	case ContentTextSourceFile:
		return ContentTextSourceFile
	case ContentTextSourceRaw:
		return ContentTextSourceRaw
	case ContentTextSourceOCR:
		return ContentTextSourceOCR
	case ContentTextSourceUnknown:
		return ContentTextSourceUnknown
	default:
		return ContentTextSourceUnknown
	}
}

func ContentTextSourceLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ContentTextSourcePDF:
		return "PDF-Text extrahiert"
	case ContentTextSourceFile:
		return "Dateitext extrahiert"
	case ContentTextSourceRaw:
		return "Unsicherer Rohtext"
	case ContentTextSourceOCR:
		return "OCR-Text"
	case ContentTextSourceNone:
		return "Kein Text"
	default:
		return "Textquelle unbekannt"
	}
}

func (d Document) ContentTextSourceLabel() string {
	return ContentTextSourceLabel(d.ContentTextSource)
}

type Tag struct {
	ID              int64
	Name            string
	Description     string
	Color           string
	PrimaryTag      bool
	GroupMode       bool
	ListHidden      bool
	DeleteProtected bool
	Count           int
}

type TagCloud struct {
	HasPrimaryTags bool
	Items          []TagCloudItem
	Clusters       []TagCloudCluster
	MaxCount       int
}

type TagCloudCluster struct {
	Primary  TagCloudItem
	Items    []TagCloudItem
	MaxCount int
}

type TagCloudItem struct {
	Tag     string
	Count   int
	Primary bool
}

const (
	TagRuleScopeText     = "text"
	TagRuleScopeFilename = "filename"
	TagRuleScopeBoth     = "both"
	TagRuleMatchAny      = "any"
	TagRuleMatchAll      = "all"
)

type TagRule struct {
	ID        int64
	TagID     int64
	Label     string
	Scope     string
	MatchMode string
	Keywords  []string
	Excludes  []string
	Position  int
}

func (r TagRule) KeywordsText() string {
	return strings.Join(r.Keywords, "\n")
}

func (r TagRule) ExcludesText() string {
	return strings.Join(r.Excludes, "\n")
}

type CustomField struct {
	ID                      int64
	Label                   string
	Position                int
	AutocompleteEnabled     bool
	ValueFolderMinDocuments int
}

const (
	CustomFieldValueFolderNever  = 0
	CustomFieldValueFolderAlways = 1
)

func NormalizeCustomFieldValueFolderMinDocuments(value int) int {
	switch value {
	case CustomFieldValueFolderAlways, 5, 10, 20, 50:
		return value
	default:
		return CustomFieldValueFolderNever
	}
}

type CustomFieldValue struct {
	Value string
	Count int
}

type CustomFieldValueFolder struct {
	FieldID    int64
	FieldLabel string
	Value      string
	Count      int
}

type CustomFieldValueSuggestion struct {
	Value   string
	Similar []string
	Reason  string
}

const (
	SearchFavoriteDateNone        = ""
	SearchFavoriteDateYear        = "year"
	SearchFavoriteDateThisMonth   = "this_month"
	SearchFavoriteDateLastMonth   = "last_month"
	SearchFavoriteDateThisYear    = "this_year"
	SearchFavoriteDateLastYear    = "last_year"
	SearchFavoriteDateThisQuarter = "this_quarter"
	SearchFavoriteDateLastQuarter = "last_quarter"
	SearchFavoriteDateThisHalf    = "this_half"
	SearchFavoriteDateLastHalf    = "last_half"
	SearchFavoriteDateLast7Days   = "last_7_days"
	SearchFavoriteDateLast30Days  = "last_30_days"
	SearchFavoriteDateLast90Days  = "last_90_days"
	SearchFavoriteDateLast365Days = "last_365_days"
)

type SearchFavorite struct {
	ID           int64
	Name         string
	Query        string
	Tags         []string
	CustomFields []CustomFieldFilter
	DateMode     string
	DateYear     int
}

type CustomFieldFilter struct {
	FieldID int64
	Value   string
	Exact   bool
}

const (
	ListSortUploadDate = "upload_date"
	ListSortDate       = "date"
	ListSortName       = "name"
	ListSortTitle      = "title"
	ListSortSize       = "size"
	ListSortDeletedAt  = "deleted_at"
)

const (
	ListDirectionAscending  = "asc"
	ListDirectionDescending = "desc"
)

type ListFilter struct {
	Query        string
	Tags         []string
	CustomFields []CustomFieldFilter
	From         *time.Time
	To           *time.Time
	Year         int
	Month        int
	Sort         string
	Direction    string
	Trash        bool
	Limit        int
	Offset       int
	Page         int
}

func CleanCustomFieldFilterValue(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func NormalizeListSort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ListSortDate:
		return ListSortDate
	case ListSortName:
		return ListSortName
	case ListSortTitle:
		return ListSortTitle
	case ListSortSize:
		return ListSortSize
	case ListSortDeletedAt:
		return ListSortDeletedAt
	default:
		return ListSortUploadDate
	}
}

func DefaultListDirection(sort string) string {
	switch NormalizeListSort(sort) {
	case ListSortName, ListSortTitle:
		return ListDirectionAscending
	default:
		return ListDirectionDescending
	}
}

func NormalizeListDirection(value, sort string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ListDirectionAscending:
		return ListDirectionAscending
	case ListDirectionDescending:
		return ListDirectionDescending
	default:
		return DefaultListDirection(sort)
	}
}

func ToggleListDirection(value string) string {
	if value == ListDirectionAscending {
		return ListDirectionDescending
	}
	return ListDirectionAscending
}

const (
	OCRJobStatusQueued      = "queued"
	OCRJobStatusRunning     = "running"
	OCRJobStatusCompleted   = "completed"
	OCRJobStatusFailed      = "failed"
	OCRJobStatusInterrupted = "interrupted"
)

type OCRJob struct {
	ID            int64
	DocumentID    int64
	Language      string
	LanguageLabel string
	Status        string
	CurrentPage   int
	TotalPages    int
	TextLength    int
	Message       string
	Error         string
	CreatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
	UpdatedAt     time.Time
}

func (j OCRJob) Active() bool {
	return j.Status == OCRJobStatusQueued || j.Status == OCRJobStatusRunning
}

func (j OCRJob) Terminal() bool {
	return j.Status == OCRJobStatusCompleted || j.Status == OCRJobStatusFailed || j.Status == OCRJobStatusInterrupted
}

func (j OCRJob) StatusText() string {
	switch j.Status {
	case OCRJobStatusQueued:
		return "Wartet"
	case OCRJobStatusRunning:
		return "Läuft"
	case OCRJobStatusCompleted:
		return "Abgeschlossen"
	case OCRJobStatusFailed:
		return "Fehlgeschlagen"
	case OCRJobStatusInterrupted:
		return "Unterbrochen"
	default:
		return "Unbekannt"
	}
}

func (j OCRJob) HasProgress() bool {
	return j.TotalPages > 0
}

func (j OCRJob) ProgressPercent() int {
	if j.TotalPages <= 0 {
		return 0
	}
	percent := (j.CurrentPage * 100) / j.TotalPages
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}
	return percent
}

type DuplicateGroup struct {
	SHA256     string
	Count      int
	Documents  []Document
	TotalBytes int64
}

type AuditLogEntry struct {
	ID         int64
	OccurredAt time.Time
	Actor      string
	Method     string
	Path       string
	Route      string
	Action     string
	Target     string
	Status     int
	RemoteAddr string
	UserAgent  string
}

type Statistics struct {
	ActiveDocuments             int
	TrashDocuments              int
	TotalDocuments              int
	TotalBytes                  int64
	TrashBytes                  int64
	AverageBytes                int64
	UploadedLast30Days          int
	DocumentsWithOCRText        int
	OCRCoveragePercent          int
	DocumentsWithDocumentDate   int
	DocumentDateCoveragePercent int
	DuplicateGroups             int
	DuplicateDocuments          int
	TrashPercent                int
	MonthlyUploads              []StatisticBucket
	MonthlyUploadsMax           int
	DocumentDateYears           []StatisticBucket
	DocumentDateYearMax         int
	UploadWays                  []StatisticBucket
	UploadWayMax                int
	FileTypes                   []StatisticBucket
	FileTypeMax                 int
	TopTags                     []StatisticBucket
	TopTagMax                   int
	TagUsageYears               []TagUsageYear
	TagUsageTags                []string
	TagUsageYearMax             int
	OCRStatuses                 []StatisticBucket
	OCRStatusMax                int
	OCRAttentionJobs            []OCRJobStatistic
	ContentTextSources          []StatisticBucket
	ContentTextSourceMax        int
	TextIssueDocuments          []TextIssueDocument
	TextIssueDocumentCount      int
	Database                    DatabaseStatus
}

type DatabaseStatus struct {
	TargetSearchVersion            int
	MinSearchVersion               int
	MaxSearchVersion               int
	TotalDocuments                 int
	CurrentSearchVersionDocuments  int
	OutdatedSearchVersionDocuments int
	SearchIndexDocuments           int
	SearchIndexTrigram             bool
}

func (s DatabaseStatus) HasDocuments() bool {
	return s.TotalDocuments > 0
}

func (s DatabaseStatus) SearchIndexComplete() bool {
	return s.SearchIndexDocuments == s.TotalDocuments
}

func (s DatabaseStatus) UpToDate() bool {
	return s.OutdatedSearchVersionDocuments == 0 &&
		s.SearchIndexTrigram &&
		s.SearchIndexComplete()
}

type StatisticBucket struct {
	Key   string
	Label string
	Count int
	Bytes int64
}

type TagUsageYear struct {
	Year     string
	Total    int
	Segments []TagUsageSegment
}

type TagUsageSegment struct {
	Tag   string
	Count int
}

type OCRJobStatistic struct {
	OCRJob
	DocumentOriginalName string
	DocumentTitle        string
}

type TextIssueDocument struct {
	ID                int64
	OriginalName      string
	Title             string
	MIMEType          string
	ContentTextSource string
	UpdatedAt         time.Time
}

func (d TextIssueDocument) ContentTextSourceLabel() string {
	return ContentTextSourceLabel(d.ContentTextSource)
}

const (
	MailImportSecurityTLS      = "tls"
	MailImportSecuritySTARTTLS = "starttls"
	MailImportSecurityNone     = "none"
)

type MailImportSettings struct {
	Enabled             bool   `json:"enabled"`
	Host                string `json:"host"`
	Port                int    `json:"port"`
	Security            string `json:"security"`
	Username            string `json:"username"`
	Password            string `json:"password,omitempty"`
	Mailbox             string `json:"mailbox"`
	PollIntervalMinutes int    `json:"poll_interval_minutes"`
	AllowedSenders      string `json:"allowed_senders"`
}

func (d Document) IsDeleted() bool {
	return d.DeletedAt != nil
}

func (d Document) DisplayDate() *time.Time {
	if d.DocumentDate != nil {
		return d.DocumentDate
	}
	return &d.UploadedAt
}
