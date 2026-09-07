package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

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
	case errors.Is(err, sql.ErrNoRows):
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
func (s *Server) handleLabeling(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	base := "/api/photos/labeling/v1"
	switch r.URL.Path {
	case base + "/session":
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
		return
	case base + "/candidates":
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
		out, err := s.photos.LabelCandidates(r.Context(), after, upper)
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		_ = writeJSON(w, 200, out)
		return
	case base + "/suggestions":
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
		return
	}
	if operation := r.PathValue("operation"); operation != "" {
		out, err := s.photos.LabelReceipt(r.Context(), labelActor(r), operation, r.URL.Query().Get("dataset"))
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		_ = writeJSON(w, 200, out)
		return
	}
	id, err := faceID(r.PathValue("id"))
	if err != nil {
		s.labelError(w, r, photos.ErrLabelInvalid)
		return
	}
	if r.Method == http.MethodPost {
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
		return
	}
	if strings.HasSuffix(r.URL.Path, "/thumbnail") {
		size, err := labelInt(r, "size", 640)
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		if size == 0 {
			size = 160
		}
		face, err := s.photos.Face(r.Context(), id)
		if err == nil && face.Ignored {
			err = sql.ErrNoRows
		}
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		if size != 160 && size != 640 {
			s.labelError(w, r, photos.ErrLabelInvalid)
			return
		}
		b, err := s.photos.FaceThumbnailSize(r.Context(), id, int(size))
		if err != nil {
			s.labelError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(b)
		return
	}
	offset, err := labelInt(r, "offset", 1<<30)
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	out, err := s.photos.LabelPerson(r.Context(), id, int(offset))
	if err != nil {
		s.labelError(w, r, err)
		return
	}
	_ = writeJSON(w, 200, out)
}
