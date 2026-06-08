package server

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) renderLogin(w http.ResponseWriter, r *http.Request, status int, errorText, returnURL string) {
	data := PageData{
		Title:     "Anmelden",
		ReturnURL: safeReturnURL(returnURL),
		Error:     errorText,
	}
	if status == 0 {
		status = http.StatusUnauthorized
	}
	if status > 0 {
		w.WriteHeader(status)
	}
	s.render(w, r, "login.html", data)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	returnURL := formReturnURL(r)
	switch r.Method {
	case http.MethodGet:
		s.renderLogin(w, r, http.StatusOK, "", returnURL)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			s.renderLogin(w, r, http.StatusBadRequest, "Ungültige Formulardaten.", returnURL)
			return
		}
		returnURL = safeReturnURL(r.FormValue("return"))
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")
		principal, ok := s.authenticateBasic(username, password)
		if !ok {
			s.renderLogin(w, r, http.StatusUnauthorized, "Login fehlgeschlagen. Bitte prüfen Sie Benutzername und Passwort.", returnURL)
			return
		}
		if truthy(r.FormValue("remember")) {
			s.setRememberedAuthSession(w, r, principal.Username)
		} else {
			s.setAuthSession(w, r, principal.Username)
		}
		if returnURL == "/" || !s.loginReturnAllowed(principal, returnURL) {
			returnURL = defaultAuthLandingURL(principal)
			if !s.loginReturnAllowed(principal, returnURL) {
				returnURL = "/help"
			}
		}
		redirect(w, r, returnURL)
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	clearAuthSession(w, r)
	redirect(w, r, "/login")
}

func defaultAuthLandingURL(principal authPrincipal) string {
	if principal.hasAll(authCapDocumentsRead) {
		return "/"
	}
	if principal.hasAny(authCapPhotosRead | authCapPhotosEdit | authCapPhotosManage) {
		return "/photos"
	}
	if principal.hasAny(authCapSystemManage | authCapSystemAudit) {
		return "/settings"
	}
	return "/help"
}

func (s *Server) loginReturnAllowed(principal authPrincipal, target string) bool {
	path := target
	if parsed, err := url.Parse(target); err == nil {
		path = parsed.Path
	}
	if path == "" {
		path = "/"
	}
	switch {
	case path == "/":
		if s.photos == nil {
			return principal.hasAny(authCapDocumentsRead)
		}
		return principal.hasAny(authCapDocumentsRead | authCapPhotosRead)
	case strings.HasPrefix(path, "/static/") ||
		path == "/favicon.ico" ||
		path == "/favicon/custom" ||
		path == "/help":
		return true
	case strings.HasPrefix(path, "/photos"):
		return s.photos != nil && principal.hasAny(authCapPhotosRead)
	case strings.HasPrefix(path, "/documents"):
		return principal.hasAll(authCapDocumentsRead)
	case strings.HasPrefix(path, "/settings"):
		if strings.HasPrefix(path, "/settings/photos") {
			return s.photos != nil && principal.hasAny(authCapPhotosManage)
		}
		return principal.hasAll(authCapSystemManage)
	case strings.HasPrefix(path, "/api"):
		return principal.hasAny(authCapDocumentsRead | authCapDocumentsUpload | authCapDocumentsWebDAVRead | authCapPhotosRead)
	case path == "/webdav" || strings.HasPrefix(path, "/webdav/"):
		return principal.hasAny(authCapDocumentsWebDAVRead)
	case strings.HasPrefix(path, "/.well-known/webdav"):
		return true
	case path == "/healthz" || path == "/login":
		return true
	case strings.HasPrefix(path, "/log"):
		return principal.hasAll(authCapSystemAudit)
	default:
		return true
	}
}
