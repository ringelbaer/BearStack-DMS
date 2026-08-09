package server

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/config"
)

func TestUserBootstrapCreatesAdministratorAndStartsProtectedSession(t *testing.T) {
	const (
		username = "first-admin"
		password = "first administrator password"
	)
	repo := openServerUserRepository(t)
	server, err := New(config.Config{Addr: "127.0.0.1:8080", DataDir: t.TempDir()}, repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	form := url.Values{
		"username":                  {username},
		"role":                      {account.RoleDocumentsRead},
		"new_password":              {password},
		"new_password_confirmation": {password},
	}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/settings/users", strings.NewReader(form.Encode()))
	req.RemoteAddr = "127.0.0.1:43120"
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("bootstrap status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); !strings.HasPrefix(location, "/settings/users?") || redirectNotice(t, location) == "" {
		t.Fatalf("bootstrap redirect = %q", location)
	}
	cookie := userRouteResponseCookie(t, rec, authSessionCookieName)
	if !cookie.HttpOnly || cookie.Value == "" {
		t.Fatalf("bootstrap session cookie = %#v", cookie)
	}

	users, err := repo.ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != username || users[0].Role != account.RoleAdmin || !users[0].Enabled {
		t.Fatalf("bootstrap users = %#v", users)
	}

	authenticatedReq := httptest.NewRequest(http.MethodGet, "http://localhost/settings/users", nil)
	authenticatedReq.RemoteAddr = "127.0.0.1:43121"
	authenticatedReq.AddCookie(cookie)
	authenticatedRec := httptest.NewRecorder()
	handler.ServeHTTP(authenticatedRec, authenticatedReq)
	if authenticatedRec.Code != http.StatusOK {
		t.Fatalf("bootstrap session status = %d, body = %s", authenticatedRec.Code, authenticatedRec.Body.String())
	}

	unauthenticatedReq := httptest.NewRequest(http.MethodGet, "http://localhost/settings/users", nil)
	unauthenticatedReq.RemoteAddr = "127.0.0.1:43122"
	unauthenticatedRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRec, unauthenticatedReq)
	if unauthenticatedRec.Code != http.StatusUnauthorized {
		t.Fatalf("post-bootstrap anonymous status = %d, want 401; body = %s", unauthenticatedRec.Code, unauthenticatedRec.Body.String())
	}
}

func TestUserPasswordResetEnableDisableAndDeleteRoutesUsePRG(t *testing.T) {
	const (
		adminUsername = "config-admin"
		adminPassword = "configuration admin password"
		oldPassword   = "old managed user password"
		newPassword   = "new managed user password"
	)
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: adminUsername,
		Password: adminPassword,
		Role:     account.RoleAdmin,
	}}})
	target := createServerUser(t, repo, "managed-user", oldPassword, account.RoleDocumentsRead, nil, true)
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	path := userEditURL(target.ID)

	resetForm := url.Values{
		"version":                   {"1"},
		"new_password":              {newPassword},
		"new_password_confirmation": {newPassword},
		"current_password":          {adminPassword},
	}
	resetRec := performUserRouteRequest(handler, http.MethodPost, path+"/password", resetForm, adminUsername, adminPassword)
	assertUserRoutePRG(t, resetRec, path)
	current, err := repo.UserByID(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertServerUserVersions(t, current, 2, 2)
	if rec := performUserRouteRequest(handler, http.MethodGet, "/account", nil, target.Username, oldPassword); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old password after reset status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := performUserRouteRequest(handler, http.MethodGet, "/account", nil, target.Username, newPassword); rec.Code != http.StatusOK {
		t.Fatalf("new password after reset status = %d, body = %s", rec.Code, rec.Body.String())
	}

	disableForm := url.Values{"version": {"2"}, "current_password": {adminPassword}}
	disableRec := performUserRouteRequest(handler, http.MethodPost, path+"/disable", disableForm, adminUsername, adminPassword)
	assertUserRoutePRG(t, disableRec, path)
	current, err = repo.UserByID(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Enabled {
		t.Fatal("disable route left target enabled")
	}
	assertServerUserVersions(t, current, 3, 3)
	if rec := performUserRouteRequest(handler, http.MethodGet, "/account", nil, target.Username, newPassword); rec.Code != http.StatusUnauthorized {
		t.Fatalf("disabled account status = %d, body = %s", rec.Code, rec.Body.String())
	}

	enableForm := url.Values{"version": {"3"}, "current_password": {adminPassword}}
	enableRec := performUserRouteRequest(handler, http.MethodPost, path+"/enable", enableForm, adminUsername, adminPassword)
	assertUserRoutePRG(t, enableRec, path)
	current, err = repo.UserByID(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.Enabled {
		t.Fatal("enable route left target disabled")
	}
	assertServerUserVersions(t, current, 4, 4)
	if rec := performUserRouteRequest(handler, http.MethodGet, "/account", nil, target.Username, newPassword); rec.Code != http.StatusOK {
		t.Fatalf("re-enabled account status = %d, body = %s", rec.Code, rec.Body.String())
	}

	deleteForm := url.Values{"version": {"4"}, "current_password": {adminPassword}}
	deleteRec := performUserRouteRequest(handler, http.MethodPost, path+"/delete", deleteForm, adminUsername, adminPassword)
	assertUserRoutePRG(t, deleteRec, "/settings/users")
	if _, err := repo.UserByID(context.Background(), target.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted UserByID() error = %v, want sql.ErrNoRows", err)
	}
	if rec := performUserRouteRequest(handler, http.MethodGet, "/account", nil, target.Username, newPassword); rec.Code != http.StatusUnauthorized {
		t.Fatalf("deleted account status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUserMutationStepUpRejectsWrongPasswordAndRateLimits(t *testing.T) {
	const (
		adminUsername = "admin"
		adminPassword = "configuration admin password"
		wrongPassword = "wrong step up password sentinel"
	)
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: adminUsername,
		Password: adminPassword,
		Role:     account.RoleAdmin,
	}}})
	target := createServerUser(t, repo, "step-up-target", "step up target password", account.RoleDocumentsRead, nil, true)
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	loginRec := performUserRouteRequest(handler, http.MethodGet, "/account", nil, adminUsername, adminPassword)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("session setup status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}
	cookie := userRouteResponseCookie(t, loginRec, authSessionCookieName)
	form := url.Values{"version": {"1"}, "current_password": {wrongPassword}}
	path := userEditURL(target.ID) + "/disable"
	for attempt := 1; attempt <= authFailureLimit; attempt++ {
		rec := performUserSessionFormRequest(handler, http.MethodPost, path, form, cookie)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("step-up attempt %d status = %d, body = %s", attempt, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Das aktuelle Passwort ist nicht korrekt") {
			t.Fatalf("step-up attempt %d lacks inline error: %s", attempt, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), wrongPassword) {
			t.Fatalf("step-up attempt %d echoed password: %s", attempt, rec.Body.String())
		}
	}

	limited := performUserSessionFormRequest(handler, http.MethodPost, path, form, cookie)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited step-up status = %d, body = %s", limited.Code, limited.Body.String())
	}
	retryAfter, err := strconv.Atoi(limited.Header().Get("Retry-After"))
	if err != nil || retryAfter < 1 {
		t.Fatalf("Retry-After = %q", limited.Header().Get("Retry-After"))
	}
	if !strings.Contains(limited.Body.String(), "Zu viele fehlgeschlagene Versuche") || strings.Contains(limited.Body.String(), wrongPassword) {
		t.Fatalf("rate-limit response is unsafe or incomplete: %s", limited.Body.String())
	}
	reloaded, err := repo.UserByID(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Enabled || reloaded.RowVersion != 1 || reloaded.SessionVersion != 1 {
		t.Fatalf("failed step-up attempts changed target: %#v", reloaded)
	}
}

func TestConfiguredAccountPasswordSelfServiceIsForbidden(t *testing.T) {
	const (
		username    = "configured-user"
		oldPassword = "configured account password"
		newPassword = "replacement configured password"
	)
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: username,
		Password: oldPassword,
		Role:     account.RoleDocumentsRead,
	}}})
	form := url.Values{
		"current_password":          {oldPassword},
		"new_password":              {newPassword},
		"new_password_confirmation": {newPassword},
	}
	rec := performUserRouteRequest(server.Handler(), http.MethodPost, "/account/password", form, username, oldPassword)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("configured self-service status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "durch die Konfiguration verwaltet") {
		t.Fatalf("configured self-service explanation missing: %s", body)
	}
	for _, secret := range []string{oldPassword, newPassword} {
		if strings.Contains(body, secret) {
			t.Fatalf("configured self-service echoed password %q: %s", secret, body)
		}
	}
	users, err := repo.ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 0 {
		t.Fatalf("configured self-service created database users: %#v", users)
	}
	if rec := performUserRouteRequest(server.Handler(), http.MethodGet, "/account", nil, username, oldPassword); rec.Code != http.StatusOK {
		t.Fatalf("configured password changed unexpectedly, status = %d", rec.Code)
	}
}

