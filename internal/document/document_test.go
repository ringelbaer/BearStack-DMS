package document

import "testing"

func TestNormalizeUploadWay(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: UploadWayAPI, want: UploadWayAPI},
		{in: " MAIL ", want: UploadWayMail},
		{in: " WebDav ", want: UploadWayWebDAV},
		{in: "unknown", want: UploadWayWeb},
		{in: "", want: UploadWayWeb},
	}
	for _, tc := range tests {
		if got := NormalizeUploadWay(tc.in); got != tc.want {
			t.Fatalf("NormalizeUploadWay(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeContentTextSource(t *testing.T) {
	if got := NormalizeContentTextSource(ContentTextSourcePDF, "   "); got != ContentTextSourceNone {
		t.Fatalf("empty content source = %q", got)
	}

	tests := []struct {
		source string
		want   string
	}{
		{source: ContentTextSourcePDF, want: ContentTextSourcePDF},
		{source: " FILE ", want: ContentTextSourceFile},
		{source: " RAW ", want: ContentTextSourceRaw},
		{source: ContentTextSourceOCR, want: ContentTextSourceOCR},
		{source: "strange", want: ContentTextSourceUnknown},
	}
	for _, tc := range tests {
		if got := NormalizeContentTextSource(tc.source, "text"); got != tc.want {
			t.Fatalf("NormalizeContentTextSource(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestContentTextSourceLabel(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{source: ContentTextSourcePDF, want: "PDF-Text extrahiert"},
		{source: ContentTextSourceFile, want: "Dateitext extrahiert"},
		{source: ContentTextSourceRaw, want: "Unsicherer Rohtext"},
		{source: ContentTextSourceOCR, want: "OCR-Text"},
		{source: ContentTextSourceNone, want: "Kein Text"},
		{source: "strange", want: "Textquelle unbekannt"},
	}
	for _, tc := range tests {
		if got := ContentTextSourceLabel(tc.source); got != tc.want {
			t.Fatalf("ContentTextSourceLabel(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestUploadWayLabel(t *testing.T) {
	tests := []struct {
		way  string
		want string
	}{
		{way: UploadWayWeb, want: "Web"},
		{way: UploadWayAPI, want: "API"},
		{way: UploadWayMail, want: "E-Mail"},
		{way: UploadWayWebDAV, want: "WebDAV"},
		{way: "unknown", want: "Web"},
	}
	for _, tc := range tests {
		doc := Document{UploadWay: tc.way}
		if got := doc.UploadWayLabel(); got != tc.want {
			t.Fatalf("UploadWayLabel(%q) = %q, want %q", tc.way, got, tc.want)
		}
	}
}

func TestListSortDirectionHelpers(t *testing.T) {
	if got := NormalizeListSort(" NAME "); got != ListSortName {
		t.Fatalf("NormalizeListSort(name) = %q", got)
	}
	if got := NormalizeListSort("invalid"); got != ListSortUploadDate {
		t.Fatalf("NormalizeListSort(invalid) = %q", got)
	}
	if got := NormalizeListDirection("", ListSortName); got != ListDirectionAscending {
		t.Fatalf("NormalizeListDirection default for name = %q", got)
	}
	if got := NormalizeListDirection("", ListSortUploadDate); got != ListDirectionDescending {
		t.Fatalf("NormalizeListDirection default for upload_date = %q", got)
	}
	if got := ToggleListDirection(ListDirectionAscending); got != ListDirectionDescending {
		t.Fatalf("ToggleListDirection(asc) = %q", got)
	}
}

func TestNormalizeCustomFieldValueFolderMinDocuments(t *testing.T) {
	valid := []int{CustomFieldValueFolderAlways, 5, 10, 20, 50}
	for _, value := range valid {
		if got := NormalizeCustomFieldValueFolderMinDocuments(value); got != value {
			t.Fatalf("NormalizeCustomFieldValueFolderMinDocuments(%d) = %d, want %d", value, got, value)
		}
	}
	if got := NormalizeCustomFieldValueFolderMinDocuments(7); got != CustomFieldValueFolderNever {
		t.Fatalf("NormalizeCustomFieldValueFolderMinDocuments(invalid) = %d", got)
	}
}

func TestOCRJobStateHelpers(t *testing.T) {
	if !(OCRJob{Status: OCRJobStatusQueued}).Active() {
		t.Fatal("queued job should be active")
	}
	if !(OCRJob{Status: OCRJobStatusRunning}).Active() {
		t.Fatal("running job should be active")
	}
	if (OCRJob{Status: OCRJobStatusFailed}).Active() {
		t.Fatal("failed job must not be active")
	}
	if !(OCRJob{Status: OCRJobStatusCompleted}).Terminal() {
		t.Fatal("completed job should be terminal")
	}
	if (OCRJob{Status: OCRJobStatusRunning}).Terminal() {
		t.Fatal("running job must not be terminal")
	}
}
