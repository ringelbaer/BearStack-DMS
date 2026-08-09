package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"bearstack/internal/account"
	"bearstack/internal/config"
	"bearstack/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

func openAuthSecurityRepository(t *testing.T) *repository.Repository {
	t.Helper()
	repo, err := repository.Open(context.Background(), filepath.Join(t.TempDir(), "auth-security.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func authSecurityHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func createAuthSecurityUser(t *testing.T, repo *repository.Repository, username, password string, enabled bool) account.User {
	t.Helper()
	user, err := repo.CreateUser(context.Background(), account.CreateUserParams{
		Username: username, PasswordHash: authSecurityHash(t, password),
		Role: account.RoleAdmin, Enabled: enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func newAuthSecurityServer(t *testing.T, cfg config.Config, repo *repository.Repository) *Server {
	t.Helper()
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	server, err := New(cfg, repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func sessionCookieForUser(t *testing.T, server *Server, username string) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	credential := server.authSnapshot().byUsername[username]
	if credential == nil || !server.setAuthSessionForPrincipal(recorder, httptest.NewRequest(http.MethodGet, "/", nil), credential.principal(), authSessionDuration) {
		t.Fatal("could not create authentication session")
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == authSessionCookieName {
			return cookie
		}
	}
	t.Fatal("missing authentication session cookie")
	return nil
}

func TestHybridAuthLoadsDatabaseAndConfigurationAccounts(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	createAuthSecurityUser(t, repo, "database-admin", "database-password", true)
	server := newAuthSecurityServer(t, config.Config{Auth: config.AuthConfig{
		Credentials: []config.AuthCredential{{Username: "config-reader", Password: "config-password", Role: account.RoleDocumentsRead}},
	}}, repo)

	if principal, ok := server.authenticateBasic("database-admin", "database-password"); !ok || principal.Source != authSourceDatabase || principal.AccountID == 0 {
		t.Fatalf("database principal = %#v, ok=%v", principal, ok)
	}
	if principal, ok := server.authenticateBasic("config-reader", "config-password"); !ok || principal.Source != authSourceConfig || principal.Subject != "config-reader" {
		t.Fatalf("config principal = %#v, ok=%v", principal, ok)
	}
	if got := len(server.authConfigAccountViews()); got != 1 {
		t.Fatalf("config account views = %d", got)
	}
}

func TestDatabaseOnlyAuthAllowsPublicBindAndConflictFailsStartup(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	createAuthSecurityUser(t, repo, "admin", "database-password", true)
	server := newAuthSecurityServer(t, config.Config{Addr: "0.0.0.0:8080"}, repo)
	if !server.AuthEnabled() {
		t.Fatal("database-only authentication is disabled")
	}
	if _, ok := server.authenticateBasic("admin", "database-password"); !ok {
		t.Fatal("database-only account was rejected")
	}

	_, err := New(config.Config{
		Addr: "127.0.0.1:0", DataDir: t.TempDir(),
		Auth: config.AuthConfig{Username: "admin", Password: "config-password"},
	}, repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "defined by both config and database") {
		t.Fatalf("source conflict error = %v", err)
	}
}

func TestInactiveDatabaseRecordsRemainFailClosed(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	createAuthSecurityUser(t, repo, "disabled-admin", "database-password", false)
	server := newAuthSecurityServer(t, config.Config{Addr: "127.0.0.1:0"}, repo)
	if !server.AuthEnabled() {
		t.Fatal("account records must keep authentication fail-closed")
	}
	if _, ok := server.authenticateBasic("disabled-admin", "database-password"); ok {
		t.Fatal("disabled account authenticated")
	}

	_, err := New(config.Config{Addr: "0.0.0.0:8080", DataDir: t.TempDir()}, repo, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "active authentication account") {
		t.Fatalf("public inactive-only startup error = %v", err)
	}
}

func TestSessionV2RejectsLegacyAndTracksDatabaseRevision(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	user := createAuthSecurityUser(t, repo, "admin", "old-password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	cookie := sessionCookieForUser(t, server, user.Username)

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookie)
	if _, ok := server.authSessionPrincipal(request); !ok {
		t.Fatal("fresh v2 session was rejected")
	}
	payload, ok := server.verifyAuthSession(cookie.Value)
	if !ok || payload.Version != 2 || payload.Source != authSourceDatabase || payload.Subject == "" || payload.Revision != "1" {
		t.Fatalf("session payload = %#v, ok=%v", payload, ok)
	}

	legacyJSON, err := json.Marshal(struct {
		User    string `json:"u"`
		Expires int64  `json:"e"`
	}{User: user.Username, Expires: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	legacy := base64.RawURLEncoding.EncodeToString(legacyJSON) + "." + base64.RawURLEncoding.EncodeToString(authSessionSignature(server.authKey, legacyJSON))
	if _, ok := server.verifyAuthSession(legacy); ok {
		t.Fatal("legacy session payload was accepted")
	}
	if _, err := server.signAuthSession(authSessionPayload{User: user.Username, Expires: time.Now().Add(time.Hour).Unix()}); err == nil {
		t.Fatal("legacy payload was auto-upgraded while signing")
	}

	updated, err := repo.UpdateUserPassword(context.Background(), user.ID, account.UpdateUserPasswordParams{
		PasswordHash: authSecurityHash(t, "new-password"), ExpectedRowVersion: user.RowVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := server.authSessionPrincipal(request); ok {
		t.Fatal("session survived password revision")
	}
	if updated.SessionVersion != 2 {
		t.Fatalf("session version = %d", updated.SessionVersion)
	}
	if _, ok := server.authenticateBasic("admin", "old-password"); ok {
		t.Fatal("old password authenticated after reload")
	}
	if _, ok := server.authenticateBasic("admin", "new-password"); !ok {
		t.Fatal("new password was rejected after reload")
	}
}

func TestConfigCredentialRevisionInvalidatesSessionAcrossRestart(t *testing.T) {
	dataDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{Addr: "127.0.0.1:0", DataDir: dataDir, Auth: config.AuthConfig{Username: "admin", Password: "old-password"}}
	first, err := New(cfg, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	cookie := sessionCookieForUser(t, first, "admin")

	unchanged, err := New(cfg, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookie)
	if _, ok := unchanged.authSessionPrincipal(request); !ok {
		t.Fatal("unchanged config invalidated session")
	}

	cfg.Auth.Password = "new-password"
	changed, err := New(cfg, nil, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := changed.authSessionPrincipal(request); ok {
		t.Fatal("changed config credential retained old session")
	}
}

func TestDeleteAndRecreateUsernameDoesNotReviveSession(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	user := createAuthSecurityUser(t, repo, "admin", "old-password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	cookie := sessionCookieForUser(t, server, user.Username)
	if _, err := repo.DeleteUser(context.Background(), user.ID, user.RowVersion, 1); err != nil {
		t.Fatal(err)
	}
	recreated := createAuthSecurityUser(t, repo, "admin", "new-password", true)
	if recreated.ID == user.ID {
		t.Fatal("recreated account unexpectedly reused stable id")
	}
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookie)
	if _, ok := server.authSessionPrincipal(request); ok {
		t.Fatal("deleted account session authenticated recreated username")
	}
}

func TestAuthFailureLimiterFixedWindowAndBoundedKeys(t *testing.T) {
	limiter := newAuthFailureLimiter()
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	for i := 0; i < authFailureLimit; i++ {
		if retry := limiter.retryAfter("exact-user"); retry != 0 {
			t.Fatalf("attempt %d unexpectedly blocked for %v", i+1, retry)
		}
		limiter.failure("exact-user")
	}
	if retry := limiter.retryAfter("exact-user"); retry != authFailureWindow {
		t.Fatalf("retry after = %v", retry)
	}
	if retry := limiter.retryAfter(" exact-user"); retry != 0 {
		t.Fatalf("different exact username blocked for %v", retry)
	}
	now = now.Add(14 * time.Minute)
	if retry := limiter.retryAfter("exact-user"); retry != time.Minute {
		t.Fatalf("blocked attempt changed fixed window, retry=%v", retry)
	}
	now = now.Add(time.Minute)
	if retry := limiter.retryAfter("exact-user"); retry != 0 {
		t.Fatalf("expired fixed window retry=%v", retry)
	}

	veryLong := strings.Repeat("x", 1<<20)
	limiter.failure(veryLong)
	for i := 0; i < authFailureMaxEntries+10; i++ {
		limiter.failure(fmt.Sprintf("user-%d", i))
	}
	if got := len(limiter.entries); got != authFailureMaxEntries {
		t.Fatalf("limiter entries = %d", got)
	}
	for key, entry := range limiter.entries {
		if entry.key != key {
			t.Fatal("limiter entry key mismatch")
		}
	}
	limiter.success("user-10")
	if got := len(limiter.entries); got != authFailureMaxEntries-1 {
		t.Fatalf("success did not clear limiter entry: %d", got)
	}
}

func TestBasicAuthRateLimitReturns429AndExpires(t *testing.T) {
	server := newAuthSecurityServer(t, config.Config{Auth: config.AuthConfig{
		Username: "admin", PasswordHash: authSecurityHash(t, "right-password"),
	}}, nil)
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	server.auth.limiter.now = func() time.Time { return now }
	handler := server.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	for i := 0; i < authFailureLimit; i++ {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetBasicAuth("admin", "wrong-password")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d status = %d", i+1, recorder.Code)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("admin", "right-password")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("blocked status=%d retry-after=%q", recorder.Code, recorder.Header().Get("Retry-After"))
	}
	now = now.Add(authFailureWindow)
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetBasicAuth("admin", "right-password")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("post-expiry status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBcryptAdmissionIsBoundedWithoutUnboundedWaiters(t *testing.T) {
	server := newAuthSecurityServer(t, config.Config{Auth: config.AuthConfig{
		Username: "admin", PasswordHash: authSecurityHash(t, "right-password"),
	}}, nil)
	for i := 0; i < cap(server.auth.bcrypt); i++ {
		server.auth.bcrypt <- struct{}{}
	}
	started := time.Now()
	if _, ok, retryAfter := server.authenticateBasicCheck("unbounded-username", "wrong-password"); ok || retryAfter != authBcryptBusyRetryAfter {
		t.Fatalf("busy authentication ok=%v retry-after=%v", ok, retryAfter)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("busy authentication waited %v", elapsed)
	}
	if got := len(server.auth.limiter.entries); got != 0 {
		t.Fatalf("capacity rejection counted as password failure: %d", got)
	}
	if _, err := server.hashAuthPassword(context.Background(), "new password long enough"); !errors.Is(err, errAuthBcryptBusy) {
		t.Fatalf("busy password hash error = %v", err)
	}
	for i := 0; i < cap(server.auth.bcrypt); i++ {
		<-server.auth.bcrypt
	}
	if _, ok, retryAfter := server.authenticateBasicCheck("admin", "right-password"); !ok || retryAfter != 0 {
		t.Fatalf("authentication after capacity release ok=%v retry-after=%v", ok, retryAfter)
	}
	generated, err := server.hashAuthPassword(context.Background(), "new password long enough")
	if err != nil {
		t.Fatal(err)
	}
	if cost, err := bcrypt.Cost([]byte(generated)); err != nil || cost != account.PasswordHashCost {
		t.Fatalf("generated bcrypt cost=%d err=%v", cost, err)
	}
}

func TestLateBasicCacheInsertAndOldPrincipalCannotUpgradeRevision(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	user := createAuthSecurityUser(t, repo, "admin", "old-password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	oldPrincipal, ok := server.authenticateBasic("admin", "old-password")
	if !ok {
		t.Fatal("initial authentication failed")
	}
	oldKey := authBasicCacheKey(server.authKey, "admin", "old-password", oldPrincipal.Source, oldPrincipal.Subject, oldPrincipal.Revision)
	if _, err := repo.UpdateUserPassword(context.Background(), user.ID, account.UpdateUserPasswordParams{
		PasswordHash: authSecurityHash(t, "new-password"), ExpectedRowVersion: user.RowVersion,
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Simulate an old bcrypt operation completing after Reload cleared the cache.
	server.auth.cache.set(oldKey, oldPrincipal)
	if _, ok := server.authenticateBasic("admin", "old-password"); ok {
		t.Fatal("late stale cache entry authenticated old password")
	}
	recorder := httptest.NewRecorder()
	if server.setAuthSessionForPrincipal(recorder, httptest.NewRequest(http.MethodGet, "/", nil), oldPrincipal, authSessionDuration) {
		t.Fatal("old principal was upgraded to current session revision")
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatal("old principal emitted a session cookie")
	}
}

func TestSelfPasswordSessionCannotUpgradeToLaterAdminRevision(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	user := createAuthSecurityUser(t, repo, "admin", "initial-password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	selfUpdated, err := repo.UpdateUserPassword(context.Background(), user.ID, account.UpdateUserPasswordParams{
		PasswordHash: authSecurityHash(t, "self-password"), ExpectedRowVersion: user.RowVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	selfPrincipal, ok := server.authPrincipalForDatabaseRevision(selfUpdated.ID, selfUpdated.Username, selfUpdated.SessionVersion)
	if !ok {
		t.Fatal("exact self-update principal unavailable")
	}
	adminUpdated, err := repo.UpdateUserPassword(context.Background(), user.ID, account.UpdateUserPasswordParams{
		PasswordHash: authSecurityHash(t, "admin-password"), ExpectedRowVersion: selfUpdated.RowVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if adminUpdated.SessionVersion == selfUpdated.SessionVersion {
		t.Fatal("second reset did not advance session revision")
	}
	recorder := httptest.NewRecorder()
	if server.setAuthSessionForPrincipal(recorder, httptest.NewRequest(http.MethodPost, "/account/password", nil), selfPrincipal, authSessionDuration) {
		t.Fatal("self-update principal was upgraded to later admin-reset revision")
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatal("stale self-update principal emitted a cookie")
	}
}

func TestPasswordConfirmationRejectsStaleRequestRevision(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	user := createAuthSecurityUser(t, repo, "admin", "same-password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	stalePrincipal, ok := server.authenticateBasic("admin", "same-password")
	if !ok {
		t.Fatal("initial authentication failed")
	}
	request := withAuthPrincipal(httptest.NewRequest(http.MethodPost, "/tags/1/delete", nil), stalePrincipal)
	if _, err := repo.UpdateUserAccess(context.Background(), user.ID, account.UpdateUserAccessParams{
		Role: account.RoleAdmin, ExpectedRowVersion: user.RowVersion,
	}, 0); err != nil {
		t.Fatal(err)
	}
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	if confirmed, retryAfter := server.authPasswordCheck(request, "same-password"); confirmed || retryAfter != 0 {
		t.Fatalf("stale step-up confirmed=%v retry-after=%v", confirmed, retryAfter)
	}
}

func TestPhotoSessionCannotUpgradeStaleBasicPrincipal(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	user := createAuthSecurityUser(t, repo, "admin", "initial-password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	stalePrincipal, ok := server.authenticateBasic("admin", "initial-password")
	if !ok {
		t.Fatal("initial authentication failed")
	}
	updated, err := repo.UpdateUserPassword(context.Background(), user.ID, account.UpdateUserPasswordParams{
		PasswordHash: authSecurityHash(t, "new-password"), ExpectedRowVersion: user.RowVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := withAuthPrincipal(httptest.NewRequest(http.MethodPost, "/photos/adminonly", nil), stalePrincipal)
	recorder := httptest.NewRecorder()
	if server.setPhotoAdminOnlyVisibilitySession(recorder, request, true) {
		t.Fatal("photo preference upgraded a stale Basic principal")
	}
	if len(recorder.Result().Cookies()) != 0 {
		t.Fatal("stale photo preference emitted a session cookie")
	}

	current, ok := server.authPrincipalForDatabaseRevision(updated.ID, updated.Username, updated.SessionVersion)
	if !ok {
		t.Fatal("current principal unavailable")
	}
	request = withAuthPrincipal(httptest.NewRequest(http.MethodPost, "/photos/adminonly", nil), current)
	recorder = httptest.NewRecorder()
	if !server.setPhotoAdminOnlyVisibilitySession(recorder, request, true) {
		t.Fatal("current Basic principal could not store photo preference")
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("photo session cookies = %d", len(cookies))
	}
	payload, valid := server.verifyAuthSession(cookies[0].Value)
	if !valid || payload.Revision != fmt.Sprint(updated.SessionVersion) || !payload.PhotoAdminOnlyShown {
		t.Fatalf("photo session payload = %#v, valid=%v", payload, valid)
	}
}

func TestReloadAuthSnapshotCompletesAfterCallerCancellation(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	user := createAuthSecurityUser(t, repo, "admin", "old-password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	if _, err := repo.UpdateUserPassword(context.Background(), user.ID, account.UpdateUserPasswordParams{
		PasswordHash: authSecurityHash(t, "new-password"), ExpectedRowVersion: user.RowVersion,
	}); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := server.ReloadAuthSnapshot(canceled); err != nil {
		t.Fatalf("reload after committed change and cancellation: %v", err)
	}
	if _, ok := server.authenticateBasic("admin", "old-password"); ok {
		t.Fatal("canceled caller left old credential active")
	}
	if _, ok := server.authenticateBasic("admin", "new-password"); !ok {
		t.Fatal("canceled caller did not publish committed credential")
	}
}

func TestReloadFailurePublishesFailClosedSnapshot(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	createAuthSecurityUser(t, repo, "admin", "password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	cookie := sessionCookieForUser(t, server, "admin")
	if _, ok := server.authenticateBasic("admin", "password"); !ok {
		t.Fatal("precondition: account did not authenticate")
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.ReloadAuthSnapshot(context.Background()); err == nil {
		t.Fatal("expected snapshot reload failure")
	}
	if !server.AuthEnabled() {
		t.Fatal("failed reload must remain enabled and fail-closed")
	}
	if _, ok := server.authenticateBasic("admin", "password"); ok {
		t.Fatal("old Basic credential remained active after reload failure")
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookie)
	if _, ok := server.authSessionPrincipal(request); ok {
		t.Fatal("old session remained active after reload failure")
	}
}

func TestConcurrentBootstrapCreatesOnlyOneFirstAdministrator(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	hash := authSecurityHash(t, "bootstrap-password")
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		username := fmt.Sprintf("bootstrap-%d", i)
		go func() {
			<-start
			results <- server.withAuthWrite(context.Background(), func() error {
				if _, err := repo.CreateUser(context.Background(), account.CreateUserParams{
					Username: username, PasswordHash: hash, Role: account.RoleAdmin, Enabled: true,
				}); err != nil {
					return err
				}
				return server.ReloadAuthSnapshot(context.Background())
			})
		}()
	}
	close(start)
	var succeeded, stale int
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, errAuthPrincipalStale):
			stale++
		default:
			t.Fatalf("bootstrap result = %v", err)
		}
	}
	if succeeded != 1 || stale != 1 {
		t.Fatalf("bootstrap results: success=%d stale=%d", succeeded, stale)
	}
}

func TestConcurrentAuthenticationAndSnapshotReload(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	createAuthSecurityUser(t, repo, "admin", "password", true)
	server := newAuthSecurityServer(t, config.Config{}, repo)
	var stopped atomic.Bool
	var wait sync.WaitGroup
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for !stopped.Load() {
				_, _ = server.authenticateBasic("admin", "password")
				_, _ = server.authenticateBasic("unknown", "password")
			}
		}()
	}
	for i := 0; i < 25; i++ {
		if err := server.ReloadAuthSnapshot(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	stopped.Store(true)
	wait.Wait()
}
