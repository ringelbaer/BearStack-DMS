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
