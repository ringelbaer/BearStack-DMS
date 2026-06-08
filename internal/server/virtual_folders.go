// Datei erzeugt virtuelle Ordneransichten fuer spezielle Dokument- und Suchkategorien.
package server

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bearstack/internal/document"
)

const searchFavoritesFolderName = "00 Suchfavoriten"

type namedDocument struct {
	Name     string
	Document document.Document
}

const (
	virtualFolderSegmentTag        = "tag"
	virtualFolderSegmentFieldValue = "field"
)

type virtualFolderSegment struct {
	Kind       string
	Tag        string
	FieldID    int64
	FieldLabel string
	Value      string
}

type virtualFolderSelection struct {
	Segments []virtualFolderSegment
}

func virtualFolderSelectionFromPathValues(values []string) (virtualFolderSelection, error) {
	segments := make([]virtualFolderSegment, 0, len(values))
	for _, value := range values {
		kind, rest, ok := strings.Cut(value, ":")
		if !ok {
			return virtualFolderSelection{}, fmt.Errorf("ungültiger Ordnerpfad")
		}
		switch kind {
		case virtualFolderSegmentTag:
			tags := normalizeTagValues([]string{rest}, "")
			if len(tags) == 0 {
				return virtualFolderSelection{}, fmt.Errorf("ungültiger Tag-Ordner")
			}
			segments = append(segments, virtualFolderSegment{Kind: virtualFolderSegmentTag, Tag: tags[0]})
		case virtualFolderSegmentFieldValue:
			fieldIDText, fieldValue, ok := strings.Cut(rest, ":")
			if !ok {
				return virtualFolderSelection{}, fmt.Errorf("ungültiger Feldwert-Ordner")
			}
			fieldID, err := strconv.ParseInt(strings.TrimSpace(fieldIDText), 10, 64)
			if err != nil || fieldID <= 0 {
				return virtualFolderSelection{}, fmt.Errorf("ungültiger Feldwert-Ordner")
			}
			fieldValue = document.CleanCustomFieldFilterValue(fieldValue)
			if fieldValue == "" {
				return virtualFolderSelection{}, fmt.Errorf("ungültiger Feldwert-Ordner")
			}
			segments = append(segments, virtualFolderSegment{Kind: virtualFolderSegmentFieldValue, FieldID: fieldID, Value: fieldValue})
		default:
			return virtualFolderSelection{}, fmt.Errorf("ungültiger Ordnerpfad")
		}
	}
	return virtualFolderSelection{Segments: segments}, nil
}

func (selection virtualFolderSelection) Depth() int {
	return len(selection.Segments)
}

func (selection virtualFolderSelection) HasCustomFieldValues() bool {
	for _, segment := range selection.Segments {
		if segment.Kind == virtualFolderSegmentFieldValue {
			return true
		}
	}
	return false
}

func (selection virtualFolderSelection) Tags() []string {
	tags := make([]string, 0, len(selection.Segments))
	for _, segment := range selection.Segments {
		if segment.Kind == virtualFolderSegmentTag {
			tags = append(tags, segment.Tag)
		}
	}
	return normalizeTagValues(tags, "")
}

func (selection virtualFolderSelection) CustomFieldFilters() []document.CustomFieldFilter {
	filters := make([]document.CustomFieldFilter, 0, len(selection.Segments))
	for _, segment := range selection.Segments {
		if segment.Kind != virtualFolderSegmentFieldValue {
			continue
		}
		filters = append(filters, document.CustomFieldFilter{
			FieldID: segment.FieldID,
			Value:   segment.Value,
			Exact:   true,
		})
	}
	return filters
}

func (selection virtualFolderSelection) ApplyToFilter(filter document.ListFilter) document.ListFilter {
	filter.Tags = selection.Tags()
	filter.CustomFields = append(filter.CustomFields, selection.CustomFieldFilters()...)
	return filter
}

func (selection virtualFolderSelection) AppendTag(tag string) virtualFolderSelection {
	next := virtualFolderSelection{Segments: append([]virtualFolderSegment(nil), selection.Segments...)}
	tags := normalizeTagValues([]string{tag}, "")
	if len(tags) == 0 {
		return next
	}
	next.Segments = append(next.Segments, virtualFolderSegment{Kind: virtualFolderSegmentTag, Tag: tags[0]})
	return next
}

func (selection virtualFolderSelection) AppendCustomFieldValue(value document.CustomFieldValueFolder) virtualFolderSelection {
	next := virtualFolderSelection{Segments: append([]virtualFolderSegment(nil), selection.Segments...)}
	next.Segments = append(next.Segments, virtualFolderSegment{
		Kind:       virtualFolderSegmentFieldValue,
		FieldID:    value.FieldID,
		FieldLabel: value.FieldLabel,
		Value:      value.Value,
	})
	return next
}

func (selection virtualFolderSelection) PathValues() []string {
	values := make([]string, 0, len(selection.Segments))
	for _, segment := range selection.Segments {
		switch segment.Kind {
		case virtualFolderSegmentTag:
			values = append(values, virtualFolderSegmentTag+":"+segment.Tag)
		case virtualFolderSegmentFieldValue:
			values = append(values, virtualFolderSegmentFieldValue+":"+strconv.FormatInt(segment.FieldID, 10)+":"+segment.Value)
		}
	}
	return values
}

