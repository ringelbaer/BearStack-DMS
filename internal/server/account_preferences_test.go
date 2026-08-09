package server

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/config"
	"bearstack/internal/repository"
)

func TestAccountPreferenceSelfServiceForConfigAccount(t *testing.T) {
	const username = "config-reader"
	const password = "configuration reader password"
	authConfig := config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: username, Password: password, Role: account.RoleDocumentsRead,
	}}}
	server, repo := newUserRouteTestServer(t, authConfig)
	handler := server.Handler()
	before, ok := server.authenticateBasic(username, password)
	if !ok {
		t.Fatal("initial authentication failed")
	}

	initial := performUserRouteRequest(handler, http.MethodGet, "/account", nil, username, password)
	if initial.Code != http.StatusOK || !strings.Contains(initial.Body.String(), `name="preference_version" value="0"`) {
		t.Fatalf("initial account status=%d body=%s", initial.Code, initial.Body.String())
	}
	form := url.Values{
		"preference_version":         {"0"},
		"custom_pdf_preview_enabled": {"true"},
	}
	rec := performUserRouteRequest(handler, http.MethodPost, "/account/preferences", form, username, password)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/account?") {
		t.Fatalf("save status=%d location=%q body=%s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	preference, err := repo.AccountPreference(context.Background(), repository.AccountSourceConfig, username)
	if err != nil {
		t.Fatal(err)
	}
	if !preference.CustomPDFPreviewEnabled || preference.RowVersion != 1 {
		t.Fatalf("saved preference = %#v", preference)
	}
	restarted := buildUserRouteTestServer(t, repo, authConfig)
	if loaded := restarted.accountPreference(repository.AccountSourceConfig, username); !loaded.CustomPDFPreviewEnabled || loaded.RowVersion != 1 {
		t.Fatalf("preference after restart = %#v", loaded)
	}
	after, ok := server.authenticateBasic(username, password)
	if !ok || after.Revision != before.Revision {
		t.Fatalf("preference changed authentication revision: before=%#v after=%#v", before, after)
	}
	page := performUserRouteRequest(handler, http.MethodGet, "/account", nil, username, password)
	if !strings.Contains(page.Body.String(), `data-custom-pdf-preview="true"`) || !strings.Contains(page.Body.String(), `value="true" checked`) {
		t.Fatalf("updated account page = %s", page.Body.String())
	}

	stale := performUserRouteRequest(handler, http.MethodPost, "/account/preferences", form, username, password)
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "zwischenzeitlich geändert") {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestAdminPreferenceMutationForDatabaseAndConfigAccountsIsAudited(t *testing.T) {
	const adminUsername = "config-admin"
	const adminPassword = "configuration admin password"
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{
		{Username: adminUsername, Password: adminPassword, Role: account.RoleAdmin},
		{Username: "config-reader", Password: "configuration reader password", Role: account.RoleDocumentsRead},
	}})
	target := createServerUser(t, repo, "database-reader", "database reader password", account.RoleDocumentsRead, nil, true)
	if err := server.reloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()

	for _, test := range []struct{ source, subject, target string }{
		{repository.AccountSourceDatabase, strconv.FormatInt(target.ID, 10), target.Username},
		{repository.AccountSourceConfig, "config-reader", "config-reader"},
	} {
		form := url.Values{
			"account_source":             {test.source},
			"account_subject":            {test.subject},
			"preference_version":         {"0"},
			"custom_pdf_preview_enabled": {"true"},
		}
		rec := performUserRouteRequest(handler, http.MethodPost, "/settings/users/preferences", form, adminUsername, adminPassword)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("target %s status=%d body=%s", test.target, rec.Code, rec.Body.String())
		}
		preference, err := repo.AccountPreference(context.Background(), test.source, test.subject)
		if err != nil || !preference.CustomPDFPreviewEnabled {
			t.Fatalf("target %s preference=%#v err=%v", test.target, preference, err)
		}
	}

	logs, err := repo.ListAuditLogs(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range logs {
		if entry.Action == "PDF-Vorschaupräferenz ändern" && entry.Actor == adminUsername && entry.Status == http.StatusSeeOther {
			seen[entry.Target] = true
		}
	}
	for _, targetName := range []string{"database-reader", "config-reader"} {
		if !seen["Benutzer:"+targetName] {
			t.Fatalf("missing audit for %s: %#v", targetName, logs)
		}
	}
	deleteForm := url.Values{"version": {"1"}, "current_password": {adminPassword}}
	deleteRec := performUserRouteRequest(handler, http.MethodPost, userEditURL(target.ID)+"/delete", deleteForm, adminUsername, adminPassword)
	if deleteRec.Code != http.StatusSeeOther {
		t.Fatalf("delete status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if preference := server.accountPreference(repository.AccountSourceDatabase, strconv.FormatInt(target.ID, 10)); preference.RowVersion != 0 {
		t.Fatalf("deleted preference remained in snapshot: %#v", preference)
	}
}

func TestDelegatedManagerPreferenceTargetRestrictions(t *testing.T) {
	const managerPassword = "delegated manager password"
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{
		{Username: "manager", Password: managerPassword, Role: account.RoleCustom, Permissions: []string{
			account.PermissionSystemUsersManage, account.PermissionDocumentsRead,
		}},
		{Username: "config-admin", Password: "configuration admin password", Role: account.RoleAdmin},
	}})
	ordinary := createServerUser(t, repo, "ordinary", "ordinary user password", account.RoleCustom, []string{account.PermissionDocumentsRead}, true)
	privileged := createServerUser(t, repo, "privileged", "privileged user password", account.RoleAdmin, nil, true)
	if err := server.reloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	for _, test := range []struct {
		id       int64
		wantCode int
	}{{ordinary.ID, http.StatusSeeOther}, {privileged.ID, http.StatusForbidden}} {
		form := url.Values{"account_source": {repository.AccountSourceDatabase}, "account_subject": {strconv.FormatInt(test.id, 10)}, "preference_version": {"0"}}
		rec := performUserRouteRequest(handler, http.MethodPost, "/settings/users/preferences", form, "manager", managerPassword)
		if rec.Code != test.wantCode {
			t.Fatalf("target %d status=%d want=%d body=%s", test.id, rec.Code, test.wantCode, rec.Body.String())
		}
	}
	configAdminForm := url.Values{"account_source": {repository.AccountSourceConfig}, "account_subject": {"config-admin"}, "preference_version": {"0"}}
	if rec := performUserRouteRequest(handler, http.MethodPost, "/settings/users/preferences", configAdminForm, "manager", managerPassword); rec.Code != http.StatusForbidden {
		t.Fatalf("config admin status=%d body=%s", rec.Code, rec.Body.String())
	}
}
