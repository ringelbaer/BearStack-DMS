package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"bearstack/internal/account"
	"bearstack/internal/transfers"
)

type transferLogin struct {
	Connection string
	Revision   int64
	Session    string
	Polling    bool
	Actor      string
	State      json.RawMessage
	Expires    time.Time
}
type transferLoginState struct {
	mu      sync.Mutex
	pending map[string]transferLogin
}

var transferRoutes = []routeSpec{
	{pattern: "GET /settings/storage-connections", capabilities: authCapSystemManage, handler: (*Server).handleTransferPage},
	{pattern: "GET /photos/uploads", capabilities: authCapSystemManage, handler: (*Server).handleTransferPage},
	{pattern: "GET /api/transfers/v1/providers", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "GET /api/transfers/v1/connections", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "POST /api/transfers/v1/connections", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "PUT /api/transfers/v1/connections/{id}", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "POST /api/transfers/v1/connections/{id}/disconnect", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "POST /api/transfers/v1/connections/{id}/login", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "POST /api/transfers/v1/connections/{id}/login/{token}", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "GET /api/transfers/v1/connections/{id}/locations", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "POST /api/transfers/v1/connections/{id}/base", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "POST /api/transfers/v1/previews", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "GET /api/transfers/v1/jobs", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "GET /api/transfers/v1/jobs/{id}", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "GET /api/transfers/v1/jobs/{id}/items", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
	{pattern: "POST /api/transfers/v1/jobs/{id}/{action}", capabilities: authCapSystemManage, handler: (*Server).handleTransfers},
}

func (s *Server) isTransferAdmin(r *http.Request) bool {
	p, ok := authPrincipalFromContext(r.Context())
	return ok && account.IsAdministratorRole(p.Role)
}
func (s *Server) handleTransferPage(w http.ResponseWriter, r *http.Request) {
	if !s.isTransferAdmin(r) {
		s.renderForbidden(w, r)
		return
	}
	if s.transfers == nil {
		http.NotFound(w, r)
		return
	}
	active, title := "storage-connections", "Externe Speicher"
	if r.URL.Path == "/photos/uploads" {
		active, title = "uploads", "Upload-Warteschlange"
	}
	s.render(w, r, "transfers.html", PageData{Title: title, Active: active, SettingsTab: "storage-connections"})
}
func transferBody(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return transfers.Fail(transfers.Invalid, "Ungültige Eingabe")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return transfers.Fail(transfers.Invalid, "Ungültige Eingabe")
	}
	return nil
}
func transferSession(r *http.Request) string {
	identity := r.Header.Get("Authorization")
	if cookie, err := r.Cookie(authSessionCookieName); err == nil {
		identity = cookie.Value
	}
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}
func (s *Server) transferError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	switch transfers.Kind(err) {
	case transfers.Invalid:
		status = 400
	case transfers.Permission:
		status = 403
	case transfers.Authentication:
		status = 409
	case transfers.Conflict:
		status = 409
	case transfers.Throttled:
		status = 429
	case transfers.Quota:
		status = 507
	case transfers.Unsupported:
		status = 422
	}
	message := "Speicheranfrage fehlgeschlagen"
	var failure *transfers.Error
	if errors.As(err, &failure) {
		message = failure.Message
	}
	if errors.Is(err, sql.ErrNoRows) {
		status = 404
		message = "Nicht gefunden"
	}
	s.renderJSONError(w, status, message)
}
func (s *Server) handleTransfers(w http.ResponseWriter, r *http.Request) {
	if !s.isTransferAdmin(r) {
		s.renderJSONError(w, 403, "Administratorrechte erforderlich")
		return
	}
	if s.transfers == nil {
		s.renderJSONError(w, 404, "Fotomodul nicht verfügbar")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var result any
	var err error
	status := 200
	id := r.PathValue("id")
	principal, _ := authPrincipalFromContext(r.Context())
	actor, _ := json.Marshal(principal)
	switch r.Pattern {
	case "GET /api/transfers/v1/providers":
		result = s.transfers.Providers()
	case "GET /api/transfers/v1/connections":
		result, err = s.transfers.Connections(ctx)
	case "POST /api/transfers/v1/connections", "PUT /api/transfers/v1/connections/{id}":
		var in struct {
			Name     string          `json:"name"`
			Provider string          `json:"provider"`
			Enabled  bool            `json:"enabled"`
			Revision int64           `json:"revision"`
			Config   json.RawMessage `json:"config"`
		}
		err = transferBody(w, r, &in)
		if err != nil {
			break
		}
		c := transfers.Connection{ID: id, Name: strings.TrimSpace(in.Name), Provider: in.Provider, Enabled: in.Enabled, Revision: in.Revision, Config: in.Config}
		if id != "" {
			old, e := s.transfers.Connection(ctx, id)
			if e != nil {
				err = e
				break
			}
			c.Base = old.Base
		}
		result, err = s.transfers.SaveConnection(ctx, c)
	case "POST /api/transfers/v1/connections/{id}/disconnect":
		err = s.transfers.Disconnect(ctx, id)
		result = map[string]bool{"disconnected": err == nil}
	case "GET /api/transfers/v1/connections/{id}/locations":
		result, err = s.transfers.Locations(ctx, id, r.URL.Query().Get("parent"))
	case "POST /api/transfers/v1/connections/{id}/base":
		var in struct {
			Revision int64              `json:"revision"`
			Base     transfers.Location `json:"base"`
		}
		err = transferBody(w, r, &in)
		if err == nil {
			result, err = s.transfers.SetBase(ctx, id, in.Revision, in.Base)
		}
	case "POST /api/transfers/v1/connections/{id}/login":
		var login transfers.Login
		var revision int64
		login, revision, err = s.transfers.BeginLogin(ctx, id)
		if err != nil {
			break
		}
		token := transfers.ID()
		s.transferLogins.mu.Lock()
		if s.transferLogins.pending == nil {
			s.transferLogins.pending = map[string]transferLogin{}
		}
		for key, pending := range s.transferLogins.pending {
			if time.Now().After(pending.Expires) || pending.Connection == id {
				delete(s.transferLogins.pending, key)
			}
		}
		s.transferLogins.pending[token] = transferLogin{Connection: id, Revision: revision, Actor: string(actor), Session: transferSession(r), State: login.State, Expires: login.Expires}
		s.transferLogins.mu.Unlock()
		result = map[string]string{"login_url": login.URL, "token": token}
	case "POST /api/transfers/v1/connections/{id}/login/{token}":
		s.transferLogins.mu.Lock()
		token := r.PathValue("token")
		pending, ok := s.transferLogins.pending[token]
		if !ok || pending.Actor != string(actor) || pending.Session != transferSession(r) || pending.Connection != id || time.Now().After(pending.Expires) {
			s.transferLogins.mu.Unlock()
			err = transfers.Fail(transfers.Authentication, "Anmeldung abgelaufen; bitte neu verbinden")
			break
		}
		if pending.Polling {
			s.transferLogins.mu.Unlock()
			err = transfers.Fail(transfers.Conflict, "Anmeldung wird bereits geprüft")
			break
		}
		pending.Polling = true
		s.transferLogins.pending[token] = pending
		s.transferLogins.mu.Unlock()
		var done bool
		done, err = s.transfers.PollLogin(ctx, id, pending.Revision, pending.State)
		s.transferLogins.mu.Lock()
		if done || err != nil {
			delete(s.transferLogins.pending, token)
		} else if _, ok := s.transferLogins.pending[token]; ok {
			pending.Polling = false
			s.transferLogins.pending[token] = pending
		}
		s.transferLogins.mu.Unlock()
		result = map[string]bool{"connected": done}
	case "POST /api/transfers/v1/previews":
		var in struct {
			ConnectionID string              `json:"connection_id"`
			Selection    transfers.Selection `json:"selection"`
			Target       string              `json:"target"`
		}
		err = transferBody(w, r, &in)
		if err != nil {
			break
		}
		in.Selection.IncludeAdminOnly = s.requestPhotoAdminOnlyVisible(r)
		result, err = s.transfers.NewPreview(ctx, in.ConnectionID, string(actor), in.Selection, in.Target)
		status = 202
	case "GET /api/transfers/v1/jobs":
		result, err = s.transfers.Jobs(ctx, r.URL.Query().Get("connection"), parsePositiveInt(r.URL.Query().Get("page"), 1))
	case "GET /api/transfers/v1/jobs/{id}":
		result, err = s.transfers.Job(ctx, id)
	case "GET /api/transfers/v1/jobs/{id}/items":
		result, err = s.transfers.Items(ctx, id, int64(parsePositiveInt(r.URL.Query().Get("after"), 0)))
	case "POST /api/transfers/v1/jobs/{id}/{action}":
		err = s.transfers.Action(ctx, id, r.PathValue("action"))
		if err == nil {
			result, err = s.transfers.Job(ctx, id)
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.transferError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}
func (s *Server) runTransfers(ctx context.Context) {
	if s.transfers != nil {
		s.transfers.Run(ctx)
	}
}
func (s *Server) transferView(r *http.Request, data *PageData) {
	data.TransferAdmin = s.photos != nil && s.isTransferAdmin(r)
	if !data.TransferAdmin || s.transfers == nil {
		return
	}
	connections, err := s.transfers.Connections(r.Context())
	if err != nil {
		return
	}
	provider := ""
	label := ""
	mixed := false
	for _, c := range connections {
		if c.Enabled {
			data.TransferEnabled = true
		}
		if !c.Enabled || !c.Connected || c.Base.ID == "" {
			continue
		}
		data.TransferReady = true
		if provider != "" && provider != c.Provider {
			mixed = true
		}
		provider = c.Provider
		for _, p := range s.transfers.Providers() {
			if p.ID == c.Provider {
				label = p.UploadLabel
			}
		}
	}
	if mixed {
		label = "Upload zu Speicher …"
	}
	data.TransferLabel = label
	if data.Active != "photos" || !data.TransferReady {
		return
	}
	sel := transfers.Selection{Path: r.URL.Query().Get("path"), Query: r.URL.Query().Get("q"), MediaType: r.URL.Query().Get("type"), GPSOnly: truthy(r.URL.Query().Get("gps"))}
	if strings.HasPrefix(sel.Path, ".people") {
		data.TransferUpload = len(strings.Split(sel.Path, "/")) == 3
	} else {
		data.TransferUpload = strings.TrimSpace(sel.Query) != "" || len(strings.Split(sel.Path, "/")) >= 2
	}
	raw, _ := json.Marshal(sel)
	data.TransferSelection = string(raw)
}
