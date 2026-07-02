package documentimport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/storage"
)

func TestImporterDefersTextExtractionAndPostProcessing(t *testing.T) {
	repo := &fakeRepository{}
	store := &fakeStore{storedPath: "2026/05/test.pdf"}
	var queued document.Document
	importer := NewImporter(repo, store, nil, func(doc document.Document) {
		queued = doc
	})

	result := importer.ImportCandidate(context.Background(), storage.Candidate{
		OriginalName: "test.pdf",
		SafeName:     "test.pdf",
		TempPath:     filepath.Join(t.TempDir(), "missing.pdf"),
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "sum",
	}, document.UploadWayAPI)

	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.Created == nil {
		t.Fatalf("created = %#v", result)
	}
	doc := result.Created.Document
	if doc.ContentText != "" || doc.ContentTextSource != document.ContentTextSourceNone {
		t.Fatalf("content text was extracted synchronously: %#v", doc)
	}
	if doc.ID == 0 || queued.ID != doc.ID {
		t.Fatalf("post import doc not queued: created=%#v queued=%#v", doc, queued)
	}
	if repo.created.ContentText != "" || repo.created.ContentTextSource != document.ContentTextSourceNone {
		t.Fatalf("created doc = %#v", repo.created)
	}
}

func TestImporterImportCandidateWithTagsPassesInitialTags(t *testing.T) {
	repo := &fakeRepository{}
	store := &fakeStore{storedPath: "2026/05/test.pdf"}
	importer := NewImporter(repo, store, nil, nil)

	result := importer.ImportCandidateWithTags(context.Background(), storage.Candidate{
		OriginalName: "test.pdf",
		SafeName:     "test.pdf",
		TempPath:     filepath.Join(t.TempDir(), "missing.pdf"),
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "sum-tags",
	}, document.UploadWayWebDAV, []string{"steuer", "privat"})

	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.Created == nil {
		t.Fatalf("created = %#v", result)
	}
	if got, want := repo.created.Tags, []string{"steuer", "privat"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("created tags = %#v, want %#v", got, want)
	}
}

func TestImporterImportCandidateWithOptionsAppliesMetadata(t *testing.T) {
	repo := &fakeRepository{}
	store := &fakeStore{storedPath: "2026/05/mail.pdf"}
	importer := NewImporter(repo, store, nil, nil)
	docDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	result := importer.ImportCandidateWithOptions(context.Background(), storage.Candidate{
		OriginalName: "mail.pdf",
		SafeName:     "mail.pdf",
		TempPath:     filepath.Join(t.TempDir(), "missing.pdf"),
		MIMEType:     "application/pdf",
		SizeBytes:    10,
		SHA256:       "sum-options",
	}, ImportOptions{
		UploadWay:    document.UploadWayMail,
		Title:        "Rechnung Juli",
		Description:  "E-Mail-Archiv",
		DocumentDate: &docDate,
		Tags:         []string{"mail"},
	})

	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if repo.created.UploadWay != document.UploadWayMail || repo.created.Title != "Rechnung Juli" || repo.created.Description != "E-Mail-Archiv" {
		t.Fatalf("created metadata = %#v", repo.created)
	}
	if repo.created.DocumentDate == nil || !repo.created.DocumentDate.Equal(docDate) {
		t.Fatalf("document date = %#v", repo.created.DocumentDate)
	}
	if len(repo.created.Tags) != 1 || repo.created.Tags[0] != "mail" {
		t.Fatalf("tags = %#v", repo.created.Tags)
	}
}

func TestPostProcessorUpdatesSearchTextAndThumbnail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("invoice text"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepository{}
	store := &fakeStore{resolvedPath: path}
	thumbnails := &fakeThumbnailer{}
	processor := NewPostProcessor(repo, store, thumbnails, nil, nil)

	if err := processor.Process(context.Background(), document.Document{
		ID:           7,
		OriginalName: "note.txt",
		StoredPath:   "2026/05/note.txt",
		MIMEType:     "text/plain; charset=utf-8",
	}); err != nil {
		t.Fatal(err)
	}

	if repo.updatedID != 7 || repo.updatedText != "invoice text" || repo.updatedSource != document.ContentTextSourceFile {
		t.Fatalf("updated search text = id:%d text:%q source:%q", repo.updatedID, repo.updatedText, repo.updatedSource)
	}
	if !thumbnails.ensured {
		t.Fatal("thumbnail was not requested")
	}
}

