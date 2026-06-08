package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bearstack/internal/document"
)

func TestFilterFromRequestRequiresThreeSearchCharacters(t *testing.T) {
	shortReq := httptest.NewRequest(http.MethodGet, "/?q=ab", nil)
	if got := filterFromRequest(shortReq, false, defaultDocumentPageSize).Query; got != "" {
		t.Fatalf("short query = %q", got)
	}

	longReq := httptest.NewRequest(http.MethodGet, "/?q=abc", nil)
	if got := filterFromRequest(longReq, false, defaultDocumentPageSize).Query; got != "abc" {
		t.Fatalf("long query = %q", got)
	}
}

func TestFilterFromRequestSortDirection(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?sort=name&dir=desc", nil)
	got := filterFromRequest(req, false, defaultDocumentPageSize)

	if got.Sort != document.ListSortName || got.Direction != document.ListDirectionDescending {
		t.Fatalf("filter = %#v", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/?sort=unknown&dir=sideways", nil)
	got = filterFromRequest(req, false, defaultDocumentPageSize)
	if got.Sort != document.ListSortUploadDate || got.Direction != document.ListDirectionDescending {
		t.Fatalf("fallback filter = %#v", got)
	}
}

func TestFolderDocumentFilterDefaultsToDocumentDateDescending(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/folders?tags=steuer", nil)
	got := folderDocumentFilterFromRequest(req, defaultDocumentPageSize)

	if got.Sort != document.ListSortDate || got.Direction != document.ListDirectionDescending {
		t.Fatalf("folder default filter = %#v", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/folders?tags=steuer&sort=name&dir=asc", nil)
	got = folderDocumentFilterFromRequest(req, defaultDocumentPageSize)
	if got.Sort != document.ListSortName || got.Direction != document.ListDirectionAscending {
		t.Fatalf("folder explicit filter = %#v", got)
	}
}

func TestFilterFromRequestYearMonth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?year=2024&month=2", nil)
	got := filterFromRequest(req, false, defaultDocumentPageSize)

	if got.Year != 2024 || got.Month != 2 {
		t.Fatalf("filter date parts = year %d month %d", got.Year, got.Month)
	}
	if got.From == nil || got.To == nil {
		t.Fatalf("filter dates = from %v to %v", got.From, got.To)
	}
	if got.From.Format("2006-01-02") != "2024-02-01" || got.To.Format("2006-01-02") != "2024-02-29" {
		t.Fatalf("filter dates = from %s to %s", got.From.Format("2006-01-02"), got.To.Format("2006-01-02"))
	}
}

func TestFilterFromRequestCustomFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?field_9=+ACME+4711+&field_x=nope&field_4=&field_2=Beta", nil)
	got := filterFromRequest(req, false, defaultDocumentPageSize)

	if len(got.CustomFields) != 2 {
		t.Fatalf("custom field filters = %#v", got.CustomFields)
	}
	if got.CustomFields[0].FieldID != 2 || got.CustomFields[0].Value != "Beta" {
		t.Fatalf("first custom field filter = %#v", got.CustomFields[0])
	}
	if got.CustomFields[1].FieldID != 9 || got.CustomFields[1].Value != "ACME 4711" {
		t.Fatalf("second custom field filter = %#v", got.CustomFields[1])
	}
}

func TestDocumentSortLinksToggleDirectionAndKeepFilters(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?q=abc&tags=steuer&sort=name&dir=asc&page=4&notice=ok&highlight=9", nil)
	filter := filterFromRequest(req, false, defaultDocumentPageSize)
	links := documentSortLinks(req, filter)

	name := links[document.ListSortName]
	if !name.Active || name.Direction != document.ListDirectionAscending || name.AriaSort != "ascending" {
		t.Fatalf("name link = %#v", name)
	}
	if name.URL != "/?dir=desc&q=abc&sort=name&tags=steuer" {
		t.Fatalf("name URL = %q", name.URL)
	}

	upload := links[document.ListSortUploadDate]
	if upload.Active || upload.Direction != document.ListDirectionDescending || upload.AriaSort != "none" {
		t.Fatalf("upload link = %#v", upload)
	}
	if upload.URL != "/?dir=desc&q=abc&sort=upload_date&tags=steuer" {
		t.Fatalf("upload URL = %q", upload.URL)
	}
}

func TestSafeReturnURLRejectsExternalAndAmbiguousRedirects(t *testing.T) {
	tests := map[string]string{
		"/documents/1?tab=tags": "/documents/1?tab=tags",
		"":                      "/",
		"https://example.test":  "/",
		"//example.test/path":   "/",
		`/\example.test/path`:   "/",
		"/%5cexample.test/path": "/",
		"/%2fexample.test/path": "/",
	}
	for input, want := range tests {
		if got := safeReturnURL(input); got != want {
			t.Fatalf("safeReturnURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDocumentDateLinksKeepFiltersAndTrimDateState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?q=abc&tags=steuer&from=2024-01-01&to=2024-12-31&page=3&notice=ok&highlight=9", nil)
	filter := document.ListFilter{Year: 2021, Month: 5}

	visible, overflow := documentYearLinks(req, []int{2024, 2023, 2022, 2021}, filter)
	if len(visible) != 4 || len(overflow) != 0 {
		t.Fatalf("visible = %#v overflow = %#v", visible, overflow)
	}
	if visible[0].URL != "/?q=abc&tags=steuer&year=2024" {
		t.Fatalf("year URL = %q", visible[0].URL)
	}
	if !visible[3].Active || visible[3].Label != "2021" {
		t.Fatalf("visible active = %#v", visible[3])
	}

	months := documentMonthLinks(req, []int{3, 5}, filter)
	if len(months) != 2 || months[0].Label != "Mrz" || months[1].URL != "/?month=5&q=abc&tags=steuer&year=2021" || !months[1].Active {
		t.Fatalf("month links = %#v", months)
	}

	reset := dateFilterResetURL(req)
	if reset != "/?q=abc&tags=steuer" {
		t.Fatalf("reset URL = %q", reset)
	}
}

func TestDocumentListPaginationData(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?q=abc&page=2&notice=ok&highlight=9", nil)
	got := documentListPaginationData(req, 2, 100, 100, 421)

	if got.Start != 101 || got.End != 200 || got.Total != 421 || got.PerPage != 100 {
		t.Fatalf("pagination = %#v", got)
	}
	if got.PrevURL != "/?q=abc" || got.NextURL != "/?page=3&q=abc" || got.PageSizeReturnURL != "/?q=abc" {
		t.Fatalf("urls = %#v", got)
	}
}

func TestPageCount(t *testing.T) {
	tests := []struct {
		total    int
		pageSize int
		want     int
	}{
		{total: 0, pageSize: 120, want: 0},
		{total: 1, pageSize: 120, want: 1},
		{total: 120, pageSize: 120, want: 1},
		{total: 121, pageSize: 120, want: 2},
		{total: 241, pageSize: 120, want: 3},
		{total: 241, pageSize: 0, want: 0},
	}
	for _, tt := range tests {
		if got := pageCount(tt.total, tt.pageSize); got != tt.want {
			t.Fatalf("pageCount(%d, %d) = %d, want %d", tt.total, tt.pageSize, got, tt.want)
		}
	}
}

func TestDocumentViewURLPreservesReturnQuery(t *testing.T) {
	tests := map[string]string{
		"/":         "/documents/1/view",
		"/?q=abc":   "/documents/1/view?return=%2F%3Fq%3Dabc",
		"/folders":  "/documents/1/view?return=%2Ffolders",
		"//bad-url": "/documents/1/view",
	}
	for returnURL, want := range tests {
		if got := documentViewURL(1, returnURL, ""); got != want {
			t.Fatalf("documentViewURL(1, %q, \"\") = %q, want %q", returnURL, got, want)
		}
	}
}
