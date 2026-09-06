// Datei definiert die HTTP-Routen der Fotodomaene.
package server

var photoRouteSpecs = []routeSpec{
	{pattern: "GET /photos/people", capabilities: authCapPhotosRead, handler: (*Server).handlePeople},
	{pattern: "GET /photos/people/{id}", capabilities: authCapPhotosRead, handler: (*Server).handlePeople},
	{pattern: "GET /photos/faces/{id}/thumbnail", capabilities: authCapPhotosRead, handler: (*Server).handleFaceThumbnail},
	{pattern: "POST /photos/people/{id}/rename", capabilities: authCapPhotosManage, handler: (*Server).handlePersonRename},
	{pattern: "POST /photos/people/{id}/merge", capabilities: authCapPhotosManage, handler: (*Server).handlePersonMerge},
	{pattern: "POST /photos/faces/edit", capabilities: authCapPhotosManage, handler: (*Server).handleFacesEdit},
	{pattern: "GET /photos", capabilities: authCapPhotosRead, handler: (*Server).handlePhotos},
	{pattern: "GET /photos/media", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoMedia},
	{pattern: "GET /photos/media/info", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoMediaInfo},
	{pattern: "POST /photos/media/info", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoMediaInfo},
	{pattern: "GET /photos/thumbnail", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoThumbnail},
	{pattern: "GET /photos/thumbnail/status", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoThumbnailStatus},
	{pattern: "POST /photos/thumbnail/status", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoThumbnailStatus},
	{pattern: "GET /photos/random", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoRandom},
	{pattern: "GET /photos/frame", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoFrame},
	{pattern: "GET /photos/frame/items", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoFrameItems},
	{pattern: "POST /photos/adminonly", capabilities: authCapPhotosRead, handler: (*Server).handlePhotoAdminOnlyVisibility},
	{pattern: "GET /photos/tags/options", capabilities: authCapPhotosEdit, handler: (*Server).handlePhotoTagOptions},
	{pattern: "POST /photos/tags", capabilities: authCapPhotosEdit, handler: (*Server).handlePhotoTags},
	{pattern: "POST /photos/tags/add", capabilities: authCapPhotosEdit, handler: (*Server).handleAddPhotoTags},
	{pattern: "POST /photos/tags/remove", capabilities: authCapPhotosEdit, handler: (*Server).handleRemovePhotoTags},
	{pattern: "POST /photos/tags/library", capabilities: authCapPhotosManage, handler: (*Server).handleSavePhotoTag},
	{pattern: "POST /photos/tags/library/rename", capabilities: authCapPhotosManage, handler: (*Server).handleRenamePhotoTag},
	{pattern: "POST /photos/tags/library/delete", capabilities: authCapPhotosManage, handler: (*Server).handleDeletePhotoTag},
}
