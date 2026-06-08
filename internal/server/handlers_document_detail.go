// Datei rendert Detailseiten und Detailaktionen fuer einzelne Dokumente.
package server

import (
	"net/http"
	"sort"
	"time"

	"bearstack/internal/document"
)

func (s *Server) handleDocument(w http.ResponseWriter, r *http.Request) {
	s.renderDocumentDetail(w, r, documentReadOnlyViewFromRequest(r))
}

func (s *Server) handleDocumentView(w http.ResponseWriter, r *http.Request) {
	s.renderDocumentDetail(w, r, true)
}

func documentReadOnlyViewFromRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	_, ok := r.URL.Query()["view"]
	return ok
}

func (s *Server) renderDocumentDetail(w http.ResponseWriter, r *http.Request, readOnlyView bool) {
	doc, err := s.documentFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	tags, err := s.repo.ListTags(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	fields, err := s.repo.ListCustomFields(r.Context())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	linkedDocs, err := s.repo.LinkedDocuments(r.Context(), doc.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	groupedDocs, err := s.repo.GroupedDocuments(r.Context(), doc.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	ocrJob, hasOCRJob, err := s.repo.LatestOCRJobForDocument(r.Context(), doc.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	var latestOCRJob *document.OCRJob
	if hasOCRJob {
		latestOCRJob = &ocrJob
	}
	returnURL := withHighlight(safeReturnURL(r.URL.Query().Get("return")), doc.ID)
	currentURL := documentURL(doc.ID, returnURL, "")
	if readOnlyView {
		currentURL = documentViewURL(doc.ID, returnURL, "")
	}
	s.render(w, r, "detail.html", PageData{
		Title:                doc.Title,
		Active:               "documents",
		Assets:               documentDetailAssets(),
		Document:             doc,
		DocumentReadOnlyView: readOnlyView,
		Tags:                 tags,
		TagDescriptions:      tagDescriptionMap(tags),
		TagStyles:            tagStyleMap(tags),
		CustomFields:         fields,
		RelatedDocuments:     relatedDocumentItems(doc, tags, linkedDocs, groupedDocs),
		OCRJob:               latestOCRJob,
		Notice:               r.URL.Query().Get("notice"),
		ReturnURL:            returnURL,
		CurrentURL:           currentURL,
	})
}

func relatedDocumentItems(current document.Document, tags []document.Tag, linkedDocs, groupedDocs []document.Document) []RelatedDocument {
	items := make([]RelatedDocument, 0, len(linkedDocs)+len(groupedDocs))
	index := make(map[int64]int, len(linkedDocs)+len(groupedDocs))
	groupModeTags := groupModeTagSet(tags)

	for _, doc := range linkedDocs {
		if _, ok := index[doc.ID]; ok {
			continue
		}
		index[doc.ID] = len(items)
		items = append(items, RelatedDocument{Document: doc, IsLinked: true})
	}
	for _, doc := range groupedDocs {
		if itemIndex, ok := index[doc.ID]; ok {
			items[itemIndex].IsGrouped = true
			items[itemIndex].GroupTags = sharedGroupTags(current.Tags, doc.Tags, groupModeTags)
			continue
		}
		index[doc.ID] = len(items)
		items = append(items, RelatedDocument{
			Document:  doc,
			IsGrouped: true,
			GroupTags: sharedGroupTags(current.Tags, doc.Tags, groupModeTags),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		left, right := relatedDocumentSortDate(items[i].Document), relatedDocumentSortDate(items[j].Document)
		if !left.Equal(right) {
			return left.After(right)
		}
		if !items[i].UploadedAt.Equal(items[j].UploadedAt) {
			return items[i].UploadedAt.After(items[j].UploadedAt)
		}
		return items[i].ID > items[j].ID
	})
	return items
}

func groupModeTagSet(tags []document.Tag) map[string]struct{} {
	groupTags := make(map[string]struct{})
	for _, tag := range tags {
		if tag.GroupMode {
			groupTags[tag.Name] = struct{}{}
		}
	}
	return groupTags
}

func sharedGroupTags(currentTags, relatedTags []string, groupModeTags map[string]struct{}) []string {
	if len(currentTags) == 0 || len(relatedTags) == 0 || len(groupModeTags) == 0 {
		return nil
	}
	related := make(map[string]struct{}, len(relatedTags))
	for _, tag := range relatedTags {
		related[tag] = struct{}{}
	}
	var shared []string
	for _, tag := range currentTags {
		if _, ok := groupModeTags[tag]; !ok {
			continue
		}
		if _, ok := related[tag]; ok {
			shared = append(shared, tag)
		}
	}
	return shared
}

func relatedDocumentSortDate(doc document.Document) time.Time {
	if doc.DocumentDate != nil {
		return *doc.DocumentDate
	}
	return doc.UploadedAt
}
