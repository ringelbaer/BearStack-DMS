package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"bearstack/internal/photos"
)

func labelActor(r *http.Request) string {
	p, _ := authPrincipalFromContext(r.Context())
	return photos.LabelActor(p.Source, p.Subject, p.Username, p.AccountID)
}
func (s *Server) labelError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, photos.ErrLabelNameExists):
		status, code = 409, "name_exists"
	case errors.Is(err, photos.ErrLabelConflict):
		status, code = 409, "conflict"
	case errors.Is(err, photos.ErrLabelInvalid):
		status, code = 400, "invalid"
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, os.ErrNotExist):
		status, code = 404, "not_found"
	case errors.Is(err, photos.ErrAdminOnly()):
		status, code = 403, "forbidden"
	}
	_ = writeJSON(w, status, map[string]string{"error": publicErrorMessage(status, err.Error()), "code": code})
}
func labelInt(r *http.Request, key string, max int64) (int64, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, nil
	}
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil || n < 0 || n > max {
		return 0, photos.ErrLabelInvalid
	}
	return n, nil
}
func (s *Server) handleLabelSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	session, err := s.photos.LabelSession(r.Context())
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, struct {
		photos.LabelSession
		Account   string `json:"account"`
		CanManage bool   `json:"can_manage"`
	}{session, labelActor(r), true})
}

func (s *Server) handleLabelCandidates(w http.ResponseWriter, r *http.Request) {
	s.handleLabelList(w, r, false)
}

func (s *Server) handleLabelNamedPeople(w http.ResponseWriter, r *http.Request) {
	s.handleLabelList(w, r, true)
}

func (s *Server) handleLabelList(w http.ResponseWriter, r *http.Request, named bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	if !r.URL.Query().Has("upper") || r.URL.Query().Get("upper") == "" {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	after, err := labelInt(r, "after", 1<<62)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	upper, err := labelInt(r, "upper", 1<<62)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	var out photos.LabelCandidates
	if named {
		q := r.URL.Query().Get("q")
		if len(q) > 800 {
			s.labelError(w, r, photos.ErrLabelInvalid)
			return
		}
		out, err = s.photos.LabelNamedPeople(r.Context(), after, upper, q)
	} else {
		out, err = s.photos.LabelCandidates(r.Context(), after, upper)
	}
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, out)
}

func (s *Server) handleLabelSuggestions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	q := r.URL.Query().Get("q")
	if len(q) > 800 {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	out, err := s.photos.LabelSuggestions(r.Context(), q, r.URL.Query().Get("exact") == "1")
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, map[string]any{"people": out})
}

func (s *Server) handleLabelMergeSuggestion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	out, err := s.photos.LabelNextMergeSuggestion(r.Context())
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"suggestion": out})
}

func (s *Server) handleLabelReceipt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	operation := r.PathValue("operation")
	out, err := s.photos.LabelReceipt(r.Context(), labelActor(r), operation, r.URL.Query().Get("dataset"))
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, out)
}

func (s *Server) handleLabelAction(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var action photos.LabelAction
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&action); err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	setAuditTarget(r, "Person:"+strconv.FormatInt(id, 10))
	out, err := s.photos.ApplyLabelAction(r.Context(), labelActor(r), id, action)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, out)
}

func (s *Server) handleLabelThumbnail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	face, ok := s.labelingFace(w, r)
	if !ok {
		return
	}
	size, err := labelInt(r, "size", 640)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	if size == 0 {
		size = 160
	}
	if size != 160 && size != 640 {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	b, err := s.photos.FaceThumbnailSize(r.Context(), face.ID, int(size))
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}

func (s *Server) handleLabelOriginal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	face, ok := s.labelingFace(w, r)
	if !ok {
		return
	}
	s.servePhotoMedia(w, r, face.Path, photoMediaCacheNoStore)
}

func (s *Server) handleLabelPerson(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	offset, err := labelInt(r, "offset", 1<<30)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	limit, err := labelInt(r, "limit", 40)
	if err != nil || (r.URL.Query().Has("limit") && limit == 0) {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	if limit == 0 {
		limit = 4
	}
	after, err := labelInt(r, "after_face", 1<<62)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	var out photos.LabelPerson
	if r.URL.Query().Has("after_face") {
		out, err = s.photos.LabelPersonAfter(r.Context(), id, after, int(limit))
	} else {
		out, err = s.photos.LabelPerson(r.Context(), id, int(offset), int(limit))
	}
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, out)
}

func (s *Server) labelingFace(w http.ResponseWriter, r *http.Request) (photos.RecognizedFace, bool) {
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return photos.RecognizedFace{}, false
	}
	face, err := s.photos.Face(r.Context(), id)
	if err == nil && face.Ignored {
		err = sql.ErrNoRows
	}
	if err != nil {
		s.labelError(w, r, err)
		return photos.RecognizedFace{}, false
	}
	return face, true
}
