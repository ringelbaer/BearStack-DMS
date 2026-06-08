// Datei verbindet virtuelle Ordnerauswahl mit HTTP-Requests und HTML-Breadcrumbs.
package server

import "net/http"

func virtualFolderSelectionFromRequest(r *http.Request) (virtualFolderSelection, error) {
	q := r.URL.Query()
	if values := folderPathValues(q["path"]); len(values) > 0 {
		return virtualFolderSelectionFromPathValues(values)
	}

	tags := normalizeTagValues(folderPathValues(q["tags"]), "")
	segments := make([]virtualFolderSegment, len(tags))
	for i, tag := range tags {
		segments[i] = virtualFolderSegment{Kind: virtualFolderSegmentTag, Tag: tag}
	}
	return virtualFolderSelection{Segments: segments}, nil
}

func virtualFolderBreadcrumb(r *http.Request, selection virtualFolderSelection) []FolderCrumb {
	crumbs := []FolderCrumb{{Label: "Ordner", URL: folderURL(r, virtualFolderSelection{})}}
	for i, segment := range selection.Segments {
		current := virtualFolderSelection{Segments: append([]virtualFolderSegment(nil), selection.Segments[:i+1]...)}
		crumbs = append(crumbs, FolderCrumb{
			Label:   virtualFolderSegmentLabel(segment),
			URL:     folderURL(r, current),
			Current: i == len(selection.Segments)-1,
			IsTag:   segment.Kind == virtualFolderSegmentTag,
		})
	}
	crumbs[len(crumbs)-1].Current = true
	return crumbs
}
