package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"bearstack/internal/config"

	"golang.org/x/crypto/bcrypt"
)

func TestBasicAuthAcceptsPasswordHash(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("hash-secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username:     "admin",
				Password:     "clear-secret",
				PasswordHash: string(hash),
				Realm:        "BearStack",
			},
		},
		authKey: []byte("01234567890123456789012345678901"),
	}
	called := 0
	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if actor := auditActor(r); actor != "admin" {
			t.Fatalf("actor = %q", actor)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "hash-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != authSessionCookieName {
		t.Fatalf("session cookie = %#v", cookies)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("session status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "clear-secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if called != 2 {
		t.Fatalf("handler calls = %d", called)
	}
}

func TestNewRejectsInvalidPasswordHash(t *testing.T) {
	_, err := New(config.Config{
		Auth: config.AuthConfig{
			Username:     "admin",
			PasswordHash: "not-a-bcrypt-hash",
			Realm:        "BearStack",
		},
	}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err == nil {
		t.Fatal("expected invalid password hash error")
	}
	if !strings.Contains(err.Error(), "invalid auth password hash") {
		t.Fatalf("error = %v", err)
	}
}

func TestBasicAuthAcceptsClearPassword(t *testing.T) {
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "clear-secret",
				Realm:    "BearStack",
			},
		},
	}
	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "clear-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("admin", "wrong-secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestBasicAuthChallengeEscapesRealm(t *testing.T) {
	got := basicAuthChallenge("Bear \"Stack\"\r\nX-Injected: yes")
	want := `Basic realm="Bear \"Stack\"X-Injected: yes", charset="UTF-8"`
	if got != want {
		t.Fatalf("challenge = %q, want %q", got, want)
	}
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("challenge contains control line break: %q", got)
	}
}

func TestBasicAuthAcceptsConfiguredCredentialsAndRejectsLegacyWhenCredentialsSet(t *testing.T) {
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "legacy",
				Password: "legacy-secret",
				Realm:    "BearStack",
				Credentials: []config.AuthCredential{
					{Username: "reader", Password: "reader-secret", Role: "documents_read"},
					{Username: "photos", Password: "photos-secret", Role: "photos_read"},
				},
			},
		},
		authKey: []byte("01234567890123456789012345678901"),
	}
	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := authPrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("missing auth principal")
		}
		if principal.Username != "reader" || principal.Role != "documents_read" {
			t.Fatalf("principal = %#v", principal)
		}
		if !principal.hasAll(authCapDocumentsRead|authCapDocumentsWebDAVRead) || principal.hasAny(authCapDocumentsUpload) {
			t.Fatalf("capabilities = %#v", principal.capabilities)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("reader", "reader-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("legacy", "legacy-secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("legacy status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRequireRejectsMissingCapability(t *testing.T) {
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Credentials: []config.AuthCredential{
					{Username: "reader", Password: "reader-secret", Role: "documents_read"},
				},
			},
		},
		authKey: []byte("01234567890123456789012345678901"),
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	called := false
	handler := server.basicAuth(http.HandlerFunc(server.require(authCapDocumentsUpload, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})))

	req := httptest.NewRequest(http.MethodPost, "/api/upload", nil)
	req.SetBasicAuth("reader", "reader-secret")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("handler was called")
	}
}

