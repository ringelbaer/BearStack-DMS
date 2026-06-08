// Datei definiert sichtbare Dokumentspalten und deren Darstellung in Listenansichten.
package server

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"bearstack/internal/document"
)

func columnVisible(columns map[string]bool, name string) bool {
	if columns == nil {
		for _, column := range defaultDocumentColumns {
			if column == name {
				return true
			}
		}
		return false
	}
	return columns[name]
}

type documentColumnSettings struct {
	options               []DocumentColumn
	table                 []DocumentColumn
	visible               map[string]bool
	order                 []string
	desktopDateUnderTitle bool
}

func (s *Server) documentColumnSettings(ctx context.Context, fields []document.CustomField) (documentColumnSettings, error) {
	value, ok, err := s.repo.GetSetting(ctx, documentColumnsSettingKey)
	if err != nil {
		return documentColumnSettings{}, err
	}
	order := defaultDocumentColumnOrder(fields)
	visible := append([]string(nil), defaultDocumentColumns...)
	if !ok {
		return makeDocumentColumnSettings(fields, order, visible, false), nil
	}
	var stored []string
	if err := json.Unmarshal([]byte(value), &stored); err == nil {
		visible = normalizeColumnSelection(stored, fields)
		order = normalizeColumnOrder(stored, fields)
		return makeDocumentColumnSettings(fields, order, visible, false), nil
	}
	var storedSettings storedDocumentColumnSettings
	if err := json.Unmarshal([]byte(value), &storedSettings); err != nil {
		return makeDocumentColumnSettings(fields, order, visible, false), nil
	}
	visible = normalizeColumnSelection(storedSettings.Visible, fields)
	order = normalizeColumnOrder(storedSettings.Order, fields)
	return makeDocumentColumnSettings(fields, order, visible, storedSettings.DesktopDateUnderTitle), nil
}

type storedDocumentColumnSettings struct {
	Order                 []string `json:"order"`
	Visible               []string `json:"visible"`
	DesktopDateUnderTitle bool     `json:"desktop_date_under_title"`
}

func normalizeColumnSelection(values []string, fields []document.CustomField) []string {
	allowed := allowedDocumentColumns(fields)
	seen := make(map[string]struct{}, len(values))
	columns := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := allowed[value]; !ok {
			continue
		}
		addColumnSelection(&columns, seen, value)
	}
	if len(columns) == 0 {
		return append([]string(nil), defaultDocumentColumns...)
	}
	return columns
}

func normalizeColumnOrder(values []string, fields []document.CustomField) []string {
	allowed := allowedDocumentColumns(fields)
	seen := make(map[string]struct{}, len(values))
	columns := make([]string, 0, len(allowed))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := allowed[value]; !ok {
			continue
		}
		addColumnSelection(&columns, seen, value)
	}
	for _, value := range defaultDocumentColumnOrder(fields) {
		addColumnSelection(&columns, seen, value)
	}
	return columns
}

func addColumnSelection(columns *[]string, seen map[string]struct{}, value string) {
	if _, ok := seen[value]; ok {
		return
	}
	seen[value] = struct{}{}
	*columns = append(*columns, value)
}

func allowedDocumentColumns(fields []document.CustomField) map[string]struct{} {
	allowed := map[string]struct{}{
		"thumbnail":     {},
		"name":          {},
		"title":         {},
		"tags":          {},
		"document_date": {},
		"upload_date":   {},
		"size":          {},
		"actions":       {},
	}
	for _, field := range fields {
		allowed["field-"+strconv.FormatInt(field.ID, 10)] = struct{}{}
	}
	return allowed
}

func defaultDocumentColumnOrder(fields []document.CustomField) []string {
	columns := []string{"thumbnail", "name", "title", "tags", "document_date", "upload_date"}
	for _, field := range fields {
		columns = append(columns, "field-"+strconv.FormatInt(field.ID, 10))
	}
	columns = append(columns, "size", "actions")
	return columns
}

func makeDocumentColumnSettings(fields []document.CustomField, order, visible []string, desktopDateUnderTitle bool) documentColumnSettings {
	visibleMap := columnsToMap(visible)
	definitions := documentColumnDefinitions(fields)
	options := make([]DocumentColumn, 0, len(order))
	table := make([]DocumentColumn, 0, len(visible))
	for _, key := range order {
		column, ok := definitions[key]
		if !ok {
			continue
		}
		options = append(options, column)
		if visibleMap[key] {
			table = append(table, column)
		}
	}
	return documentColumnSettings{
		options:               options,
		table:                 table,
		visible:               visibleMap,
		order:                 order,
		desktopDateUnderTitle: desktopDateUnderTitle,
	}
}

func documentColumnDefinitions(fields []document.CustomField) map[string]DocumentColumn {
	columns := map[string]DocumentColumn{
		"thumbnail":     {Key: "thumbnail", Label: "Vorschau"},
		"name":          {Key: "name", Label: "Name", SortKey: document.ListSortName},
		"title":         {Key: "title", Label: "Titel", SortKey: document.ListSortTitle},
		"tags":          {Key: "tags", Label: "Tags"},
		"document_date": {Key: "document_date", Label: "Dateidatum", SortKey: document.ListSortDate},
		"upload_date":   {Key: "upload_date", Label: "Uploaddatum", SortKey: document.ListSortUploadDate},
		"size":          {Key: "size", Label: "Größe", SortKey: document.ListSortSize},
		"actions":       {Key: "actions", Label: "Aktionen"},
	}
	for _, field := range fields {
		key := "field-" + strconv.FormatInt(field.ID, 10)
		columns[key] = DocumentColumn{Key: key, Label: field.Label, IsCustom: true, FieldID: field.ID}
	}
	return columns
}

func columnsToMap(columns []string) map[string]bool {
	visible := make(map[string]bool, len(columns))
	for _, column := range columns {
		visible[column] = true
	}
	return visible
}
