// Datei definiert die HTTP-Routen der WebDAV-Schnittstelle.
package server

import "net/http"

var webDAVWellKnownRouteSpecs = []routeSpec{
	{pattern: webDAVWellKnownPrefix, capabilities: 0, handler: (*Server).handleWebDAVWellKnown},
	{pattern: webDAVWellKnownPrefix + "/", capabilities: 0, handler: (*Server).handleWebDAVWellKnown},
}

func webDAVRouteSpecs(webDAVPath string) []routeSpec {
	methods := []string{
		http.MethodGet,
		http.MethodOptions,
		http.MethodPut,
		http.MethodPost,
		http.MethodDelete,
		http.MethodPatch,
		"PROPFIND",
		"PROPPATCH",
		"MKCOL",
		"MOVE",
		"COPY",
		"LOCK",
		"UNLOCK",
	}
	routes := make([]routeSpec, 0, len(methods)*2)
	for _, method := range methods {
		capability := authCapDocumentsWebDAVRead
		if method == http.MethodPut {
			capability = authCapDocumentsWebDAVRead | authCapDocumentsUpload
		}
		routes = append(routes,
			routeSpec{pattern: method + " " + webDAVPath, capabilities: capability, handler: (*Server).handleWebDAV},
			routeSpec{pattern: method + " " + webDAVPath + "/", capabilities: capability, handler: (*Server).handleWebDAV},
		)
	}
	return routes
}
