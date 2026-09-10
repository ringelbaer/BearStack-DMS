package ocrservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"bearstack/internal/document"
	"bearstack/internal/documentocr"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

type testEngine struct {
	err   error
	calls int
}

func (e *testEngine) CheckAvailable(string) error { return nil }
func (e *testEngine) Document(ctx context.Context, source, mime, lang string, progress documentocr.ProgressFunc) (string, error) {
	e.calls++
	if err := progress(1, 1, "page complete"); err != nil {
		return "", err
	}
	return "Grüße", e.err
}

func TestJobLifecycleOutsideHTTP(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		deleted bool
		status  string
	}{
		{"success", nil, false, document.OCRJobStatusCompleted},
		{"engine failure", errors.New("conversion failed"), false, document.OCRJobStatusFailed},
		{"cancelled", context.Canceled, false, document.OCRJobStatusInterrupted},
		{"deadline", context.DeadlineExceeded, false, document.OCRJobStatusFailed},
		{"deleted", nil, true, document.OCRJobStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			repo, err := repository.Open(ctx, filepath.Join(dir, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer repo.Close()
			store, err := storage.New(filepath.Join(dir, "documents"))
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(dir, "documents", "test.pdf")
			if err := os.WriteFile(source, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
			id, err := repo.CreateDocument(ctx, document.Document{OriginalName: "test.pdf", StoredPath: "test.pdf", MIMEType: "application/pdf", Title: "test"})
			if err != nil {
				t.Fatal(err)
			}
			job, _, err := repo.EnqueueOCRJob(ctx, id, "deu", "de")
			if err != nil {
				t.Fatal(err)
			}
			if tc.deleted {
				if err := repo.SoftDelete(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			engine := &testEngine{err: tc.err}
			invalidated := 0
			var actions []string
			service := New(repo, store, nil, engine, func() { invalidated++ }, func(_ context.Context, action string, got document.OCRJob, doc document.Document, status int, detail string) {
				if got.ID != job.ID || doc.ID != id {
					t.Error("audit lost job/document identity")
				}
				actions = append(actions, action)
			})
			service.RunJob(ctx, job.ID)
			finished, err := repo.GetOCRJob(ctx, job.ID)
			if err != nil || finished.Status != tc.status {
				t.Fatalf("job = %#v, %v", finished, err)
			}
			doc, err := repo.GetDocument(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if tc.status == document.OCRJobStatusCompleted {
				if doc.ContentText != "Grüße" || doc.ContentTextSource != document.ContentTextSourceOCR || finished.TextLength != 5 || invalidated != 1 {
					t.Fatalf("OCR text was not committed: %#v, %#v, invalidations=%d", doc, finished, invalidated)
				}
			} else if doc.ContentText != "" || invalidated != 0 {
				t.Fatal("failed job published text or invalidated counts")
			}
			if len(actions) == 0 {
				t.Fatal("missing audit event")
			}
			if tc.deleted && engine.calls != 0 {
				t.Fatal("deleted document reached OCR engine")
			}
		})
	}
}

type queueRepository struct {
	Repository
	interrupted bool
	limit       int
}

func (r *queueRepository) InterruptActiveOCRJobs(context.Context, string) error {
	r.interrupted = true
	return nil
}
func (r *queueRepository) QueuedOCRJobIDs(ctx context.Context, limit int) ([]int64, error) {
	r.limit = limit
	return nil, ctx.Err()
}

func TestQueueCancellationAndBoundedWakeup(t *testing.T) {
	repo := &queueRepository{}
	service := New(repo, nil, nil, nil, nil, nil)
	for id := int64(1); id <= 1000; id++ {
		service.Enqueue(id)
	}
	if len(service.wake) != 1 {
		t.Fatal("wakeups are not coalesced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.RunQueue(ctx)
	if !repo.interrupted || repo.limit != ocrQueueBatchSize {
		t.Fatalf("queue initialization/batch limit lost: %#v", repo)
	}
}