func TestLastActiveManagerMutationRouteReturnsConflict(t *testing.T) {
	const (
		stepUpUsername = "step-up-reader"
		stepUpPassword = "step up reader password"
	)
	repo := openServerUserRepository(t)
	target := createServerUser(t, repo, "only-database-manager", "only manager password", account.RoleAdmin, nil, true)
	server := buildUserRouteTestServer(t, repo, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: stepUpUsername,
		Password: stepUpPassword,
		Role:     account.RoleDocumentsRead,
	}}})
	credential := server.authSnapshot().byUsername[stepUpUsername]
	if credential == nil {
		t.Fatal("step-up credential is missing")
	}
	principal := credential.principal()
	// Isolate the handler's last-manager mapping with an already-authorized
	// request while keeping the real step-up credential outside the manager set.
	principal.Role = account.RoleAdmin
	principal.capabilities = account.AllCapabilities
	form := url.Values{"version": {"1"}, "current_password": {stepUpPassword}}
	req := httptest.NewRequest(http.MethodPost, "http://bearstack.test"+userEditURL(target.ID)+"/disable", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withAuthPrincipal(req, principal)
	mux := http.NewServeMux()
	server.registerSettingsRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("last-manager status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "letzte aktive Nutzerverwalter") {
		t.Fatalf("last-manager response lacks explanation: %s", rec.Body.String())
	}
	reloaded, err := repo.UserByID(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Enabled || reloaded.RowVersion != 1 || reloaded.SessionVersion != 1 {
		t.Fatalf("last-manager conflict changed target: %#v", reloaded)
	}
}

func assertUserRoutePRG(t *testing.T, rec *httptest.ResponseRecorder, target string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, target+"?") || redirectNotice(t, location) == "" {
		t.Fatalf("redirect = %q, want target %q with notice", location, target)
	}
}

func assertServerUserVersions(t *testing.T, user account.User, sessionVersion, rowVersion int64) {
	t.Helper()
	if user.SessionVersion != sessionVersion || user.RowVersion != rowVersion {
		t.Fatalf("versions = session %d, row %d; want session %d, row %d", user.SessionVersion, user.RowVersion, sessionVersion, rowVersion)
	}
}

func userRouteResponseCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie", name)
	return nil
}

func performUserSessionFormRequest(handler http.Handler, method, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://bearstack.test"+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://bearstack.test")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
