// Datei definiert die HTTP-Routen der globalen Anwendungsebene.
package server

var coreRouteSpecs = []routeSpec{
	{pattern: "GET /login", capabilities: 0, handler: (*Server).handleLogin},
	{pattern: "POST /login", capabilities: 0, handler: (*Server).handleLogin},
	{pattern: "POST /logout", capabilities: 0, handler: (*Server).handleLogout},
	{pattern: "GET /favicon/custom", capabilities: 0, handler: (*Server).handleCustomFavicon},
	{pattern: "GET /healthz", capabilities: 0, handler: (*Server).handleHealth},
	{pattern: "GET /api", capabilities: authCapDocumentsRead | authCapDocumentsUpload | authCapDocumentsWebDAVRead | authCapPhotosRead, requireAny: true, handler: (*Server).handleAPI},
	{pattern: "GET /help", capabilities: 0, handler: (*Server).handleHelp},
	{pattern: "GET /log", capabilities: authCapSystemAudit, handler: (*Server).handleAuditLog},
}
