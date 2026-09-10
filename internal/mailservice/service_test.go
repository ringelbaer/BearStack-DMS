package mailservice

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/documentimport"
	"bearstack/internal/storage"
)

type fakeMailbox struct {
	deleted   []uint32
	loggedOut bool
}

func (m *fakeMailbox) Logout() error                    { m.loggedOut = true; return nil }
func (m *fakeMailbox) UndeletedUIDs() ([]uint32, error) { return []uint32{1, 2, 3}, nil }
func (m *fakeMailbox) FetchMessage(uid uint32) (io.Reader, error) {
	if uid == 3 {
		return nil, errors.New("fetch failed")
	}
	name := "good.pdf"
	if uid == 2 {
		name = "bad.pdf"
	}
	return strings.NewReader("From: billing@example.com\r\nSubject: invoice\r\nMIME-Version: 1.0\r\nContent-Type: application/pdf; name=" + name + "\r\nContent-Disposition: attachment; filename=" + name + "\r\n\r\n%PDF-1.4\nfixture"), nil
}
func (m *fakeMailbox) DeleteMessage(uid uint32) error { m.deleted = append(m.deleted, uid); return nil }

type fakeStore struct{}

func (fakeStore) ReceiveReader(name string, r io.Reader, limit int64) (storage.Candidate, error) {
	_, err := io.Copy(io.Discard, r)
	return storage.Candidate{OriginalName: name, MIMEType: "application/pdf"}, err
}
func (fakeStore) EnsureDir(string) (string, error) { return "", errors.New("unexpected archive") }

type fakeImporter struct{ ways []string }

func (i *fakeImporter) ImportCandidate(_ context.Context, candidate storage.Candidate, way string) documentimport.Result {
	i.ways = append(i.ways, way)
	if candidate.OriginalName == "bad.pdf" {
		return documentimport.Result{Error: errors.New("storage unavailable")}
	}
	return documentimport.Result{Created: &documentimport.Created{Document: document.Document{OriginalName: candidate.OriginalName}}}
}
func (i *fakeImporter) ImportCandidateWithOptions(context.Context, storage.Candidate, documentimport.ImportOptions) documentimport.Result {
	panic("unexpected archive")
}

func TestImportOnlyDeletesSuccessfullyProcessedMail(t *testing.T) {
	box := &fakeMailbox{}
	importer := &fakeImporter{}
	var statuses []int
	service := New(1<<20, nil, fakeStore{}, nil, importer, func(_ context.Context, action, target string, status int) { statuses = append(statuses, status) })
	service.openMailbox = func(settings document.MailImportSettings, readOnly bool) (mailbox, error) {
		if settings.Host != "imap.example.com" || settings.Port != 993 || readOnly {
			t.Fatal("mailbox settings not normalized or writable")
		}
		return box, nil
	}
	result, err := service.ImportPDFs(context.Background(), document.MailImportSettings{Host: " imap.example.com "})
	if err != nil || result.Messages != 3 || result.Uploaded != 1 || result.Errors != 2 || result.Deleted != 1 {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if !reflect.DeepEqual(box.deleted, []uint32{1}) || !box.loggedOut {
		t.Fatalf("mailbox cleanup = %#v", box)
	}
	if !reflect.DeepEqual(importer.ways, []string{document.UploadWayMail, document.UploadWayMail}) {
		t.Fatalf("upload ways = %v", importer.ways)
	}
	if !reflect.DeepEqual(statuses, []int{200, 500, 500}) {
		t.Fatalf("audit statuses = %v", statuses)
	}
}

type settingsRepository struct{ err error }

func (r settingsRepository) GetMailImportSettings(context.Context) (document.MailImportSettings, bool, error) {
	return document.MailImportSettings{}, true, r.err
}

func TestScheduledImportHandlesSettingsFailureAndCancellation(t *testing.T) {
	var status int
	service := New(1024, settingsRepository{err: errors.New("database unavailable")}, nil, nil, nil, func(_ context.Context, _, _ string, got int) { status = got })
	service.openMailbox = func(document.MailImportSettings, bool) (mailbox, error) {
		t.Fatal("settings failure must not open IMAP")
		return nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { defer close(done); service.Run(ctx) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("mail worker did not stop")
	}
	if status != 500 {
		t.Fatalf("audit status = %d", status)
	}
}
