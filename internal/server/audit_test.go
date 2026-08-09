package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"bearstack/internal/document"
	"bearstack/internal/repository"
)

func TestAuditWriteActionsLogsMatchedRoute(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /documents/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/documents/42/metadata", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	server.auditWriteActions(mux).ServeHTTP(rec, req)

	logs, err := repo.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %#v", logs)
	}
	got := logs[0]
	if got.Action != "Dokument-Metadaten speichern" || got.Target != "Dokument #42" {
		t.Fatalf("audit target = %#v", got)
	}
	if got.Route != "POST /documents/{id}/metadata" || got.Status != http.StatusNoContent || got.Actor != "admin" {
		t.Fatalf("audit entry = %#v", got)
	}
}

func TestAuditWriteActionsUsesTargetOverride(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tags/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		setAuditTarget(r, namedAuditTarget("Tag", 215, "steuer"))
		w.WriteHeader(http.StatusSeeOther)
	})

	req := httptest.NewRequest(http.MethodPost, "/tags/215/delete", nil)
	rec := httptest.NewRecorder()
	server.auditWriteActions(mux).ServeHTTP(rec, req)

	logs, err := repo.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %#v", logs)
	}
	if got := logs[0]; got.Action != "Tag löschen" || got.Target != `Tag "steuer" (#215)` {
		t.Fatalf("audit entry = %#v", got)
	}
}

func TestAuditWriteActionsLogsDocumentTitleFromHandler(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	docID, err := repo.CreateDocument(ctx, document.Document{
		OriginalName:  "rechnung-mai.pdf",
		StoredPath:    "2026/05/rechnung-mai.pdf",
		Title:         "Rechnung Mai",
		MIMEType:      "application/pdf",
		SizeBytes:     42,
		SHA256:        "audit-title",
		SearchVersion: document.CurrentSearchVersion,
	})
	if err != nil {
		t.Fatal(err)
	}

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /documents/{id}/delete", server.handleDelete)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/documents/%d/delete", docID), nil)
	rec := httptest.NewRecorder()
	server.auditWriteActions(mux).ServeHTTP(rec, req)

	logs, err := repo.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %#v", logs)
	}
	if got := logs[0]; got.Action != "Dokument in Papierkorb verschieben" || got.Target != fmt.Sprintf(`Dokument "Rechnung Mai" (#%d)`, docID) {
		t.Fatalf("audit entry = %#v", got)
	}
}

func TestAuditActionTargetNamesUserMutations(t *testing.T) {
	tests := []struct {
		pattern string
		action  string
	}{
		{pattern: "POST /settings/users", action: "Benutzer anlegen"},
		{pattern: "POST /settings/users/{id}", action: "Benutzerrechte ändern"},
		{pattern: "POST /settings/users/{id}/password", action: "Benutzerpasswort zurücksetzen"},
		{pattern: "POST /settings/users/{id}/enable", action: "Benutzer aktivieren"},
		{pattern: "POST /settings/users/{id}/disable", action: "Benutzer deaktivieren"},
		{pattern: "POST /settings/users/{id}/delete", action: "Benutzer löschen"},
		{pattern: "POST /account/password", action: "Eigenes Passwort ändern"},
	}
	for _, test := range tests {
		t.Run(test.action, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Pattern = test.pattern
			action, target := auditActionTarget(req)
			if action != test.action || target != "" {
				t.Fatalf("action=%q target=%q, want %q and empty target", action, target, test.action)
			}
		})
	}
}

func TestAuditRejectedAccountActionsLogsEarlyAuthenticationFailure(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	credential := &authCredential{
		username:  "target-user",
		enabled:   true,
		source:    authSourceDatabase,
		subject:   "42",
		revision:  "1",
		accountID: 42,
	}
	state := &authState{
		cache:   &authBasicCache{entries: make(map[[32]byte]authBasicCacheEntry)},
		limiter: newAuthFailureLimiter(),
		bcrypt:  make(chan struct{}, 1),
	}
	state.snapshot.Store(&authSnapshot{
		enabled:    true,
		byUsername: map[string]*authCredential{credential.username: credential},
		bySubject:  map[string]*authCredential{authSubjectKey(credential.source, credential.subject): credential},
	})
	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		auth: state,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /settings/users/{id}/disable", func(http.ResponseWriter, *http.Request) {
		t.Fatal("account handler must not run after an early authentication rejection")
	})
	earlyReject := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})

	req := httptest.NewRequest(http.MethodPost, "/settings/users/42/disable", nil)
	req.SetBasicAuth("attempted-admin", "not-recorded")
	rec := httptest.NewRecorder()
	server.auditRejectedAccountActions(mux, earlyReject).ServeHTTP(rec, req)

	logs, err := repo.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %#v", logs)
	}
	got := logs[0]
	if got.Action != "Benutzer deaktivieren" || got.Target != "Benutzer:target-user" {
		t.Fatalf("audit action/target = %#v", got)
	}
	if got.Actor != "attempted-admin" || got.Status != http.StatusUnauthorized || got.Route != "POST /settings/users/{id}/disable" {
		t.Fatalf("audit entry = %#v", got)
	}
}

func TestAuditRejectedAccountActionsDoesNotDuplicateInnerAudit(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()

	server := &Server{
		repo: repo,
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /account/password", func(w http.ResponseWriter, r *http.Request) {
		setAuditTarget(r, "Benutzer:admin")
		w.WriteHeader(http.StatusSeeOther)
	})
	handler := server.auditRejectedAccountActions(mux, server.auditWriteActions(mux))

	req := httptest.NewRequest(http.MethodPost, "/account/password", nil)
	req.SetBasicAuth("admin", "not-recorded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logs, err := repo.ListAuditLogs(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %#v", logs)
	}
	if got := logs[0]; got.Action != "Eigenes Passwort ändern" || got.Target != "Benutzer:admin" || got.Status != http.StatusSeeOther {
		t.Fatalf("audit entry = %#v", got)
	}
}