func TestHandlerRoutesRequireConfiguredCapabilities(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Credentials: []config.AuthCredential{
					{Username: "reader", Password: "reader-secret", Role: "documents_read"},
					{Username: "photos", Password: "photos-secret", Role: "photos_read"},
				},
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		static:    http.NotFoundHandler(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodOptions, "/webdav", nil)
	req.SetBasicAuth("reader", "reader-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reader webdav status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodOptions, "/webdav", nil)
	req.SetBasicAuth("photos", "photos-secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("photos webdav status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.SetBasicAuth("reader", "reader-secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("settings status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestNewRejectsInvalidAuthCredentialConfig(t *testing.T) {
	tests := []struct {
		name string
		auth config.AuthConfig
		want string
	}{
		{
			name: "unknown role",
			auth: config.AuthConfig{Credentials: []config.AuthCredential{
				{Username: "user", Password: "secret", Role: "unknown"},
			}},
			want: "unknown auth role",
		},
		{
			name: "unknown permission",
			auth: config.AuthConfig{Credentials: []config.AuthCredential{
				{Username: "user", Password: "secret", Permissions: []string{"documents.fly"}},
			}},
			want: "unknown auth permission",
		},
		{
			name: "duplicate username",
			auth: config.AuthConfig{Credentials: []config.AuthCredential{
				{Username: "user", Password: "secret", Role: "documents_read"},
				{Username: "user", Password: "secret", Role: "photos_read"},
			}},
			want: "duplicate auth credential username",
		},
		{
			name: "invalid hash",
			auth: config.AuthConfig{Credentials: []config.AuthCredential{
				{Username: "user", PasswordHash: "not-a-bcrypt-hash", Role: "documents_read"},
			}},
			want: "invalid auth credential 1 password hash",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(config.Config{Auth: tc.auth}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestBasicAuthPositiveCache(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("cache-secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Credentials: []config.AuthCredential{
					{Username: "dav", PasswordHash: string(hash), Role: "documents_read"},
				},
			},
		},
		authKey: []byte("01234567890123456789012345678901"),
	}

	principal, ok := server.authenticateBasic("dav", "cache-secret")
	if !ok || principal.Username != "dav" {
		t.Fatalf("first auth = %#v %v", principal, ok)
	}
	if len(server.auth.cache.entries) != 1 {
		t.Fatalf("cache entries = %d", len(server.auth.cache.entries))
	}
	server.auth.credentials["dav"].passwordHash = []byte("not-a-bcrypt-hash")

	principal, ok = server.authenticateBasic("dav", "cache-secret")
	if !ok || principal.Username != "dav" {
		t.Fatalf("cached auth = %#v %v", principal, ok)
	}
	if _, ok := server.authenticateBasic("dav", "wrong-secret"); ok {
		t.Fatal("wrong password should not be accepted")
	}
}

func TestSameOriginUnsafeRequestsRejectsCrossOrigin(t *testing.T) {
	server := &Server{}
	called := false
	handler := server.sameOriginUnsafeRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://bearstack.local/settings", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("handler was called")
	}
}

func TestSameOriginUnsafeRequestsAllowsSameOriginAndAPIClients(t *testing.T) {
	server := sameOriginAuthServer()
	handler := server.sameOriginUnsafeRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for name, configure := range map[string]func(*http.Request){
		"same origin": func(r *http.Request) {
			r.Header.Set("Origin", "http://bearstack.local")
		},
		"same origin default port": func(r *http.Request) {
			r.Host = "bearstack.local:80"
			r.Header.Set("Origin", "http://bearstack.local")
		},
		"authorization header without browser origin": func(r *http.Request) {
			r.Header.Set("Authorization", "Basic dXNlcjpzZWNyZXQ=")
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://bearstack.local/settings", nil)
			configure(req)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSameOriginUnsafeRequestsRejectsMissingOriginWithoutAuthorization(t *testing.T) {
	server := sameOriginAuthServer()
	called := false
	handler := server.sameOriginUnsafeRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://bearstack.local/settings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if called {
		t.Fatal("handler was called")
	}
}

func sameOriginAuthServer() *Server {
	return &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey: []byte("01234567890123456789012345678901"),
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestSameOriginUnsafeRequestsTreatsWebDAVPropfindAsSafe(t *testing.T) {
	server := &Server{}
	handler := server.sameOriginUnsafeRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest("PROPFIND", "http://bearstack.local/.well-known/webdav", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestSameOriginUnsafeRequestsChecksRefererWhenOriginMissing(t *testing.T) {
	server := &Server{}
	handler := server.sameOriginUnsafeRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "http://bearstack.local/settings", nil)
	req.Header.Set("Referer", "http://evil.example/form")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestBasicAuthRendersLoginPageForHTMLRequest(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/photos/random", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("unexpected auth challenge = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `name="username"`) {
		t.Fatalf("login page missing username field: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="password"`) {
		t.Fatalf("login page missing password field: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="remember"`) {
		t.Fatalf("login page missing remember field: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="return" value="/photos/random"`) {
		t.Fatalf("login page missing return: %s", rec.Body.String())
	}
}

func TestBasicAuthReturnsUnauthorizedForNonHTMLRequest(t *testing.T) {
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey: []byte("01234567890123456789012345678901"),
	}

	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/photos/random", nil)
	req.Header.Set("Accept", "image/webp")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatalf("missing auth challenge")
	}
}

func TestBasicAuthReturnsUnauthorizedForFetchRequestWithEmptyDestination(t *testing.T) {
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey: []byte("01234567890123456789012345678901"),
	}

	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatalf("missing auth challenge")
	}
	if strings.Contains(rec.Body.String(), `name="username"`) {
		t.Fatalf("fetch request rendered login page: %s", rec.Body.String())
	}
}

func TestBasicAuthAllowsPublicAssetPathsWithoutBasicChallenge(t *testing.T) {
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey: []byte("01234567890123456789012345678901"),
	}
	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	req.Header.Set("Accept", "text/css")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("unexpected auth challenge = %q", got)
	}
}

func TestBasicAuthRendersLoginForNavigationRequestsWithoutHTMLAccept(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/photos/random", nil)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("unexpected auth challenge = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `name="username"`) {
		t.Fatalf("login page missing username field: %s", rec.Body.String())
	}
}

func TestLoginRouteIsPublic(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		static:    http.NotFoundHandler(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	handler := server.Handler()

	req := httptest.NewRequest(http.MethodGet, "/login?return=/photos/random", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `name="return" value="/photos/random"`) {
		t.Fatalf("login page missing return: %s", rec.Body.String())
	}
}

func TestLoginPostCreatesSessionAndRedirects(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		static:    http.NotFoundHandler(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"username": {"admin"},
		"password": {"secret"},
		"return":   {"/settings"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/settings" {
		t.Fatalf("location = %q", got)
	}
	cookie := testCookieByName(t, rec.Result().Cookies(), authSessionCookieName)
	assertCookieMaxAgeAround(t, cookie, authSessionDuration)

	next := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	authenticatedReq := httptest.NewRequest(http.MethodGet, "/photos", nil)
	authenticatedReq.AddCookie(cookie)
	authenticatedRec := httptest.NewRecorder()
	next.ServeHTTP(authenticatedRec, authenticatedReq)

	if authenticatedRec.Code != http.StatusNoContent {
		t.Fatalf("session auth status = %d body = %s", authenticatedRec.Code, authenticatedRec.Body.String())
	}
}

func TestLoginPostRememberExtendsSession(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		static:    http.NotFoundHandler(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"username": {"admin"},
		"password": {"secret"},
		"remember": {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	cookie := testCookieByName(t, rec.Result().Cookies(), authSessionCookieName)
	assertCookieMaxAgeAround(t, cookie, authRememberSessionDuration)
	payload, ok := server.verifyAuthSession(cookie.Value)
	if !ok {
		t.Fatal("remember session cookie did not verify")
	}
	sessionRemaining := time.Until(time.Unix(payload.Expires, 0))
	if sessionRemaining < authRememberSessionDuration-time.Minute || sessionRemaining > authRememberSessionDuration+time.Minute {
		t.Fatalf("remember session remaining = %s, want about %s", sessionRemaining, authRememberSessionDuration)
	}
}

func TestLogoutPostClearsSessionCookieAndRedirectsToLogin(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		static:    http.NotFoundHandler(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"username": {"admin"},
		"password": {"secret"},
	}
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("Origin", "http://example.com")
	loginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginRec, loginReq)
	sessionCookie := testCookieByName(t, loginRec.Result().Cookies(), authSessionCookieName)

	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutReq.Header.Set("Origin", "http://example.com")
	logoutRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", logoutRec.Code, logoutRec.Body.String())
	}
	if got := logoutRec.Header().Get("Location"); got != "/login" {
		t.Fatalf("location = %q", got)
	}
	clearedCookie := testCookieByName(t, logoutRec.Result().Cookies(), authSessionCookieName)
	if clearedCookie.MaxAge >= 0 {
		t.Fatalf("cleared cookie max age = %d, want negative", clearedCookie.MaxAge)
	}
	if !clearedCookie.Expires.Before(time.Now()) {
		t.Fatalf("cleared cookie expires = %s, want in the past", clearedCookie.Expires)
	}
}

func TestLoginPostRedirectsToLandingForUnauthorizedReturnPath(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Credentials: []config.AuthCredential{
					{Username: "reader", Password: "secret", Role: "documents_read"},
				},
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		static:    http.NotFoundHandler(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	form := url.Values{
		"username": {"reader"},
		"password": {"secret"},
		"return":   {"/photos/random"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.com")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/" {
		t.Fatalf("location = %q", got)
	}
}

func TestLoginPostRejectsInvalidCredentials(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg: config.Config{
			Auth: config.AuthConfig{
				Username: "admin",
				Password: "secret",
			},
		},
		authKey:   []byte("01234567890123456789012345678901"),
		templates: templates,
		static:    http.NotFoundHandler(),
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	form := url.Values{
		"username": {"admin"},
		"password": {"wrong"},
	}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("unexpected auth challenge = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "Login fehlgeschlagen") {
		t.Fatalf("error text missing: %s", rec.Body.String())
	}
}

func assertCookieMaxAgeAround(t *testing.T, cookie *http.Cookie, duration time.Duration) {
	t.Helper()
	if cookie == nil {
		t.Fatal("missing cookie")
	}
	maxAge := time.Duration(cookie.MaxAge) * time.Second
	if maxAge < duration-time.Minute || maxAge > duration+time.Minute {
		t.Fatalf("cookie max age = %s, want about %s", maxAge, duration)
	}
}
