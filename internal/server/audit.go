// Datei erstellt Audit-Eintraege aus Serveraktionen und verbindet Handler mit dem Audit-Repository.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"bearstack/internal/document"
)

const auditLogRetention = 30 * 24 * time.Hour

type auditResponseWriter struct {
	http.ResponseWriter
	status int
}

type auditRequestData struct {
	target string
}

type auditRequestDataKey struct{}

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

		auditData := &auditRequestData{}
		r = r.WithContext(context.WithValue(r.Context(), auditRequestDataKey{}, auditData))
		recorder := &auditResponseWriter{ResponseWriter: w}
		occurredAt := time.Now().UTC()
		next.ServeHTTP(recorder, r)

		entry := auditLogEntryForRequest(r, recorder.statusCode(), occurredAt)
		if target := strings.TrimSpace(auditData.target); target != "" {
			entry.Target = target
		}
		s.recordAuditLog(r.Context(), entry)
	})
}

func setAuditTarget(r *http.Request, target string) {
	auditData, ok := r.Context().Value(auditRequestDataKey{}).(*auditRequestData)
	if !ok || auditData == nil {
		return
	}
	auditData.target = strings.TrimSpace(target)
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
