package textmeta

import "testing"

func TestFromFilenameExtractsDateAndTitle(t *testing.T) {
	title, date := FromFilename("2024-03-15_Rechnung_Strom.pdf")
	if title != "Rechnung Strom" {
		t.Fatalf("title = %q", title)
	}
	if date == nil || date.Format("2006-01-02") != "2024-03-15" {
		t.Fatalf("date = %v", date)
	}
}

func TestFromFilenameSupportsGermanDate(t *testing.T) {
	title, date := FromFilename("15.03.2024 - Versicherung.pdf")
	if title != "Versicherung" {
		t.Fatalf("title = %q", title)
	}
	if date == nil || date.Format("2006-01-02") != "2024-03-15" {
		t.Fatalf("date = %v", date)
	}
}
