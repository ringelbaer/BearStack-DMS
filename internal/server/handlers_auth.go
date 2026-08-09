package server

import (
	"net/http"
	"net/url"
	"strconv"
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
		username := r.FormValue("username")
		password := r.FormValue("password")
		principal, ok, retryAfter := s.authenticateBasicCheck(username, password)
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds(retryAfter), 10))
			s.renderLogin(w, r, http.StatusTooManyRequests, "Zu viele Anmeldeversuche. Bitte versuchen Sie es später erneut.", returnURL)
			return
		}
		if !ok {
			s.renderLogin(w, r, http.StatusUnauthorized, "Login fehlgeschlagen. Bitte prüfen Sie Benutzername und Passwort.", returnURL)
			return
		}
		setAuditActor(r, principal.Username)
		setAuditTarget(r, "Benutzer:"+principal.Username)
		if truthy(r.FormValue("remember")) {
			if !s.setAuthSessionForPrincipal(w, r, principal, authRememberSessionDuration) {
				s.renderLogin(w, r, http.StatusUnauthorized, "Die Anmeldedaten wurden zwischenzeitlich geändert. Bitte melden Sie sich erneut an.", returnURL)
				return
			}
		} else {
			if !s.setAuthSessionForPrincipal(w, r, principal, authSessionDuration) {
				s.renderLogin(w, r, http.StatusUnauthorized, "Die Anmeldedaten wurden zwischenzeitlich geändert. Bitte melden Sie sich erneut an.", returnURL)
				return
			}
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
	if principal, ok := authPrincipalFromContext(r.Context()); ok {
		setAuditTarget(r, "Benutzer:"+principal.Username)
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
	if principal.hasAll(authCapSystemUsersManage) {
		return "/settings/users"
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
		if strings.HasPrefix(path, "/settings/users") {
			return principal.hasAll(authCapSystemUsersManage)
		}
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
	case path == "/account":
		return strings.TrimSpace(principal.Username) != ""
	case strings.HasPrefix(path, "/log"):
		return principal.hasAll(authCapSystemAudit)
	default:
		return true
	}
}
