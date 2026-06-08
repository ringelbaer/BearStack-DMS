// Datei registriert HTTP-Routen und verbindet Pfade mit den passenden Handlern.
package server

import "net/http"

type routeHandler func(*Server, http.ResponseWriter, *http.Request)

type routeSpec struct {
	pattern      string
	capabilities authCapabilities
	requireAny   bool
	handler      routeHandler
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerCoreRoutes(mux)
	s.registerDocumentRoutes(mux)
	s.registerSettingsRoutes(mux)
	s.registerPhotoRoutes(mux)
	s.registerWebDAVRoutes(mux)

	return s.securityHeaders(s.basicAuth(s.sameOriginUnsafeRequests(s.auditWriteActions(mux))))
}

func (s *Server) registerRouteSpecs(mux *http.ServeMux, routes []routeSpec) {
	for _, route := range routes {
		handler := route.handlerFunc(s)
		if route.requireAny {
			mux.HandleFunc(route.pattern, s.requireAny(route.capabilities, handler))
			continue
		}
		mux.HandleFunc(route.pattern, s.require(route.capabilities, handler))
	}
}

func (r routeSpec) handlerFunc(s *Server) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.handler(s, w, req)
	}
}

func (s *Server) registerCoreRoutes(mux *http.ServeMux) {
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.static))
	s.registerRouteSpecs(mux, coreRouteSpecs)
}

func (s *Server) registerSettingsRoutes(mux *http.ServeMux) {
	s.registerRouteSpecs(mux, settingsRouteSpecs)
	if s.photos != nil {
		s.registerRouteSpecs(mux, photoSettingsRouteSpecs)
	}
}

func (s *Server) registerDocumentRoutes(mux *http.ServeMux) {
	s.registerRouteSpecs(mux, homeRouteSpecs(s.photos != nil))
	s.registerRouteSpecs(mux, documentRouteSpecs)
}

func (s *Server) registerPhotoRoutes(mux *http.ServeMux) {
	if s.photos == nil {
		return
	}
	s.registerRouteSpecs(mux, photoRouteSpecs)
}

func (s *Server) registerWebDAVRoutes(mux *http.ServeMux) {
	s.registerRouteSpecs(mux, webDAVWellKnownRouteSpecs)
	s.registerRouteSpecs(mux, webDAVRouteSpecs(s.webDAVPath()))
}
