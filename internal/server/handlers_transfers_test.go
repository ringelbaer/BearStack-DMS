package server

import (
	"bearstack/internal/testutil/apicontract"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bearstack/internal/config"
	"bearstack/internal/transfers"
)

func transferTestServer(t *testing.T) *Server {
	t.Helper()
	data := t.TempDir()
	cfg := config.Config{DataDir: data, Photos: config.PhotoConfig{Enabled: true, RootDir: t.TempDir(), CacheDir: filepath.Join(data, "cache"), DBPath: filepath.Join(data, "photos.db"), DataDir: filepath.Join(data, "photos")}, Auth: config.AuthConfig{Credentials: []config.AuthCredential{{Username: "admin", Password: "secret", Role: "admin"}, {Username: "manager", Password: "secret", Role: "photos_manager"}, {Username: "reader", Password: "secret", Role: "photos_read"}, {Username: "system-manager", Password: "secret", Role: "photos_read", Permissions: []string{"system.manage"}}}}}
	s := newAuthSecurityServer(t, cfg, openAuthSecurityRepository(t))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := s.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return s
}
func TestTransferRoutesAdministratorAndCSRF(t *testing.T) {
	s := transferTestServer(t)
	for _, route := range transferRoutes {
		method, pathname, _ := strings.Cut(route.pattern, " ")
		pathname = strings.NewReplacer("{id}", "missing", "{token}", "missing", "{action}", "start").Replace(pathname)
		for _, user := range []string{"reader", "manager", "system-manager"} {
			r := httptest.NewRequest(method, pathname, strings.NewReader(`{}`))
			r.SetBasicAuth(user, "secret")
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("%s %s: %d", route.pattern, user, w.Code)
			}
		}
		if method != "GET" {
			r := httptest.NewRequest(method, pathname, strings.NewReader(`{}`))
			r.AddCookie(sessionCookieForUser(t, s, "admin"))
			r.Header.Set("Origin", "https://attacker.invalid")
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("CSRF accepted %s: %d", route.pattern, w.Code)
			}
		}
	}
	for _, pathname := range []string{"/settings/storage-connections", "/photos/uploads", "/api/transfers/v1/providers", "/api/transfers/v1/connections"} {
		r := httptest.NewRequest("GET", pathname, nil)
		r.SetBasicAuth("admin", "secret")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("admin %s: %d %s", pathname, w.Code, w.Body.String())
		}
	}
	create := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/transfers/v1/connections", strings.NewReader(body))
		r.AddCookie(sessionCookieForUser(t, s, "admin"))
		r.Header.Set("Origin", "http://example.com")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	w := create(`{"name":"Archiv","provider":"nextcloud","enabled":false,"revision":0,"config":{"server":"https://cloud.example.test"}}`)
	if w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	var c transfers.Connection
	if err := json.Unmarshal(w.Body.Bytes(), &c); err != nil || c.ID == "" || c.Connected {
		t.Fatalf("connection %+v %v", c, err)
	}
	for _, body := range []string{`{} {}`, `{"name":"Archiv","provider":"nextcloud","config":{"server":"http://cloud.test"}}`, `{"name":"Archiv","provider":"nextcloud","password":"secret","config":{"server":"https://cloud.test"}}`} {
		if w = create(body); w.Code != 400 {
			t.Fatalf("bad input %s: %d", body, w.Code)
		}
	}
	p, _ := s.authenticateBasic("admin", "secret")
	actor, _ := json.Marshal(p)
	if !s.transferAdministrator(context.Background(), string(actor)) {
		t.Fatal("valid admin rejected")
	}
	p.Role = "photos_manager"
	p.Revision = "obsolete"
	actor, _ = json.Marshal(p)
	if s.transferAdministrator(context.Background(), string(actor)) {
		t.Fatal("stale actor accepted")
	}
}
func TestTransferLoginBoundToSessionAndJSONDoesNotExposeCredentials(t *testing.T) {
	s := transferTestServer(t)
	r := httptest.NewRequest("POST", "/api/transfers/v1/connections/missing/login/token", nil)
	r.SetBasicAuth("admin", "secret")
	p, _ := s.authenticateBasic("admin", "secret")
	actor, _ := json.Marshal(p)
	s.transferLogins.pending = map[string]transferLogin{"token": {Connection: "missing", Actor: string(actor), Session: "another-session", Expires: time.Now().Add(time.Minute), State: json.RawMessage(`{"token":"secret-poll"}`)}}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 409 || strings.Contains(w.Body.String(), "secret-poll") {
		t.Fatalf("session boundary %d %s", w.Code, w.Body.String())
	}
}

func TestOpenAPITransferResponses(t *testing.T) {
	raw, err := os.ReadFile("../../openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := apicontract.Load(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := transferTestServer(t)
	cases := []struct {
		method, path, canonical, body string
		status                        int
	}{
		{"GET", "/api/transfers/v1/providers", "/api/transfers/v1/providers", "", 200},
		{"GET", "/api/transfers/v1/connections", "/api/transfers/v1/connections", "", 200},
		{"POST", "/api/transfers/v1/connections", "/api/transfers/v1/connections", `{"name":"Archiv","provider":"nextcloud","enabled":false,"config":{"server":"https://cloud.example.org"}}`, 200},
		{"GET", "/api/transfers/v1/jobs", "/api/transfers/v1/jobs", "", 200},
		{"GET", "/api/transfers/v1/jobs/missing", "/api/transfers/v1/jobs/{id}", "", 404},
		{"POST", "/api/transfers/v1/previews", "/api/transfers/v1/previews", "{}", 404},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.SetBasicAuth("admin", "secret")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if err := contract.ValidateResponse(tc.canonical, tc.method, w.Code, w.Header(), w.Body.Bytes()); err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
	}
}