func TestPostProcessorEnqueueSignalsPendingScan(t *testing.T) {
	processor := NewPostProcessor(&fakeRepository{}, &fakeStore{}, nil, nil, nil)
	processor.Enqueue(document.Document{ID: 999, OriginalName: "pending.pdf"})

	select {
	case <-processor.wake:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("post import scan was not signaled")
	}
}

func TestPostProcessorEnqueueReturnsWhenWakeSignalIsAlreadyPending(t *testing.T) {
	processor := NewPostProcessor(&fakeRepository{}, &fakeStore{}, nil, nil, nil)
	processor.wake <- struct{}{}

	done := make(chan struct{})
	go func() {
		processor.Enqueue(document.Document{ID: 999, OriginalName: "pending.pdf"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Enqueue blocked when a wake signal was already pending")
	}
}

func TestPostProcessorProcessPendingMarksCompleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("invoice text"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepository{
		pending: []document.Document{{
			ID:           7,
			OriginalName: "note.txt",
			StoredPath:   "2026/05/note.txt",
			MIMEType:     "text/plain; charset=utf-8",
		}},
	}
	processor := NewPostProcessor(repo, &fakeStore{resolvedPath: path}, &fakeThumbnailer{}, nil, nil)

	attempted, err := processor.ProcessPending(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if attempted != 1 {
		t.Fatalf("attempted = %d, want 1", attempted)
	}
	if repo.updatedID != 7 || repo.updatedText != "invoice text" || repo.updatedSource != document.ContentTextSourceFile {
		t.Fatalf("updated search text = id:%d text:%q source:%q", repo.updatedID, repo.updatedText, repo.updatedSource)
	}
	if len(repo.completedIDs) != 1 || repo.completedIDs[0] != 7 {
		t.Fatalf("completed ids = %#v, want [7]", repo.completedIDs)
	}
	if len(repo.attemptedIDs) != 1 || repo.attemptedIDs[0] != 7 {
		t.Fatalf("attempted ids = %#v, want [7]", repo.attemptedIDs)
	}
}

func TestPostProcessorProcessPendingKeepsFailedDocumentPending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("invoice text"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepository{
		pending: []document.Document{{
			ID:           7,
			OriginalName: "note.txt",
			StoredPath:   "2026/05/note.txt",
			MIMEType:     "text/plain; charset=utf-8",
		}},
	}
	processor := NewPostProcessor(repo, &fakeStore{resolvedPath: path}, &fakeThumbnailer{err: errors.New("thumbnail failed")}, nil, nil)

	attempted, err := processor.ProcessPending(context.Background(), 10)
	if err == nil {
		t.Fatal("expected processing error")
	}
	if attempted != 1 {
		t.Fatalf("attempted = %d, want 1", attempted)
	}
	if len(repo.completedIDs) != 0 {
		t.Fatalf("completed ids = %#v, want none", repo.completedIDs)
	}
	if len(repo.attemptedIDs) != 1 || repo.attemptedIDs[0] != 7 {
		t.Fatalf("attempted ids = %#v, want [7]", repo.attemptedIDs)
	}
}

func TestPostProcessorCompletesPlainTextDocumentWhenSofficeIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("invoice text"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepository{
		pending: []document.Document{{
			ID:           7,
			OriginalName: "note.txt",
			StoredPath:   "2026/05/note.txt",
			MIMEType:     "text/plain; charset=utf-8",
		}},
	}
	processor := NewPostProcessor(repo, &fakeStore{resolvedPath: path}, nil, nil, nil)

	attempted, err := processor.ProcessPending(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if attempted != 1 {
		t.Fatalf("attempted = %d, want 1", attempted)
	}
	if repo.updatedText != "invoice text" || len(repo.completedIDs) != 1 || repo.completedIDs[0] != 7 {
		t.Fatalf("text/completed with missing soffice: text=%q completed=%#v", repo.updatedText, repo.completedIDs)
	}
	if len(repo.attemptedIDs) != 1 || repo.attemptedIDs[0] != 7 {
		t.Fatalf("attempted ids = %#v, want [7]", repo.attemptedIDs)
	}
}

func TestExtractDocumentTextSources(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "note.md")
	if err := os.WriteFile(plain, []byte("# Heading\n\ninvoice text"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, source, err := ExtractDocumentText(plain, "text/markdown; charset=utf-8")
	if err != nil {
		t.Fatal(err)
	}
	if text != "# Heading invoice text" || source != document.ContentTextSourceFile {
		t.Fatalf("plain text source = text:%q source:%q", text, source)
	}

	raw := filepath.Join(dir, "note.bin")
	if err := os.WriteFile(raw, []byte("binary fallback text"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, source, err = ExtractDocumentText(raw, "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	if text != "binary fallback text" || source != document.ContentTextSourceRaw {
		t.Fatalf("raw fallback source = text:%q source:%q", text, source)
	}
}

func TestExtractDocumentTextUsesFileSourceForOfficeDocuments(t *testing.T) {
	installFakeSoffice(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "note.rtf")
	if err := os.WriteFile(path, []byte(`{\rtf1\ansi invoice text}`), 0o600); err != nil {
		t.Fatal(err)
	}

	text, source, err := ExtractDocumentText(path, "application/rtf")
	if err != nil {
		t.Fatal(err)
	}
	if text == "" || source != document.ContentTextSourceFile {
		t.Fatalf("office text source = text:%q source:%q", text, source)
	}
}

func installFakeSoffice(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "soffice")
	if err := os.WriteFile(script, []byte(`#!/bin/sh
format=""
outdir=""
source=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		--convert-to)
			shift
			format="$1"
			;;
		--outdir)
			shift
			outdir="$1"
			;;
		*)
			source="$1"
			;;
	esac
	shift
done
base=$(basename "$source")
stem=${base%.*}
case "$format" in
	txt*) cp "$source" "$outdir/$stem.txt" ;;
	pdf*) printf '%s' '%PDF fake' > "$outdir/$stem.pdf" ;;
	*) exit 2 ;;
esac
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

type fakeRepository struct {
	existing      document.Document
	hasExisting   bool
	created       document.Document
	updatedID     int64
	updatedText   string
	updatedSource string
	pending       []document.Document
	pendingErr    error
	completedIDs  []int64
	attemptedIDs  []int64
	markErr       error
}

func (f *fakeRepository) PostImportPendingDocuments(_ context.Context, limit int) ([]document.Document, error) {
	if f.pendingErr != nil {
		return nil, f.pendingErr
	}
	pending := f.pending
	if limit > 0 && limit < len(pending) {
		pending = pending[:limit]
	}
	return append([]document.Document(nil), pending...), nil
}

func (f *fakeRepository) MarkPostImportComplete(_ context.Context, id int64) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.completedIDs = append(f.completedIDs, id)
	return nil
}

func (f *fakeRepository) MarkPostImportAttempted(_ context.Context, id int64) error {
	f.attemptedIDs = append(f.attemptedIDs, id)
	return nil
}

func (f *fakeRepository) FindActiveByChecksum(context.Context, string) (document.Document, bool, error) {
	return f.existing, f.hasExisting, nil
}

func (f *fakeRepository) CreateDocument(_ context.Context, doc document.Document) (int64, error) {
	f.created = doc
	return 42, nil
}

func (f *fakeRepository) UpdateSearchText(_ context.Context, id int64, text, source string, _ int) error {
	f.updatedID = id
	f.updatedText = text
	f.updatedSource = source
	return nil
}

type fakeStore struct {
	storedPath   string
	resolvedPath string
}

func (f *fakeStore) Commit(storage.Candidate, time.Time) (string, error) {
	return f.storedPath, nil
}

func (f *fakeStore) Delete(string) error {
	return nil
}

func (f *fakeStore) RemoveTemp(storage.Candidate) {
}

func (f *fakeStore) Resolve(string) (string, error) {
	return f.resolvedPath, nil
}

type fakeThumbnailer struct {
	ensured bool
	err     error
}

func (f *fakeThumbnailer) Ensure(context.Context, document.Document) error {
	f.ensured = true
	return f.err
}
