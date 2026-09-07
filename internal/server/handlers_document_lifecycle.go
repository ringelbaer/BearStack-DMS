// Datei behandelt Lebenszyklusaktionen fuer Dokumente wie Anlegen, Bearbeiten, Archivieren und Entfernen.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"bearstack/internal/repository"
)

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	doc, err := s.documentFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if doc.DeleteProtected {
		s.renderErrorWithReturn(w, r, http.StatusForbidden, errors.New("Dokument ist durch ein Tag vor dem Löschen geschützt."), formReturnURL(r))
		return
	}
	if err := s.repo.SoftDelete(r.Context(), doc.ID); err != nil {
		if errors.Is(err, repository.ErrDeleteProtected) {
			s.renderErrorWithReturn(w, r, http.StatusForbidden, errors.New("Dokument ist durch ein Tag vor dem Löschen geschützt."), formReturnURL(r))
			return
		}
		s.renderHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	setAuditTarget(r, documentAuditTargetFor(doc))
	returnURL := formReturnURL(r)
	if returnURL == "" {
		returnURL = "/"
	}
	redirectWithNotice(w, r, returnURL, "Dokument in den Papierkorb verschoben.")
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	if err := s.repo.Restore(r.Context(), id); err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	if doc, err := s.repo.GetDocument(r.Context(), id); err == nil {
		setAuditTarget(r, documentAuditTargetFor(doc))
	}
	redirect(w, r, documentURL(id, formReturnURL(r), "Dokument wiederhergestellt."))
}

func (s *Server) handlePurge(w http.ResponseWriter, r *http.Request) {
	id, err := idFromRequest(r)
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	doc, err := s.repo.Purge(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrDeleteProtected) {
			s.renderErrorWithReturn(w, r, http.StatusForbidden, errors.New("Dokument ist durch ein Tag vor dem Löschen geschützt."), formReturnURL(r))
			return
		}
		s.renderHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	setAuditTarget(r, documentAuditTargetFor(doc))
	if err := s.trashService().DeletePurgedDocumentFiles(r.Context(), doc.ID); err != nil {
		logWarn(s.log, "purged files queued for retry", "id", doc.ID, "error", err)
		redirectWithNotice(w, r, "/trash", "Dokument gelöscht. Die Dateibereinigung wird im Hintergrund wiederholt.")
		return
	}
	redirectWithNotice(w, r, "/trash", "Dokument endgültig gelöscht.")
}

func (s *Server) handleEmptyTrash(w http.ResponseWriter, r *http.Request) {
	docs, err := s.repo.PurgeTrash(r.Context())
	if err != nil {
		s.renderHTTPError(w, r, err)
		return
	}
	s.invalidateDocumentCountCache()
	pending := 0
	for _, doc := range docs {
		if err := s.trashService().DeletePurgedDocumentFiles(r.Context(), doc.ID); err != nil {
			pending++
			logWarn(s.log, "purged files queued for retry", "id", doc.ID, "error", err)
		}
	}
	setAuditTarget(r, documentAuditTargetsFor(docs))
	if len(docs) == 0 {
		redirectWithNotice(w, r, "/trash", "Keine löschbaren Dokumente im Papierkorb. Geschützte Dokumente bleiben erhalten.")
		return
	}
	notice := fmt.Sprintf("%d Dokument(e) endgültig gelöscht.", len(docs))
	if pending > 0 {
		notice = fmt.Sprintf("%d Dokument(e) gelöscht. Die Dateibereinigung für %d Dokument(e) wird im Hintergrund wiederholt.", len(docs), pending)
	}
	redirectWithNotice(w, r, "/trash", notice)
}

func (s *Server) purgeTrashByRetention(ctx context.Context) (int, error) {
	return s.trashService().PurgeByRetention(ctx)
}
