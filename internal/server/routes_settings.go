// Datei definiert die HTTP-Routen der Einstellungsdomaene.
package server

var settingsRouteSpecs = []routeSpec{
	{pattern: "GET /settings", capabilities: authCapSystemManage, handler: (*Server).handleSettings},
	{pattern: "POST /settings", capabilities: authCapSystemManage, handler: (*Server).handleSaveSettings},
	{pattern: "POST /settings/favicon", capabilities: authCapSystemManage, handler: (*Server).handleUploadFavicon},
	{pattern: "POST /settings/favicon/reset", capabilities: authCapSystemManage, handler: (*Server).handleResetFavicon},
	{pattern: "GET /settings/mail-import", capabilities: authCapSystemManage, handler: (*Server).handleMailImportSettings},
	{pattern: "POST /settings/mail-import", capabilities: authCapSystemManage, handler: (*Server).handleSaveMailImportSettings},
	{pattern: "POST /settings/mail-import/test", capabilities: authCapSystemManage, handler: (*Server).handleTestMailImportSettings},
	{pattern: "POST /settings/mail-import/run", capabilities: authCapSystemManage, handler: (*Server).handleRunMailImportNow},
	{pattern: "POST /settings/columns", capabilities: authCapSystemManage, handler: (*Server).handleSaveColumns},
	{pattern: "POST /settings/page-size", capabilities: authCapSystemManage, handler: (*Server).handleSavePageSize},
}

var photoSettingsRouteSpecs = []routeSpec{
	{pattern: "GET /settings/photos", capabilities: authCapPhotosManage, handler: (*Server).handlePhotoSettings},
	{pattern: "POST /settings/photos", capabilities: authCapPhotosManage, handler: (*Server).handleSavePhotoSettings},
	{pattern: "POST /settings/photos/index/run", capabilities: authCapPhotosManage, handler: (*Server).handleRunPhotoIndexWorkerNow},
	{pattern: "POST /settings/photos/thumbnails/run", capabilities: authCapPhotosManage, handler: (*Server).handleRunPhotoThumbnailWorkerNow},
}
