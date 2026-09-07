// Datei bereitet Anzeigeoptionen und Sortiermodi fuer Fotoordner und Medien auf.
package photos

import (
	"path"
	"strings"
	"time"

	"bearstack/internal/textmeta"
)

// mediaDisplayPath uses the gallery breadcrumb rules for every folder while
// preserving the source filename. Never expose the host's filesystem root.
func mediaDisplayPath(rel string) string {
	dir := path.Dir(rel)
	if dir == "." {
		dir = ""
	}
	crumbs := breadcrumbs(dir)
	parts := make([]string, 0, len(crumbs)+1)
	for _, crumb := range crumbs {
		label := crumb.DisplayName
		if crumb.DisplayDate != nil {
			label = crumb.DisplayDate.Format("02.01.2006")
			if crumb.DisplayName != "" {
				label += " · " + crumb.DisplayName
			}
		} else if label == "" {
			label = crumb.Name
		}
		parts = append(parts, label)
	}
	return strings.Join(append(parts, path.Base(rel)), " / ")
}

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
