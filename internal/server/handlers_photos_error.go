// Datei mappt Foto-Fehler auf HTTP-Antworten und transienten Thumbnail-Status.
package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"bearstack/internal/photos"
)

const (
	sqliteResultBusy   = 5
	sqliteResultLocked = 6
)

type sqliteCodeError interface {
	error
	Code() int
}

func photoSQLiteBusyError(err error) bool {
	var coded sqliteCodeError
	if !errors.As(err, &coded) {
		return false
	}
	switch coded.Code() & 0xff {
	case sqliteResultBusy, sqliteResultLocked:
		return true
	default:
		return false
	}
}

func (s *Server) renderPhotoBulkError(w http.ResponseWriter, r *http.Request, status int, err error) {
	if wantsJSONResponse(r) {
		s.renderJSONError(w, status, err.Error())
		return
	}
	s.renderErrorWithReturn(w, r, status, err, formReturnURL(r))
}

func (s *Server) renderPhotoError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, photos.ErrAdminOnly()) {
		s.renderForbidden(w, r)
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		s.renderError(w, r, http.StatusNotFound, errors.New("Foto nicht gefunden"))
		return
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "Fotomodul-Fehler"
	}
	if errors.Is(err, photos.ErrPathEscapesRoot()) {
		s.renderError(w, r, http.StatusBadRequest, errors.New("ungültiger Fotopfad"))
		return
	}
	if errors.Is(err, photos.ErrSearchTooBroad()) {
		s.renderError(w, r, http.StatusBadRequest, err)
		return
	}
	if errors.Is(err, photos.ErrThumbnailUnavailable()) {
		s.renderError(w, r, http.StatusNotFound, errors.New("kein Vorschaubild verfügbar"))
		return
	}
	s.renderError(w, r, http.StatusInternalServerError, fmt.Errorf("%s", message))
}
