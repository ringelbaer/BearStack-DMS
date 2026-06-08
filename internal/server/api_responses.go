// Datei definiert einheitliche API-Antworten und JSON-Fehlerformate fuer HTTP-Handler.
package server

import (
	"strconv"
	"time"

	"bearstack/internal/document"
)

type customFieldAPIResponse struct {
	ID                      int64  `json:"id"`
	Label                   string `json:"label"`
	Position                int    `json:"position"`
	AutocompleteEnabled     bool   `json:"autocomplete_enabled"`
	ValueFolderMinDocuments int    `json:"value_folder_min_documents"`
}

func customFieldAPIResponsesFrom(fields []document.CustomField) []customFieldAPIResponse {
	responses := make([]customFieldAPIResponse, len(fields))
	for i, field := range fields {
		responses[i] = customFieldAPIResponse{
			ID:                      field.ID,
			Label:                   field.Label,
			Position:                field.Position,
			AutocompleteEnabled:     field.AutocompleteEnabled,
			ValueFolderMinDocuments: field.ValueFolderMinDocuments,
		}
	}
	return responses
}

type documentAPIResponse struct {
	ID                     int64                         `json:"id"`
	OriginalName           string                        `json:"original_name"`
	Title                  string                        `json:"title"`
	Description            string                        `json:"description"`
	Tags                   []string                      `json:"tags"`
	CustomValues           []documentCustomValueResponse `json:"custom_values"`
	MIMEType               string                        `json:"mime_type"`
	SizeBytes              int64                         `json:"size_bytes"`
	SHA256                 string                        `json:"sha256"`
	UploadWay              string                        `json:"upload_way"`
	UploadWayLabel         string                        `json:"upload_way_label"`
	ContentTextSource      string                        `json:"content_text_source"`
	ContentTextSourceLabel string                        `json:"content_text_source_label"`
	DocumentDate           string                        `json:"document_date,omitempty"`
	UploadedAt             string                        `json:"uploaded_at"`
	UpdatedAt              string                        `json:"updated_at"`
	DeletedAt              string                        `json:"deleted_at,omitempty"`
	DuplicateCount         int                           `json:"duplicate_count"`
	LinkedCount            int                           `json:"linked_count"`
	DocumentURL            string                        `json:"document_url"`
	DownloadURL            string                        `json:"download_url"`
	PreviewURL             string                        `json:"preview_url"`
	ThumbnailURL           string                        `json:"thumbnail_url"`
}

type documentCustomValueResponse struct {
	FieldID int64  `json:"field_id"`
	Label   string `json:"label"`
	Value   string `json:"value"`
}

func documentAPIResponsesFrom(docs []document.Document, fields []document.CustomField) []documentAPIResponse {
	responses := make([]documentAPIResponse, len(docs))
	for i, doc := range docs {
		responses[i] = documentAPIResponseFrom(doc, fields)
	}
	return responses
}

func documentAPIResponseFrom(doc document.Document, fields []document.CustomField) documentAPIResponse {
	tags := doc.Tags
	if tags == nil {
		tags = []string{}
	}
	response := documentAPIResponse{
		ID:                     doc.ID,
		OriginalName:           doc.OriginalName,
		Title:                  doc.Title,
		Description:            doc.Description,
		Tags:                   tags,
		CustomValues:           documentCustomValuesFrom(doc, fields),
		MIMEType:               doc.MIMEType,
		SizeBytes:              doc.SizeBytes,
		SHA256:                 doc.SHA256,
		UploadWay:              doc.UploadWay,
		UploadWayLabel:         doc.UploadWayLabel(),
		ContentTextSource:      doc.ContentTextSource,
		ContentTextSourceLabel: doc.ContentTextSourceLabel(),
		UploadedAt:             doc.UploadedAt.Format(time.RFC3339),
		UpdatedAt:              doc.UpdatedAt.Format(time.RFC3339),
		DuplicateCount:         doc.DuplicateCount,
		LinkedCount:            doc.LinkedCount,
		DocumentURL:            "/documents/" + strconv.FormatInt(doc.ID, 10),
		DownloadURL:            "/documents/" + strconv.FormatInt(doc.ID, 10) + "/download",
		PreviewURL:             "/documents/" + strconv.FormatInt(doc.ID, 10) + "/preview",
		ThumbnailURL:           "/documents/" + strconv.FormatInt(doc.ID, 10) + "/thumbnail",
	}
	if doc.DocumentDate != nil {
		response.DocumentDate = doc.DocumentDate.Format("2006-01-02")
	}
	if doc.DeletedAt != nil {
		response.DeletedAt = doc.DeletedAt.Format(time.RFC3339)
	}
	return response
}

