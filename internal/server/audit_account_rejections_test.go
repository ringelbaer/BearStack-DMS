package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"bearstack/internal/account"
	"bearstack/internal/config"
	"bearstack/internal/repository"
)

func TestRejectedAccountAuditTargetNormalizesUserID(t *testing.T) {
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: "audit-admin",
		Password: "audit administrator password",
		Role:     account.RoleAdmin,
	}}})
	target := createServerUser(t, repo, "audit-target", "audit target password", account.RoleDocumentsRead, nil, true)
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "known positive id resolves username",
			path: fmt.Sprintf("/settings/users/%d/delete", target.ID),
			want: "Benutzer:" + target.Username,
		},
		{
			name: "unknown positive id is normalized",
			path: "/settings/users/987654/delete",
			want: "Benutzer-ID:987654",
		},
		{name: "non numeric", path: "/settings/users/not-a-number/delete", want: "Benutzer"},
		{name: "zero", path: "/settings/users/0/delete", want: "Benutzer"},
		{name: "negative", path: "/settings/users/-7/delete", want: "Benutzer"},
		{name: "int64 overflow", path: "/settings/users/9223372036854775808/delete", want: "Benutzer"},
		{name: "escaped newline", path: "/settings/users/42%0aforged/delete", want: "Benutzer"},
		{name: "escaped slash", path: "/settings/users/42%2fforged/delete", want: "Benutzer"},
		{name: "escaped whitespace", path: "/settings/users/%2042/delete", want: "Benutzer"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://bearstack.test"+test.path, nil)
			got := server.rejectedAccountAuditTarget(req, "POST /settings/users/{id}/delete", "audit-admin")
			if got != test.want {
				t.Fatalf("target = %q, want %q", got, test.want)
			}
			if strings.ContainsAny(got, "\r\n") || strings.Contains(got, "forged") {
				t.Fatalf("unsafe path data reached audit target: %q", got)
			}
		})
	}
}

func TestAuditRejectedAccountActionsIgnoresUnmatchedAndNonAccountRequests(t *testing.T) {
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: "audit-admin",
		Password: "audit administrator password",
		Role:     account.RoleAdmin,
	}}})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /account/password", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("POST /documents/{id}/delete", func(http.ResponseWriter, *http.Request) {})
	earlyReject := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
	handler := server.auditRejectedAccountActions(mux, earlyReject)

	requests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/documents/12/delete"},
		{method: http.MethodGet, path: "/account/password"},
		{method: http.MethodPost, path: "/settings/user/12/delete"},
	}
	for _, request := range requests {
		req := httptest.NewRequest(request.method, "http://bearstack.test"+request.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	logs, err := repo.ListAuditLogs(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatalf("outer account audit recorded unrelated requests: %#v", logs)
	}
}

func TestAuditRejectedAccountActionsRecordsEarlySecurityStatuses(t *testing.T) {
	const (
		adminUsername = "audit-admin"
		adminPassword = "audit administrator password"
		blockedUser   = "rate-limited-name"
		blockedSecret = "must never reach the audit log"
	)
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: adminUsername,
		Password: adminPassword,
		Role:     account.RoleAdmin,
	}}})
	target := createServerUser(t, repo, "audit-target", "audit target password", account.RoleDocumentsRead, nil, true)
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	path := "/settings/users/" + strconv.FormatInt(target.ID, 10) + "/disable"

	t.Run("same origin rejection keeps session actor", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://bearstack.test"+path, nil)
		req.AddCookie(sessionCookieForUser(t, server, adminUsername))
		req.Header.Set("Origin", "https://foreign.example")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertLatestRejectedAccountAudit(t, repo, 1, http.StatusForbidden, adminUsername, target.Username, blockedSecret)
	})

	t.Run("missing authentication is audited", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://bearstack.test"+path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertLatestRejectedAccountAudit(t, repo, 2, http.StatusUnauthorized, "anonymous", target.Username, blockedSecret)
	})

	t.Run("rate limit is audited without password", func(t *testing.T) {
		for i := 0; i < authFailureLimit; i++ {
			server.ensureAuthState().limiter.failure(blockedUser)
		}
		req := httptest.NewRequest(http.MethodPost, "http://bearstack.test"+path, nil)
		req.SetBasicAuth(blockedUser, blockedSecret)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Fatal("rate-limited response has no Retry-After header")
		}
		assertLatestRejectedAccountAudit(t, repo, 3, http.StatusTooManyRequests, blockedUser, target.Username, blockedSecret)
	})
}

