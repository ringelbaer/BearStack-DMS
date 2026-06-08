package server

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"bearstack/internal/config"
	"bearstack/internal/document"
	"bearstack/internal/mailimport"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
	"bearstack/internal/uploadlimit"
)

func TestImportPDFsFromMailImportsPDFAttachment(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	message := strings.NewReader(strings.Join([]string{
		"From: sender@example.com",
		"To: inbox@example.com",
		"Subject: Rechnung",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="bearstack-test"`,
		"",
		"--bearstack-test",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Anbei die Rechnung.",
		"--bearstack-test",
		`Content-Type: application/pdf; name="rechnung.pdf"`,
		`Content-Disposition: attachment; filename="rechnung.pdf"`,
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString(pdf),
		"--bearstack-test--",
		"",
	}, "\r\n"))

	result, err := server.importPDFsFromMail(ctx, message, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.PDFs != 1 || result.Uploaded != 1 || result.Errors != 0 {
		t.Fatalf("result = %#v", result)
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].OriginalName != "rechnung.pdf" || docs[0].MIMEType != "application/pdf" || docs[0].UploadWay != document.UploadWayMail {
		t.Fatalf("docs = %#v", docs)
	}
}

func TestImportPDFsFromMailReportsDuplicateAttachment(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	message := func() *strings.Reader {
		pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
		return strings.NewReader(strings.Join([]string{
			"Subject: Rechnung",
			"MIME-Version: 1.0",
			`Content-Type: multipart/mixed; boundary="bearstack-test"`,
			"",
			"--bearstack-test",
			`Content-Type: application/pdf; name="rechnung.pdf"`,
			`Content-Disposition: attachment; filename="rechnung.pdf"`,
			"Content-Transfer-Encoding: base64",
			"",
			base64.StdEncoding.EncodeToString(pdf),
			"--bearstack-test--",
			"",
		}, "\r\n"))
	}

	first, err := server.importPDFsFromMail(ctx, message(), "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.importPDFsFromMail(ctx, message(), "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Uploaded != 1 || second.Duplicates != 1 || second.Uploaded != 0 || second.Errors != 0 {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}

func TestImportPDFsFromMailRejectsDisallowedSenderBeforeImport(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	store, err := storage.New(filepath.Join(t.TempDir(), "documents"))
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:   config.Config{MaxUploadBytes: 1 << 20},
		repo:  repo,
		store: store,
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n%%EOF\n")
	message := strings.NewReader(strings.Join([]string{
		"From: attacker@example.net",
		"Subject: Rechnung",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="bearstack-test"`,
		"",
		"--bearstack-test",
		`Content-Type: application/pdf; name="rechnung.pdf"`,
		`Content-Disposition: attachment; filename="rechnung.pdf"`,
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString(pdf),
		"--bearstack-test--",
		"",
	}, "\r\n"))

	result, err := server.importPDFsFromMail(ctx, message, "billing@example.com\nexample.org")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Rejected || result.PDFs != 0 || result.Uploaded != 0 || result.Errors != 0 {
		t.Fatalf("result = %#v", result)
	}

	docs, err := repo.ListDocuments(ctx, document.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Fatalf("docs = %#v", docs)
	}
}

func TestImportPDFsFromMailRejectsOversizedMessage(t *testing.T) {
	server := &Server{
		cfg: config.Config{MaxUploadBytes: 64},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	message := strings.NewReader(strings.Join([]string{
		"From: sender@example.com",
		"Subject: Zu gross",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="bearstack-test"`,
		"",
		"--bearstack-test",
		"Content-Type: text/plain; charset=utf-8",
		"",
		strings.Repeat("x", int(uploadlimit.EnvelopeLimit(server.cfg.MaxUploadBytes))+1),
		"--bearstack-test",
		`Content-Type: application/pdf; name="rechnung.pdf"`,
		`Content-Disposition: attachment; filename="rechnung.pdf"`,
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n%%EOF\n")),
		"--bearstack-test--",
		"",
	}, "\r\n"))

	_, err := server.importPDFsFromMail(context.Background(), message, "")
	if !errors.Is(err, mailimport.ErrMessageTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestImportPDFsFromMailRejectsMultipartWithoutBoundary(t *testing.T) {
	server := &Server{
		cfg: config.Config{MaxUploadBytes: 1 << 20},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	message := strings.NewReader(strings.Join([]string{
		"From: sender@example.com",
		"Subject: Kaputt",
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed",
		"",
		"body",
	}, "\r\n"))

	_, err := server.importPDFsFromMail(context.Background(), message, "")
	if err == nil || !strings.Contains(err.Error(), "Boundary") {
		t.Fatalf("error = %v", err)
	}
}

func TestMailImportSettingsFromRequestPreservesStoredPassword(t *testing.T) {
	form := url.Values{}
	form.Set("enabled", "1")
	form.Set("host", "imap.example.com")
	form.Set("port", "993")
	form.Set("security", document.MailImportSecurityTLS)
	form.Set("mailbox", "INBOX")
	form.Set("username", "user@example.com")
	form.Set("poll_interval_minutes", "15")

	req := httptest.NewRequest("POST", "/settings/mail-import/test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}

	settings, err := mailImportSettingsFromRequest(req, document.MailImportSettings{Password: "stored-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Password != "stored-secret" {
		t.Fatalf("password = %q", settings.Password)
	}
}

func TestRenderMailImportFormSetsContentTypeBeforeStatus(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		templates: templates,
		log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	req := httptest.NewRequest(http.MethodGet, "/settings/mail-import", nil)
	rec := httptest.NewRecorder()

	server.renderMailImportForm(rec, req, http.StatusBadRequest, document.MailImportSettings{}, "", "Fehler")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content type = %q", got)
	}
}

func TestMailSenderAllowedMatchesAddressAndDomain(t *testing.T) {
	allowed := "billing@example.com\nexample.org\n@trusted.test"
	cases := map[string]bool{
		"billing@example.com":       true,
		"other@example.com":         false,
		"sender@example.org":        true,
		"sender@mail.example.org":   true,
		"sender@trusted.test":       true,
		"sender@sub.trusted.test":   true,
		"sender@untrusted.test":     false,
		"not-an-address":            false,
		"attacker@example.com.test": false,
	}
	for sender, want := range cases {
		if got := mailimport.SenderAllowed(sender, allowed); got != want {
			t.Fatalf("mailimport.SenderAllowed(%q) = %v, want %v", sender, got, want)
		}
	}
}