func documentCustomValuesFrom(doc document.Document, fields []document.CustomField) []documentCustomValueResponse {
	if len(doc.CustomValues) == 0 {
		return []documentCustomValueResponse{}
	}
	values := make([]documentCustomValueResponse, 0, len(doc.CustomValues))
	for _, field := range fields {
		value := doc.CustomValues[field.ID]
		if value == "" {
			continue
		}
		values = append(values, documentCustomValueResponse{
			FieldID: field.ID,
			Label:   field.Label,
			Value:   value,
		})
	}
	return values
}

type documentAPIFilterResponse struct {
	Query        string                         `json:"query"`
	Tags         []string                       `json:"tags"`
	CustomFields []documentAPICustomFieldFilter `json:"custom_fields"`
	From         string                         `json:"from,omitempty"`
	To           string                         `json:"to,omitempty"`
	Year         int                            `json:"year,omitempty"`
	Month        int                            `json:"month,omitempty"`
	Sort         string                         `json:"sort"`
	Direction    string                         `json:"direction"`
	Trash        bool                           `json:"trash"`
}

type documentAPICustomFieldFilter struct {
	FieldID int64  `json:"field_id"`
	Value   string `json:"value"`
}

func documentAPIFilterResponseFrom(filter document.ListFilter) documentAPIFilterResponse {
	tags := filter.Tags
	if tags == nil {
		tags = []string{}
	}
	customFields := make([]documentAPICustomFieldFilter, 0, len(filter.CustomFields))
	for _, fieldFilter := range filter.CustomFields {
		value := document.CleanCustomFieldFilterValue(fieldFilter.Value)
		if fieldFilter.FieldID <= 0 || value == "" {
			continue
		}
		customFields = append(customFields, documentAPICustomFieldFilter{
			FieldID: fieldFilter.FieldID,
			Value:   value,
		})
	}
	response := documentAPIFilterResponse{
		Query:        filter.Query,
		Tags:         tags,
		CustomFields: customFields,
		Year:         filter.Year,
		Month:        filter.Month,
		Sort:         filter.Sort,
		Direction:    filter.Direction,
		Trash:        filter.Trash,
	}
	if filter.From != nil {
		response.From = filter.From.Format("2006-01-02")
	}
	if filter.To != nil {
		response.To = filter.To.Format("2006-01-02")
	}
	return response
}

type documentAPIPaginationResult struct {
	Page    int  `json:"page"`
	PerPage int  `json:"per_page"`
	Total   int  `json:"total"`
	Start   int  `json:"start"`
	End     int  `json:"end"`
	HasPrev bool `json:"has_prev"`
	HasNext bool `json:"has_next"`
}

func documentAPIPaginationFrom(filter document.ListFilter, count, total int) documentAPIPaginationResult {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	perPage := filter.Limit
	if perPage < 1 {
		perPage = defaultDocumentPageSize
	}
	result := documentAPIPaginationResult{
		Page:    page,
		PerPage: perPage,
		Total:   total,
		HasPrev: page > 1,
		HasNext: filter.Offset+count < total,
	}
	if count > 0 && total > 0 {
		result.Start = filter.Offset + 1
		result.End = filter.Offset + count
		if result.End > total {
			result.End = total
		}
	}
	return result
}