func TestAuditRejectedAccountActionsUsesGlobalBoundedWindow(t *testing.T) {
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: "audit-admin",
		Password: "audit administrator password",
		Role:     account.RoleAdmin,
	}}})
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	server.earlyAudit.now = func() time.Time { return now }

	mux := http.NewServeMux()
	mux.HandleFunc("POST /settings/users/{id}/disable", func(http.ResponseWriter, *http.Request) {
		t.Fatal("early rejection unexpectedly reached account handler")
	})
	earlyReject := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
	handler := server.auditRejectedAccountActions(mux, earlyReject)
	secret := "global audit limiter secret"

	for i := 0; i < auditRejectionLimit+17; i++ {
		req := httptest.NewRequest(http.MethodPost, "http://bearstack.test/settings/users/987654/disable", nil)
		req.SetBasicAuth(fmt.Sprintf("distinct-actor-%d", i), secret)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d status = %d", i, rec.Code)
		}
	}

	logs, err := repo.ListAuditLogs(context.Background(), auditRejectionLimit+20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != auditRejectionLimit {
		t.Fatalf("audit entries = %d, want global cap %d", len(logs), auditRejectionLimit)
	}
	actors := make(map[string]struct{}, len(logs))
	for _, entry := range logs {
		actors[entry.Actor] = struct{}{}
		if strings.Contains(fmt.Sprint(entry), secret) {
			t.Fatalf("audit limiter entry contains password: %#v", entry)
		}
	}
	if len(actors) != auditRejectionLimit {
		t.Fatalf("distinct audited actors = %d, want %d", len(actors), auditRejectionLimit)
	}

	now = now.Add(auditRejectionWindow)
	req := httptest.NewRequest(http.MethodPost, "http://bearstack.test/settings/users/987654/disable", nil)
	req.SetBasicAuth("actor-after-window-reset", secret)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("post-reset status = %d", rec.Code)
	}
	logs, err = repo.ListAuditLogs(context.Background(), auditRejectionLimit+20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != auditRejectionLimit+1 {
		t.Fatalf("post-reset audit entries = %d, want %d", len(logs), auditRejectionLimit+1)
	}
	if logs[0].Actor != "actor-after-window-reset" {
		t.Fatalf("post-reset audit actor = %q", logs[0].Actor)
	}
}

