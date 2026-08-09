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
	{pattern: "GET /settings/users", capabilities: authCapSystemUsersManage, handler: (*Server).handleUsers},
	{pattern: "GET /settings/users/new", capabilities: authCapSystemUsersManage, handler: (*Server).handleNewUser},
	{pattern: "POST /settings/users", capabilities: authCapSystemUsersManage, handler: (*Server).handleCreateUser},
	{pattern: "GET /settings/users/{id}", capabilities: authCapSystemUsersManage, handler: (*Server).handleEditUser},
	{pattern: "POST /settings/users/{id}", capabilities: authCapSystemUsersManage, handler: (*Server).handleUpdateUser},
	{pattern: "POST /settings/users/{id}/password", capabilities: authCapSystemUsersManage, handler: (*Server).handleResetUserPassword},
	{pattern: "POST /settings/users/{id}/enable", capabilities: authCapSystemUsersManage, handler: (*Server).handleEnableUser},
	{pattern: "POST /settings/users/{id}/disable", capabilities: authCapSystemUsersManage, handler: (*Server).handleDisableUser},
	{pattern: "POST /settings/users/{id}/delete", capabilities: authCapSystemUsersManage, handler: (*Server).handleDeleteUser},
}

var photoSettingsRouteSpecs = []routeSpec{
	{pattern: "GET /settings/photos", capabilities: authCapPhotosManage, handler: (*Server).handlePhotoSettings},
	{pattern: "POST /settings/photos", capabilities: authCapPhotosManage, handler: (*Server).handleSavePhotoSettings},
	{pattern: "POST /settings/photos/index/run", capabilities: authCapPhotosManage, handler: (*Server).handleRunPhotoIndexWorkerNow},
	{pattern: "POST /settings/photos/thumbnails/run", capabilities: authCapPhotosManage, handler: (*Server).handleRunPhotoThumbnailWorkerNow},
}
