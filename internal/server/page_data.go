// Datei baut gemeinsame Seitendaten fuer Templates, Navigation und Benutzerkontext.
package server

import (
	"html/template"

	"bearstack/internal/document"
	"bearstack/internal/photos"
)

type PageData struct {
	People                  photos.PeoplePage
	FaceSettings            FaceSettingsView
	AppName                 string
	AppVersion              string
	Title                   string
	Active                  string
	Assets                  PageAssets
	Documents               []document.Document
	Document                document.Document
	DocumentReadOnlyView    bool
	RelatedDocuments        []RelatedDocument
	Duplicates              []document.DuplicateGroup
	AuditLogs               []document.AuditLogEntry
	MailImport              document.MailImportSettings
	MailPasswordSet         bool
	Statistics              document.Statistics
	Tag                     document.Tag
	Tags                    []document.Tag
	PhotoTags               []document.Tag
	TagTab                  string
	SearchFavorites         []SearchFavoriteView
	SearchFavoriteDates     []SearchFavoriteDateOption
	TagDisplayMode          string
	TagDisplayOptions       []TagDisplayOption
	HomePage                string
	HomeURL                 string
	HomePageOptions         []HomePageOption
	DocumentCloudEnabled    bool
	ThemeMode               string
	ThemeOptions            []ThemeOption
	TrashRetentionDays      int
	TrashRetentionOptions   []TrashRetentionOption
	PhotoSettings           PhotoSettings
	TagCloud                TagCloudView
	FolderTags              []FolderTag
	SelectedTags            []string
	FolderBreadcrumb        []FolderCrumb
	HideFolderPanel         bool
	ShowFolderDocuments     bool
	TagRules                []document.TagRule
	TagDescriptions         map[string]string
	TagStyles               map[string]template.CSS
	TagListHidden           map[string]bool
	CustomFields            []document.CustomField
	CustomField             document.CustomField
	CustomFieldValues       []document.CustomFieldValue
	CustomFieldSuggestions  []document.CustomFieldValueSuggestion
	DocumentOCRJobs         map[int64]*document.OCRJob
	OCRJob                  *document.OCRJob
	VisibleColumns          map[string]bool
	DocumentColumns         []DocumentColumn
	ColumnOptions           []DocumentColumn
	DesktopDateUnderTitle   bool
	FolderTagMinDocuments   int
	Filter                  document.ListFilter
	DocumentFilterActive    bool
	FilterDates             filterDates
	DateYears               []int
	DateYearLinks           []DateLink
	DateOverflowYears       []DateLink
	DateMonthLinks          []DateLink
	DateResetURL            string
	SettingsTab             string
	DesktopPreviewMode      string
	InlineDesktopPreview    bool
	CustomPDFPreviewEnabled bool
	HighlightID             int64
	Notice                  string
	Error                   string
	MaxUploadMB             int64
	ReturnURL               string
	CurrentURL              string
	Pagination              PaginationData
	SortLinks               map[string]SortLink
	PhotoModuleEnabled      bool
	Photos                  PhotoListingView
	PhotoMediaGroups        []PhotoMediaGroup
	PhotoFilter             PhotoFilter
	PhotoFrame              bool
	PhotoPage               bool
	PhotoIndexTelemetry     photos.IndexTelemetry
	PhotoListTrace          photos.ListTraceSnapshot
	PhotoStatistics         photos.Statistics
	WebDAVPath              string
	Auth                    AuthPermissions
	CustomFavicon           CustomFaviconView
	UserManagement          UserManagementView
	Account                 AccountView
}

type PageAssets struct {
	Explicit   bool
	Documents  bool
	OCR        bool
	Statistics bool
	Photos     bool
	Tags       bool
}

type RelatedDocument struct {
	document.Document
	IsLinked  bool
	IsGrouped bool
	GroupTags []string
}