func TestInnerLoginFailuresAreGloballyBoundedButSuccessIsAlwaysAudited(t *testing.T) {
	const (
		adminUsername = "successful-login-admin"
		adminPassword = "successful login administrator password"
		failedSecret  = "failed login secret must not be audited"
	)
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: adminUsername,
		Password: adminPassword,
		Role:     account.RoleAdmin,
	}}})
	fixedNow := time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC)
	server.earlyAudit.now = func() time.Time { return fixedNow }
	handler := server.Handler()

	for i := 0; i < auditRejectionLimit+17; i++ {
		username := fmt.Sprintf("blocked-login-%d", i)
		for failure := 0; failure < authFailureLimit; failure++ {
			server.ensureAuthState().limiter.failure(username)
		}
		form := url.Values{"username": {username}, "password": {failedSecret}}
		req := httptest.NewRequest(http.MethodPost, "http://bearstack.test/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://bearstack.test")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("failed login %d status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Fatalf("failed login %d has no Retry-After header", i)
		}
	}

	logs, err := repo.ListAuditLogs(context.Background(), auditRejectionLimit+20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != auditRejectionLimit {
		t.Fatalf("failed-login audit entries = %d, want global cap %d", len(logs), auditRejectionLimit)
	}
	for _, entry := range logs {
		if entry.Status != http.StatusTooManyRequests || entry.Action != "Anmeldung" || entry.Route != "POST /login" {
			t.Fatalf("failed-login audit entry = %#v", entry)
		}
		if strings.Contains(fmt.Sprint(entry), failedSecret) {
			t.Fatalf("failed-login audit entry contains password: %#v", entry)
		}
	}

	successForm := url.Values{"username": {adminUsername}, "password": {adminPassword}}
	successReq := httptest.NewRequest(http.MethodPost, "http://bearstack.test/login", strings.NewReader(successForm.Encode()))
	successReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	successReq.Header.Set("Origin", "http://bearstack.test")
	successRec := httptest.NewRecorder()
	handler.ServeHTTP(successRec, successReq)
	if successRec.Code != http.StatusSeeOther {
		t.Fatalf("successful login status = %d, body = %s", successRec.Code, successRec.Body.String())
	}
	logs, err = repo.ListAuditLogs(context.Background(), auditRejectionLimit+20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != auditRejectionLimit+1 {
		t.Fatalf("audit entries after successful login = %d, want %d", len(logs), auditRejectionLimit+1)
	}
	got := logs[0]
	if got.Status != http.StatusSeeOther || got.Actor != adminUsername || got.Target != "Benutzer:"+adminUsername || got.Action != "Anmeldung" {
		t.Fatalf("successful login audit entry = %#v", got)
	}
	if strings.Contains(fmt.Sprint(got), adminPassword) {
		t.Fatalf("successful login audit entry contains password: %#v", got)
	}
}

func TestAuditRejectedAccountActionsRecordsInnerResponsesExactlyOnce(t *testing.T) {
	statuses := []int{http.StatusForbidden, http.StatusConflict, http.StatusSeeOther}
	for _, status := range statuses {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
				Username: "audit-admin",
				Password: "audit administrator password",
				Role:     account.RoleAdmin,
			}}})
			mux := http.NewServeMux()
			mux.HandleFunc("POST /settings/users/{id}", func(w http.ResponseWriter, r *http.Request) {
				setAuditTarget(r, "Benutzer:inner-target")
				w.WriteHeader(status)
			})
			handler := server.auditRejectedAccountActions(mux, server.auditWriteActions(mux))

			req := httptest.NewRequest(http.MethodPost, "http://bearstack.test/settings/users/42", nil)
			req.SetBasicAuth("inner-actor", "inner secret must not be audited")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != status {
				t.Fatalf("status = %d, want %d", rec.Code, status)
			}

			logs, err := repo.ListAuditLogs(context.Background(), 10, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(logs) != 1 {
				t.Fatalf("audit entries = %d, want exactly one: %#v", len(logs), logs)
			}
			got := logs[0]
			if got.Status != status || got.Actor != "inner-actor" || got.Action != "Benutzerrechte ändern" || got.Target != "Benutzer:inner-target" {
				t.Fatalf("audit entry = %#v", got)
			}
			if strings.Contains(fmt.Sprint(got), "inner secret must not be audited") {
				t.Fatalf("audit entry contains Basic password: %#v", got)
			}
		})
	}
}

func TestSuccessfulLoginAuditUsesAuthenticatedAccountWithoutPassword(t *testing.T) {
	const (
		username = "login-audit-admin"
		password = "login audit administrator password"
	)
	server, repo := newUserRouteTestServer(t, config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: username,
		Password: password,
		Role:     account.RoleAdmin,
	}}})
	form := url.Values{
		"username": {username},
		"password": {password},
	}
	req := httptest.NewRequest(http.MethodPost, "http://bearstack.test/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://bearstack.test")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}

	logs, err := repo.ListAuditLogs(context.Background(), 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("audit entries = %d, want exactly one: %#v", len(logs), logs)
	}
	got := logs[0]
	if got.Status != http.StatusSeeOther || got.Actor != username || got.Action != "Anmeldung" || got.Target != "Benutzer:"+username || got.Route != "POST /login" {
		t.Fatalf("login audit entry = %#v", got)
	}
	if strings.Contains(fmt.Sprint(got), password) {
		t.Fatalf("login audit entry contains password: %#v", got)
	}
}

func assertLatestRejectedAccountAudit(t *testing.T, repo *repository.Repository, wantCount, wantStatus int, wantActor, wantTargetUsername, forbiddenSecret string) {
	t.Helper()
	logs, err := repo.ListAuditLogs(context.Background(), 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != wantCount {
		t.Fatalf("audit entries = %d, want %d: %#v", len(logs), wantCount, logs)
	}
	got := logs[0]
	if got.Status != wantStatus || got.Actor != wantActor || got.Action != "Benutzer deaktivieren" || got.Target != "Benutzer:"+wantTargetUsername || got.Route != "POST /settings/users/{id}/disable" {
		t.Fatalf("audit entry = %#v", got)
	}
	if strings.Contains(fmt.Sprint(got), forbiddenSecret) {
		t.Fatalf("audit entry contains password: %#v", got)
	}
}
