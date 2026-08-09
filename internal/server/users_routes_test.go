package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/config"
	"bearstack/internal/repository"
)

func TestUserManagementRoutesCreateAndUpdateWithPRG(t *testing.T) {
	const (
		adminUsername = "config-admin"
		adminPassword = "configuration admin password"
		newPassword   = "new database user password"
	)
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: adminUsername,
		Password: adminPassword,
		Role:     account.RoleAdmin,
	}}})
	handler := server.Handler()

	createForm := url.Values{
		"username":                  {"new-reader"},
		"role":                      {account.RoleDocumentsRead},
		"new_password":              {newPassword},
		"new_password_confirmation": {newPassword},
		"current_password":          {adminPassword},
	}
	createRec := performUserRouteRequest(handler, http.MethodPost, "/settings/users", createForm, adminUsername, adminPassword)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	createLocation := createRec.Header().Get("Location")
	if !strings.HasPrefix(createLocation, "/settings/users?") || redirectNotice(t, createLocation) == "" {
		t.Fatalf("create redirect = %q", createLocation)
	}

	users, err := repo.ListUsers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != "new-reader" || users[0].Role != account.RoleDocumentsRead {
		t.Fatalf("users after create = %#v", users)
	}
	created := users[0]
	if created.SessionVersion != 1 || created.RowVersion != 1 {
		t.Fatalf("created versions = session %d, row %d", created.SessionVersion, created.RowVersion)
	}

	accountRec := performUserRouteRequest(handler, http.MethodGet, "/account", nil, "new-reader", newPassword)
	if accountRec.Code != http.StatusOK {
		t.Fatalf("new account login status = %d, body = %s", accountRec.Code, accountRec.Body.String())
	}
	if !strings.Contains(accountRec.Body.String(), "new-reader") || strings.Contains(accountRec.Body.String(), newPassword) {
		t.Fatalf("new account page is unsafe or incomplete: %s", accountRec.Body.String())
	}

	updateForm := url.Values{
		"version":          {"1"},
		"role":             {account.RolePhotosRead},
		"current_password": {adminPassword},
	}
	updatePath := userEditURL(created.ID)
	updateRec := performUserRouteRequest(handler, http.MethodPost, updatePath, updateForm, adminUsername, adminPassword)
	if updateRec.Code != http.StatusSeeOther {
		t.Fatalf("update status = %d, body = %s", updateRec.Code, updateRec.Body.String())
	}
	updateLocation := updateRec.Header().Get("Location")
	if !strings.HasPrefix(updateLocation, updatePath+"?") || redirectNotice(t, updateLocation) == "" {
		t.Fatalf("update redirect = %q", updateLocation)
	}
	updated, err := repo.UserByID(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != account.RolePhotosRead || updated.SessionVersion != 2 || updated.RowVersion != 2 {
		t.Fatalf("updated user = %#v", updated)
	}
}

func TestUserManagementCreateValidationDoesNotEchoPasswords(t *testing.T) {
	const (
		adminPassword    = "configuration admin password"
		passwordSentinel = "never echo this new password"
		confirmSentinel  = "never echo this confirmation"
	)
	server, _ := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: "admin",
		Password: adminPassword,
		Role:     account.RoleAdmin,
	}}})
	form := url.Values{
		"username":                  {"  retained-user  "},
		"role":                      {account.RoleDocumentsRead},
		"new_password":              {passwordSentinel},
		"new_password_confirmation": {confirmSentinel},
		"current_password":          {adminPassword},
	}
	rec := performUserRouteRequest(server.Handler(), http.MethodPost, "/settings/users", form, "admin", adminPassword)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("validation status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"retained-user", "Die Passwörter stimmen nicht überein.", `id="new-password-confirmation-error"`, `aria-invalid="true"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("validation response missing %q: %s", want, body)
		}
	}
	for _, secret := range []string{passwordSentinel, confirmSentinel, adminPassword} {
		if strings.Contains(body, secret) {
			t.Fatalf("validation response echoed password %q: %s", secret, body)
		}
	}
	assertRenderedPasswordInputsEmpty(t, body)
}

func TestUserManagementUpdateReturnsConflictForStaleForm(t *testing.T) {
	const adminPassword = "configuration admin password"
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: "admin",
		Password: adminPassword,
		Role:     account.RoleAdmin,
	}}})
	target := createServerUser(t, repo, "stale-target", "stale target password", account.RoleDocumentsRead, nil, true)
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}

	current, err := repo.UpdateUserAccess(context.Background(), target.ID, account.UpdateUserAccessParams{
		Role:               account.RoleDocumentsEditor,
		ExpectedRowVersion: target.RowVersion,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"version":          {"1"},
		"role":             {account.RolePhotosRead},
		"current_password": {adminPassword},
	}
	rec := performUserRouteRequest(server.Handler(), http.MethodPost, userEditURL(target.ID), form, "admin", adminPassword)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "zwischenzeitlich geändert") || !strings.Contains(body, "Bitte laden Sie die Seite neu") {
		t.Fatalf("stale update response lacks actionable conflict message: %s", body)
	}
	if strings.Contains(body, adminPassword) {
		t.Fatalf("stale response echoed current password: %s", body)
	}
	reloaded, err := repo.UserByID(context.Background(), target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Role != current.Role || reloaded.RowVersion != current.RowVersion || reloaded.SessionVersion != current.SessionVersion {
		t.Fatalf("stale request changed user: %#v, current %#v", reloaded, current)
	}
}

