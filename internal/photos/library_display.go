// Datei bereitet Anzeigeoptionen und Sortiermodi fuer Fotoordner und Medien auf.
package photos

import (
	"strings"
	"time"

	"bearstack/internal/textmeta"
)

func decorateListingDisplay(listing *Listing) {
	for i := range listing.Breadcrumbs {
		if listing.Breadcrumbs[i].DisplayName != "" || listing.Breadcrumbs[i].DisplayDate != nil {
			continue
		}
		listing.Breadcrumbs[i].DisplayName, listing.Breadcrumbs[i].DisplayDate = folderNameDisplay(listing.Breadcrumbs[i].Name)
	}
	for i := range listing.Folders {
		listing.Folders[i].DisplayName, listing.Folders[i].DisplayDate = folderNameDisplay(listing.Folders[i].Name)
	}
}

func folderNameDisplay(name string) (string, *time.Time) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	title, date := textmeta.FromFilename(name + ".folder")
	if date != nil && title == "Dokument" {
		title = ""
	}
	if title == "" && date == nil {
		title = name
	}
	return title, date
}