type FolderTag struct {
	document.Tag
	URL        string
	Label      string
	SubLabel   string
	Kind       string
	CountLabel string
	Redundant  bool
}

type FolderCrumb struct {
	Label   string
	URL     string
	Current bool
	IsTag   bool
}

type SearchFavoriteView struct {
	document.SearchFavorite
	URL       string
	DateLabel string
	Summary   string
}

type SearchFavoriteDateOption struct {
	Value string
	Label string
}

type TagDisplayOption struct {
	Value string
	Label string
}

type ThemeOption struct {
	Value       string
	Label       string
	Description string
}

type HomePageOption struct {
	Value string
	Label string
	URL   string
}

type TagCloudView struct {
	HasPrimaryTags bool
	Items          []TagCloudItemView
	Clusters       []TagCloudClusterView
	Empty          bool
}

type TagCloudClusterView struct {
	Primary TagCloudItemView
	Items   []TagCloudItemView
}

type TagCloudItemView struct {
	Name       string
	URL        string
	Count      int
	Primary    bool
	SizeRem    float64
	CloudStyle template.CSS
}

type TrashRetentionOption struct {
	Value int
	Label string
}

type CustomFaviconView struct {
	Uploaded  bool
	Href      string
	Type      string
	Filename  string
	SizeBytes int64
}

type PaginationData struct {
	Page              int
	PrevURL           string
	NextURL           string
	Start             int
	End               int
	Total             int
	PerPage           int
	PerPageOptions    []int
	PageSizeReturnURL string
	DocumentList      bool
}

type SortLink struct {
	URL       string
	Active    bool
	Direction string
	AriaSort  string
}

type DateLink struct {
	Label  string
	URL    string
	Active bool
}

func (d PageData) DisplayAppName() string {
	if d.AppName == "" {
		return defaultAppName
	}
	return d.AppName
}

func (d PageData) SystemMenuVisible() bool {
	return d.Auth.Authenticated
}

func (d PageData) SystemMenuPrimaryVisible() bool {
	return d.Auth.CanDocumentsRead ||
		d.Auth.CanPhotosRead ||
		d.Auth.CanPhotosManage ||
		d.Auth.CanDocumentsStructure ||
		d.Auth.CanDocumentsDelete
}

func (d PageData) SystemMenuSecondaryVisible() bool {
	return d.Auth.CanDocumentsRead ||
		d.Auth.CanSystemManage ||
		d.Auth.CanSystemUsersManage ||
		d.Auth.CanPhotosManage ||
		d.Auth.CanSystemAudit
}

func (d PageData) SystemMenuAPILinkVisible() bool {
	return d.Auth.CanDocumentsRead ||
		d.Auth.CanDocumentsUpload ||
		d.Auth.CanDocumentsWebDAV ||
		d.Auth.CanPhotosRead
}

type PhotoFilter struct {
	Path                   string
	Query                  string
	MediaType              string
	GPSOnly                bool
	Sort                   string
	SortLabel              string
	SortOptions            []PhotoSortOption
	AdminOnlyToggleVisible bool
	ShowAdminOnly          bool
	MapView                bool
	MapAvailable           bool
	RandomURL              string
	FrameURL               string
	ClearURL               string
	PrevURL                string
	NextURL                string
	PageLinks              []PhotoPageLink
	ParentURL              string
	MediaTypeAllURL        string
	MediaTypeImageURL      string
	MediaTypeVideoURL      string
	MediaTypeAudioURL      string
	GPSURL                 string
	MapURL                 string
	GalleryURL             string
}

type PhotoPageLink struct {
	Page     int
	URL      string
	Current  bool
	Ellipsis bool
}

type PhotoSortOption struct {
	Value  string
	Label  string
	URL    string
	Active bool
}

type DocumentColumn struct {
	Key      string
	Label    string
	SortKey  string
	IsCustom bool
	FieldID  int64
}

type filterDates struct {
	From string
	To   string
}