func TestUserManagementRoutesEnforceCapabilitySelfAndDelegatedTargetProtection(t *testing.T) {
	t.Run("route capability", func(t *testing.T) {
		server, _ := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
			Username: "reader",
			Password: "reader configuration password",
			Role:     account.RoleDocumentsRead,
		}}})
		rec := performUserRouteRequest(server.Handler(), http.MethodGet, "/settings/users", nil, "reader", "reader configuration password")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("users route status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("self protection", func(t *testing.T) {
		repo := openServerUserRepository(t)
		self := createServerUser(t, repo, "db-admin", "database admin password", account.RoleAdmin, nil, true)
		server := buildUserRouteTestServer(t, repo, config.AuthConfig{})
		rec := performUserRouteRequest(server.Handler(), http.MethodGet, userEditURL(self.ID), nil, "db-admin", "database admin password")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("self edit status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("delegated target protection and ordinary subset", func(t *testing.T) {
		const managerPassword = "delegated manager password"
		server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
			Username:    "manager",
			Password:    managerPassword,
			Role:        account.RoleCustom,
			Permissions: []string{account.PermissionSystemUsersManage, account.PermissionDocumentsRead, account.PermissionDocumentsWebDAVRead},
		}}})
		privileged := createServerUser(t, repo, "other-admin", "other administrator password", account.RoleAdmin, nil, true)
		ordinary := createServerUser(t, repo, "ordinary-reader", "ordinary reader password", account.RoleDocumentsRead, nil, true)
		if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
			t.Fatal(err)
		}

		forbidden := performUserRouteRequest(server.Handler(), http.MethodGet, userEditURL(privileged.ID), nil, "manager", managerPassword)
		if forbidden.Code != http.StatusForbidden {
			t.Fatalf("privileged target status = %d, body = %s", forbidden.Code, forbidden.Body.String())
		}
		allowed := performUserRouteRequest(server.Handler(), http.MethodGet, userEditURL(ordinary.ID), nil, "manager", managerPassword)
		if allowed.Code != http.StatusOK {
			t.Fatalf("ordinary target status = %d, body = %s", allowed.Code, allowed.Body.String())
		}
		if !strings.Contains(allowed.Body.String(), "ordinary-reader") {
			t.Fatalf("ordinary target form missing username: %s", allowed.Body.String())
		}
	})
}

func TestDatabaseAccountPasswordChangeUsesPRGAndRefreshesAuthentication(t *testing.T) {
	const (
		username    = "db-user"
		oldPassword = "old database account password"
		newPassword = "new database account password"
	)
	repo := openServerUserRepository(t)
	user := createServerUser(t, repo, username, oldPassword, account.RoleDocumentsRead, nil, true)
	server := buildUserRouteTestServer(t, repo, config.AuthConfig{})
	handler := server.Handler()

	form := url.Values{
		"current_password":          {oldPassword},
		"new_password":              {newPassword},
		"new_password_confirmation": {newPassword},
	}
	rec := performUserRouteRequest(handler, http.MethodPost, "/account/password", form, username, oldPassword)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("password change status = %d, body = %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/account?") || redirectNotice(t, location) == "" {
		t.Fatalf("password change redirect = %q", location)
	}
	if setCookie := rec.Header().Get("Set-Cookie"); !strings.Contains(setCookie, authSessionCookieName+"=") || !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("password change did not refresh secure session cookie: %q", setCookie)
	}
	updated, err := repo.UserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SessionVersion != 2 || updated.RowVersion != 2 {
		t.Fatalf("password change versions = session %d, row %d", updated.SessionVersion, updated.RowVersion)
	}

	oldLogin := performUserRouteRequest(handler, http.MethodGet, "/account", nil, username, oldPassword)
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old password status = %d, body = %s", oldLogin.Code, oldLogin.Body.String())
	}
	newLogin := performUserRouteRequest(handler, http.MethodGet, "/account", nil, username, newPassword)
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password status = %d, body = %s", newLogin.Code, newLogin.Body.String())
	}
}

func newUserRouteTestServer(t *testing.T, authConfig config.AuthConfig) (*Server, *repository.Repository) {
	t.Helper()
	repo := openServerUserRepository(t)
	return buildUserRouteTestServer(t, repo, authConfig), repo
}

func buildUserRouteTestServer(t *testing.T, repo *repository.Repository, authConfig config.AuthConfig) *Server {
	t.Helper()
	server, err := New(config.Config{
		DataDir: t.TempDir(),
		Auth:    authConfig,
	}, repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func openServerUserRepository(t *testing.T) *repository.Repository {
	t.Helper()
	repo, err := repository.Open(context.Background(), filepath.Join(t.TempDir(), "server-users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("close repository: %v", err)
		}
	})
	return repo
}

func createServerUser(t *testing.T, repo *repository.Repository, username, password, role string, permissions []string, enabled bool) account.User {
	t.Helper()
	hash, err := account.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user, err := repo.CreateUser(context.Background(), account.CreateUserParams{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Permissions:  permissions,
		Enabled:      enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func performUserRouteRequest(handler http.Handler, method, path string, form url.Values, username, password string) *httptest.ResponseRecorder {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, "http://bearstack.test"+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
