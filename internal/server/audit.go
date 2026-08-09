// Datei erstellt Audit-Eintraege aus Serveraktionen und verbindet Handler mit dem Audit-Repository.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"bearstack/internal/document"
)

const auditLogRetention = 30 * 24 * time.Hour

const (
	auditRejectionLimit  = 60
	auditRejectionWindow = time.Minute
)

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

type auditRequestData struct {
	target   string
	actor    string
	recorded bool
}

type auditRequestDataKey struct{}

type auditRejectionLimiter struct {
	mu          sync.Mutex
	windowStart time.Time
	events      int
	now         func() time.Time
}

func (w *auditResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (w *auditResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *auditResponseWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (s *Server) auditWriteActions(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isWriteMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		auditData, r := requestAuditData(r)
		recorder := &auditResponseWriter{ResponseWriter: w}
		occurredAt := time.Now().UTC()
		next.ServeHTTP(recorder, r)
		if auditData.recorded {
			return
		}
		auditData.recorded = true
		status := recorder.statusCode()
		if status >= http.StatusBadRequest && isAccountAuditPattern(r.Pattern) && !s.earlyAudit.allow() {
			return
		}

		entry := auditLogEntryForRequest(r, status, occurredAt)
		if actor := strings.TrimSpace(auditData.actor); actor != "" {
			entry.Actor = actor
		}
		if target := strings.TrimSpace(auditData.target); target != "" {
			entry.Target = target
		}
		s.recordAuditLog(r.Context(), entry)
	})
}

// auditRejectedAccountActions covers account mutations that authentication or
// same-origin checks reject before they reach auditWriteActions. It deliberately
// limits the outer audit layer to account routes: arbitrary unauthenticated
// writes must not turn the audit database into an amplification primitive.
func (s *Server) auditRejectedAccountActions(mux *http.ServeMux, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isWriteMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		_, pattern := mux.Handler(r)
		if !isAccountAuditPattern(pattern) {
			next.ServeHTTP(w, r)
			return
		}

		auditData, r := requestAuditData(r)
		recorder := &auditResponseWriter{ResponseWriter: w}
		occurredAt := time.Now().UTC()
		next.ServeHTTP(recorder, r)
		if auditData.recorded {
			return
		}
		auditData.recorded = true
		if !s.earlyAudit.allow() {
			return
		}

		auditRequest := r.Clone(r.Context())
		auditRequest.Pattern = pattern
		entry := auditLogEntryForRequest(auditRequest, recorder.statusCode(), occurredAt)
		if principal, ok := s.authSessionPrincipal(r); ok {
			entry.Actor = principal.Username
		}
		if target := s.rejectedAccountAuditTarget(r, pattern, entry.Actor); target != "" {
			entry.Target = target
		}
		s.recordAuditLog(r.Context(), entry)
	})
}

func requestAuditData(r *http.Request) (*auditRequestData, *http.Request) {
	if auditData, ok := r.Context().Value(auditRequestDataKey{}).(*auditRequestData); ok && auditData != nil {
		return auditData, r
	}
	auditData := &auditRequestData{}
	return auditData, r.WithContext(context.WithValue(r.Context(), auditRequestDataKey{}, auditData))
}