func (selection virtualFolderSelection) WithFieldLabels(fields []document.CustomField) virtualFolderSelection {
	labels := make(map[int64]string, len(fields))
	for _, field := range fields {
		labels[field.ID] = field.Label
	}
	next := virtualFolderSelection{Segments: append([]virtualFolderSegment(nil), selection.Segments...)}
	for i := range next.Segments {
		if next.Segments[i].Kind == virtualFolderSegmentFieldValue && next.Segments[i].FieldLabel == "" {
			next.Segments[i].FieldLabel = labels[next.Segments[i].FieldID]
		}
	}
	return next
}

func (selection virtualFolderSelection) LegacyTagsOnly() ([]string, bool) {
	tags := make([]string, 0, len(selection.Segments))
	for _, segment := range selection.Segments {
		if segment.Kind != virtualFolderSegmentTag {
			return nil, false
		}
		tags = append(tags, segment.Tag)
	}
	return tags, true
}

func virtualFolderSegmentLabel(segment virtualFolderSegment) string {
	switch segment.Kind {
	case virtualFolderSegmentTag:
		return segment.Tag
	case virtualFolderSegmentFieldValue:
		return customFieldValueFolderLabel(segment.FieldLabel, segment.Value)
	default:
		return ""
	}
}

func customFieldValueFolderLabel(fieldLabel, value string) string {
	fieldLabel = strings.Join(strings.Fields(strings.TrimSpace(fieldLabel)), " ")
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if fieldLabel == "" {
		return value
	}
	if value == "" {
		return fieldLabel
	}
	return fieldLabel + ": " + value
}

func folderPathValues(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" {
			continue
		}
		items = append(items, value)
	}
	return items
}

func isSearchFavoritesFolderName(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), searchFavoritesFolderName)
}

func searchFavoriteFilter(favorite document.SearchFavorite, now time.Time, page, perPage int) document.ListFilter {
	if now.IsZero() {
		now = time.Now()
	}
	filter := document.ListFilter{
		Query:        favorite.Query,
		Tags:         append([]string(nil), favorite.Tags...),
		CustomFields: append([]document.CustomFieldFilter(nil), favorite.CustomFields...),
		Sort:         document.ListSortDate,
		Direction:    document.ListDirectionDescending,
	}
	if page > 0 && perPage > 0 {
		filter.Page = page
		filter.Limit = perPage
		filter.Offset = (page - 1) * perPage
	}
	if favorite.DateMode == document.SearchFavoriteDateYear && favorite.DateYear > 0 {
		filter.From, filter.To = yearRange(favorite.DateYear)
	} else if from, to := searchFavoriteDynamicRange(favorite.DateMode, now); from != nil && to != nil {
		filter.From = from
		filter.To = to
	}
	return filter
}

func findSearchFavoriteByName(favorites []document.SearchFavorite, name string) (document.SearchFavorite, bool) {
	for _, favorite := range favorites {
		if strings.EqualFold(favorite.Name, name) {
			return favorite, true
		}
	}
	return document.SearchFavorite{}, false
}

func namedDocuments(docs []document.Document) []namedDocument {
	items := make([]namedDocument, len(docs))
	counts := make(map[string]int, len(docs))
	for _, doc := range docs {
		counts[documentDisplayName(doc)]++
	}

	used := make(map[string]int, len(docs))
	for i, doc := range docs {
		base := documentDisplayName(doc)
		name := base
		if counts[base] > 1 {
			name = addDocumentID(base, doc.ID)
		}
		if n := used[name]; n > 0 {
			name = addDocumentID(base, doc.ID)
			if used[name] > 0 {
				name = addOrdinal(name, n+1)
			}
		}
		used[name]++
		items[i] = namedDocument{Name: name, Document: doc}
	}
	return items
}

func documentDisplayName(doc document.Document) string {
	ext := filepath.Ext(filepath.Base(doc.OriginalName))
	title := strings.TrimSpace(doc.Title)
	if title == "" {
		title = strings.TrimSpace(strings.TrimSuffix(filepath.Base(doc.OriginalName), ext))
	}
	if title == "" {
		title = "Dokument"
	}

	prefix := "ohne-datum"
	if doc.DocumentDate != nil {
		prefix = doc.DocumentDate.Format("2006-01-02")
	}
	return sanitizePathComponent(prefix + " - " + title + ext)
}

func escapePathComponent(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "/", "%2F")
	return value
}

func unescapePathComponent(value string) (string, error) {
	return url.PathUnescape(value)
}

func sanitizePathComponent(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, ". ")
	if value == "" {
		return "Dokument"
	}
	return value
}

func addDocumentID(name string, id int64) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s (ID %d)%s", base, id, ext)
}

func addOrdinal(name string, ordinal int) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s-%d%s", base, ordinal, ext)
}

func uniqueDocumentNames(docs []document.Document, reserved map[string]struct{}) []namedDocument {
	items := namedDocuments(docs)
	used := make(map[string]struct{}, len(reserved)+len(items))
	for name := range reserved {
		used[strings.ToLower(name)] = struct{}{}
	}
	for i := range items {
		key := strings.ToLower(items[i].Name)
		if _, exists := used[key]; exists {
			items[i].Name = addDocumentID(documentDisplayName(items[i].Document), items[i].Document.ID)
			key = strings.ToLower(items[i].Name)
			for ordinal := 2; ; ordinal++ {
				if _, exists := used[key]; !exists {
					break
				}
				items[i].Name = addOrdinal(addDocumentID(documentDisplayName(items[i].Document), items[i].Document.ID), ordinal)
				key = strings.ToLower(items[i].Name)
			}
		}
		used[key] = struct{}{}
	}
	return items
}
