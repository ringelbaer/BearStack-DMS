// Datei definiert die HTTP-Routen der Dokumentdomaene.
package server

func homeRouteSpecs(photosEnabled bool) []routeSpec {
	if photosEnabled {
		return []routeSpec{
			{pattern: "GET /{$}", capabilities: authCapDocumentsRead | authCapPhotosRead, requireAny: true, handler: (*Server).handleHome},
		}
	}
	return []routeSpec{
		{pattern: "GET /{$}", capabilities: authCapDocumentsRead, handler: (*Server).handleHome},
	}
}

var documentRouteSpecs = []routeSpec{
	{pattern: "GET /documents", capabilities: authCapDocumentsRead, handler: (*Server).handleIndex},
	{pattern: "GET /cloud", capabilities: authCapDocumentsRead, handler: (*Server).handleCloud},
	{pattern: "GET /trash", capabilities: authCapDocumentsDelete, handler: (*Server).handleTrash},
	{pattern: "POST /trash/empty", capabilities: authCapDocumentsDelete, handler: (*Server).handleEmptyTrash},
	{pattern: "GET /duplicates", capabilities: authCapDocumentsRead, handler: (*Server).handleDuplicates},
	{pattern: "GET /statistics", capabilities: authCapDocumentsRead, handler: (*Server).handleStatistics},
	{pattern: "POST /statistics/text-issues/ocr/{lang}", capabilities: authCapDocumentsEdit, handler: (*Server).handleProblemTextOCR},
	{pattern: "GET /api/documents", capabilities: authCapDocumentsRead, handler: (*Server).handleAPIDocuments},
	{pattern: "GET /api/documents/{id}/download", capabilities: authCapDocumentsRead, handler: (*Server).handleDownload},
	{pattern: "GET /api/fields", capabilities: authCapDocumentsRead, handler: (*Server).handleAPIFields},
	{pattern: "GET /api/folders", capabilities: authCapDocumentsRead, handler: (*Server).handleAPIFolders},
	{pattern: "GET /api/tags", capabilities: authCapDocumentsRead, handler: (*Server).handleAPITags},
	{pattern: "GET /folders", capabilities: authCapDocumentsRead, handler: (*Server).handleFolders},
	{pattern: "GET /tags", capabilities: authCapDocumentsRead | authCapPhotosRead | authCapPhotosManage, requireAny: true, handler: (*Server).handleTags},
	{pattern: "POST /tags", capabilities: authCapDocumentsStructure, handler: (*Server).handleSaveTag},
	{pattern: "GET /tags/{id}", capabilities: authCapDocumentsStructure, handler: (*Server).handleTagDetail},
	{pattern: "POST /tags/{id}", capabilities: authCapDocumentsStructure, handler: (*Server).handleUpdateTag},
	{pattern: "POST /tags/{id}/delete", capabilities: authCapDocumentsStructure, handler: (*Server).handleDeleteTag},
	{pattern: "POST /tags/{id}/rules", capabilities: authCapDocumentsStructure, handler: (*Server).handleSaveTagRules},
	{pattern: "GET /search-favorites", capabilities: authCapDocumentsStructure, handler: (*Server).handleSearchFavorites},
	{pattern: "POST /search-favorites", capabilities: authCapDocumentsStructure, handler: (*Server).handleSaveSearchFavorite},
	{pattern: "POST /search-favorites/{id}", capabilities: authCapDocumentsStructure, handler: (*Server).handleUpdateSearchFavorite},
	{pattern: "POST /search-favorites/{id}/delete", capabilities: authCapDocumentsStructure, handler: (*Server).handleDeleteSearchFavorite},
	{pattern: "GET /fields", capabilities: authCapDocumentsStructure, handler: (*Server).handleFields},
	{pattern: "POST /fields", capabilities: authCapDocumentsStructure, handler: (*Server).handleSaveField},
	{pattern: "POST /fields/{id}", capabilities: authCapDocumentsStructure, handler: (*Server).handleUpdateField},
	{pattern: "POST /fields/{id}/autocomplete", capabilities: authCapDocumentsStructure, handler: (*Server).handleFieldAutocomplete},
	{pattern: "GET /fields/{id}/values", capabilities: authCapDocumentsRead | authCapDocumentsStructure, requireAny: true, handler: (*Server).handleFieldValueSuggestions},
	{pattern: "POST /fields/{id}/values", capabilities: authCapDocumentsStructure, handler: (*Server).handleUpdateFieldValue},
	{pattern: "POST /fields/{id}/delete", capabilities: authCapDocumentsStructure, handler: (*Server).handleDeleteField},
	{pattern: "POST /upload", capabilities: authCapDocumentsUpload, handler: (*Server).handleUploadWeb},
	{pattern: "POST /api/upload", capabilities: authCapDocumentsUpload, handler: (*Server).handleUploadAPI},
	{pattern: "GET /export", capabilities: authCapDocumentsRead, handler: (*Server).handleExport},
	{pattern: "POST /documents/link", capabilities: authCapDocumentsEdit, handler: (*Server).handleLinkDocuments},
	{pattern: "POST /documents/tags/add", capabilities: authCapDocumentsEdit, handler: (*Server).handleAddDocumentTags},
	{pattern: "POST /documents/tags/remove", capabilities: authCapDocumentsEdit, handler: (*Server).handleRemoveDocumentTags},
	{pattern: "POST /documents/fields", capabilities: authCapDocumentsEdit, handler: (*Server).handleBulkDocumentFields},
	{pattern: "GET /documents/{id}/view", capabilities: authCapDocumentsRead, handler: (*Server).handleDocumentView},
	{pattern: "GET /documents/{id}", capabilities: authCapDocumentsRead, handler: (*Server).handleDocument},
	{pattern: "POST /documents/{id}/links/{linkedID}/delete", capabilities: authCapDocumentsEdit, handler: (*Server).handleUnlinkDocument},
	{pattern: "POST /documents/{id}/metadata", capabilities: authCapDocumentsEdit, handler: (*Server).handleMetadata},
	{pattern: "POST /documents/{id}/document-date", capabilities: authCapDocumentsEdit, handler: (*Server).handleDocumentDate},
	{pattern: "POST /documents/{id}/tags", capabilities: authCapDocumentsEdit, handler: (*Server).handleDocumentTags},
	{pattern: "POST /documents/{id}/ocr/{lang}", capabilities: authCapDocumentsEdit, handler: (*Server).handleOCR},
	{pattern: "GET /documents/{id}/ocr/status", capabilities: authCapDocumentsRead, handler: (*Server).handleOCRStatus},
	{pattern: "POST /documents/{id}/delete", capabilities: authCapDocumentsDelete, handler: (*Server).handleDelete},
	{pattern: "POST /documents/{id}/restore", capabilities: authCapDocumentsDelete, handler: (*Server).handleRestore},
	{pattern: "POST /documents/{id}/purge", capabilities: authCapDocumentsDelete, handler: (*Server).handlePurge},
	{pattern: "GET /documents/{id}/download", capabilities: authCapDocumentsRead, handler: (*Server).handleDownload},
	{pattern: "GET /documents/{id}/preview", capabilities: authCapDocumentsRead, handler: (*Server).handlePreview},
	{pattern: "GET /documents/{id}/thumbnail", capabilities: authCapDocumentsRead, handler: (*Server).handleThumbnail},
}