func (limiter *auditRejectionLimiter) allow() bool {
	if limiter == nil {
		return false
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := time.Now()
	if limiter.now != nil {
		now = limiter.now()
	}
	if limiter.windowStart.IsZero() || !now.Before(limiter.windowStart.Add(auditRejectionWindow)) {
		limiter.windowStart = now
		limiter.events = 0
	}
	if limiter.events >= auditRejectionLimit {
		return false
	}
	limiter.events++
	return true
}

func isAccountAuditPattern(pattern string) bool {
	switch pattern {
	case "POST /login",
		"POST /logout",
		"POST /settings/users",
		"POST /settings/users/{id}",
		"POST /settings/users/{id}/password",
		"POST /settings/users/{id}/enable",
		"POST /settings/users/{id}/disable",
		"POST /settings/users/{id}/delete",
		"POST /account/password":
		return true
	default:
		return false
	}
}

func (s *Server) rejectedAccountAuditTarget(r *http.Request, pattern, actor string) string {
	if pattern == "POST /account/password" || pattern == "POST /logout" {
		if actor != "" && actor != "anonymous" {
			return "Benutzer:" + actor
		}
		return ""
	}
	if pattern == "POST /login" || pattern == "POST /settings/users" {
		// The target username exists only in the rejected form body. Password
		// fields and form data are intentionally never parsed by audit code.
		return ""
	}
	remainder := strings.TrimPrefix(r.URL.EscapedPath(), "/settings/users/")
	rawID := strings.SplitN(remainder, "/", 2)[0]
	decodedID, err := url.PathUnescape(rawID)
	if err != nil {
		return "Benutzer"
	}
	numericID, err := strconv.ParseInt(decodedID, 10, 64)
	if err != nil || numericID < 1 {
		return "Benutzer"
	}
	id := strconv.FormatInt(numericID, 10)
	if snapshot := s.authSnapshot(); snapshot != nil {
		if credential := snapshot.bySubject[authSubjectKey(authSourceDatabase, id)]; credential != nil {
			return "Benutzer:" + credential.username
		}
	}
	return "Benutzer-ID:" + id
}

func setAuditTarget(r *http.Request, target string) {
	auditData, ok := r.Context().Value(auditRequestDataKey{}).(*auditRequestData)
	if !ok || auditData == nil {
		return
	}
	auditData.target = strings.TrimSpace(target)
}

func setAuditActor(r *http.Request, actor string) {
	auditData, ok := r.Context().Value(auditRequestDataKey{}).(*auditRequestData)
	if !ok || auditData == nil {
		return
	}
	auditData.actor = strings.TrimSpace(actor)
}

func (s *Server) recordAuditLog(ctx context.Context, entry document.AuditLogEntry) {
	if s == nil || s.repo == nil {
		return
	}
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = time.Now().UTC()
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := s.repo.SaveAuditLog(ctx, entry); err != nil {
		if s.log != nil {
			s.log.Warn("failed to save audit log", "error", err)
		}
		return
	}
	if err := s.repo.PruneAuditLogs(ctx, entry.OccurredAt.Add(-auditLogRetention)); err != nil {
		if s.log != nil {
			s.log.Warn("failed to prune audit log", "error", err)
		}
	}
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if err := s.repo.PruneAuditLogs(r.Context(), time.Now().UTC().Add(-auditLogRetention)); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}

	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	entries, err := s.repo.ListAuditLogs(r.Context(), pageSize+1, (page-1)*pageSize)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err)
		return
	}
	hasNext := len(entries) > pageSize
	if hasNext {
		entries = entries[:pageSize]
	}

	s.render(w, r, "audit_log.html", PageData{
		Title:      "Log",
		Active:     "log",
		AuditLogs:  entries,
		Pagination: paginationData(r, page, hasNext),
	})
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func auditLogEntryForRequest(r *http.Request, status int, occurredAt time.Time) document.AuditLogEntry {
	action, target := auditActionTarget(r)
	return document.AuditLogEntry{
		OccurredAt: occurredAt,
		Actor:      auditActor(r),
		Method:     r.Method,
		Path:       auditPath(r),
		Route:      r.Pattern,
		Action:     action,
		Target:     target,
		Status:     status,
		RemoteAddr: auditRemoteAddr(r.RemoteAddr),
		UserAgent:  r.UserAgent(),
	}
}

func auditActor(r *http.Request) string {
	if principal, ok := authPrincipalFromContext(r.Context()); ok {
		return principal.Username
	}
	if user, ok := r.Context().Value(authActorContextKey{}).(string); ok {
		user = strings.TrimSpace(user)
		if user != "" {
			return user
		}
	}
	user, _, ok := r.BasicAuth()
	user = strings.TrimSpace(user)
	if ok && user != "" {
		return user
	}
	return "anonymous"
}

func auditPath(r *http.Request) string {
	path := r.URL.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func auditRemoteAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	return remoteAddr
}

func auditActionTarget(r *http.Request) (string, string) {
	if r.Method == http.MethodPut {
		return "Dokument per WebDAV hochladen", ""
	}
	switch r.Pattern {
	case "POST /login":
		return "Anmeldung", ""
	case "POST /logout":
		return "Abmeldung", ""
	case "POST /tags":
		return "Tag speichern", ""
	case "POST /tags/{id}":
		return "Tag aktualisieren", tagAuditTarget(r)
	case "POST /tags/{id}/delete":
		return "Tag löschen", tagAuditTarget(r)
	case "POST /tags/{id}/rules":
		return "Tag-Regeln speichern", tagAuditTarget(r)
	case "POST /search-favorites":
		return "Suchfavorit anlegen", ""
	case "POST /search-favorites/{id}":
		return "Suchfavorit aktualisieren", searchFavoriteAuditTarget(r)
	case "POST /search-favorites/{id}/delete":
		return "Suchfavorit löschen", searchFavoriteAuditTarget(r)
	case "POST /fields":
		return "Feld anlegen", ""
	case "POST /fields/{id}":
		return "Feld aktualisieren", fieldAuditTarget(r)
	case "POST /fields/{id}/autocomplete":
		return "Feld-Auto-Vervollständigung speichern", fieldAuditTarget(r)
	case "POST /fields/{id}/values":
		return "Feldwert ändern", fieldAuditTarget(r)
	case "POST /fields/{id}/delete":
		return "Feld löschen", fieldAuditTarget(r)
	case "POST /settings/columns":
		return "Spalten speichern", ""
	case "POST /settings/page-size":
		return "Seitengröße speichern", ""
	case "POST /settings":
		return "Einstellungen speichern", ""
	case "POST /settings/favicon":
		return "Favicon speichern", ""
	case "POST /settings/favicon/reset":
		return "Favicon zurücksetzen", ""
	case "POST /settings/mail-import":
		return "E-Mail-Import-Einstellungen speichern", ""
	case "POST /settings/mail-import/test":
		return "E-Mail-Import-Einstellungen prüfen", ""
	case "POST /settings/mail-import/run":
		return "E-Mails manuell abrufen", ""
	case "POST /settings/users":
		return "Benutzer anlegen", ""
	case "POST /settings/users/{id}":
		return "Benutzerrechte ändern", ""
	case "POST /settings/users/{id}/password":
		return "Benutzerpasswort zurücksetzen", ""
	case "POST /settings/users/{id}/enable":
		return "Benutzer aktivieren", ""
	case "POST /settings/users/{id}/disable":
		return "Benutzer deaktivieren", ""
	case "POST /settings/users/{id}/delete":
		return "Benutzer löschen", ""
	case "POST /account/password":
		return "Eigenes Passwort ändern", ""
	case "POST /upload":
		return "Dokumente hochladen", ""
	case "POST /api/upload":
		return "Dokumente per API hochladen", ""
	case "POST /documents/link":
		return "Dokumente verknüpfen", ""
	case "POST /documents/tags/add":
		return "Dokument-Tags ergänzen", ""
	case "POST /documents/tags/remove":
		return "Dokument-Tags entfernen", ""
	case "POST /documents/fields":
		return "Dokument-Felder setzen", ""
	case "POST /documents/{id}/links/{linkedID}/delete":
		return "Dokument-Verknüpfung aufheben", documentLinkAuditTarget(r)
	case "POST /documents/{id}/metadata":
		return "Dokument-Metadaten speichern", documentAuditTarget(r)
	case "POST /documents/{id}/document-date":
		return "Dokument-Dateidatum speichern", documentAuditTarget(r)
	case "POST /documents/{id}/tags":
		return "Dokument-Tags speichern", documentAuditTarget(r)
	case "POST /documents/{id}/ocr/{lang}":
		target := documentAuditTarget(r)
		if lang := strings.TrimSpace(r.PathValue("lang")); lang != "" {
			target += ", OCR " + lang
		}
		return "OCR starten", target
	case "POST /statistics/text-issues/ocr/{lang}":
		target := "OCR-Kandidaten"
		if lang := strings.TrimSpace(r.PathValue("lang")); lang != "" {
			target += ", OCR " + lang
		}
		return "OCR fuer Textprobleme starten", target
	case "POST /documents/{id}/delete":
		return "Dokument in Papierkorb verschieben", documentAuditTarget(r)
	case "POST /documents/{id}/restore":
		return "Dokument wiederherstellen", documentAuditTarget(r)
	case "POST /documents/{id}/purge":
		return "Dokument endgültig löschen", documentAuditTarget(r)
	case "POST /trash/empty":
		return "Papierkorb leeren", ""
	case "POST /photos/tags":
		return "Foto-Tags speichern", ""
	default:
		return strings.TrimSpace(r.Method + " " + auditPath(r)), ""
	}
}

func documentAuditTarget(r *http.Request) string {
	return idAuditTarget("Dokument", r.PathValue("id"))
}

func documentAuditTargetFor(doc document.Document) string {
	return namedAuditTarget("Dokument", doc.ID, documentAuditTitle(doc))
}

func documentAuditTargetsFor(docs []document.Document) string {
	if len(docs) == 0 {
		return "0 Dokumente"
	}
	const maxNamedDocuments = 3
	parts := make([]string, 0, min(len(docs), maxNamedDocuments)+1)
	for i, doc := range docs {
		if i >= maxNamedDocuments {
			parts = append(parts, fmt.Sprintf("+%d weitere", len(docs)-maxNamedDocuments))
			break
		}
		parts = append(parts, documentAuditTargetFor(doc))
	}
	return strings.Join(parts, ", ")
}

func documentAuditTitle(doc document.Document) string {
	title := strings.TrimSpace(doc.Title)
	if title != "" {
		return title
	}
	return doc.OriginalName
}

func documentLinkAuditTarget(r *http.Request) string {
	return idAuditTarget("Dokument", r.PathValue("id")) + " und " + idAuditTarget("Dokument", r.PathValue("linkedID"))
}

func tagAuditTarget(r *http.Request) string {
	return idAuditTarget("Tag", r.PathValue("id"))
}

func tagAuditTargetFor(tag document.Tag) string {
	return namedAuditTarget("Tag", tag.ID, tag.Name)
}

func fieldAuditTarget(r *http.Request) string {
	return idAuditTarget("Feld", r.PathValue("id"))
}

func idAuditTarget(kind, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return kind
	}
	return kind + " #" + id
}

func namedAuditTarget(kind string, id int64, title string) string {
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if title == "" {
		if id <= 0 {
			return kind
		}
		return fmt.Sprintf("%s #%d", kind, id)
	}
	if id <= 0 {
		return fmt.Sprintf("%s %q", kind, title)
	}
	return fmt.Sprintf("%s %q (#%d)", kind, title, id)
}
