package server

import (
	"context"
	"testing"

	"bearstack/internal/document"
)

type ocrRunnerStub struct {
	runQueueCalls int
	enqueuedIDs   []int64
	prepareDoc    document.Document
	preparePath   string
	prepareErr    error
}

func (s *ocrRunnerStub) RunQueue(context.Context) {}

func (s *ocrRunnerStub) Enqueue(id int64) {
	s.enqueuedIDs = append(s.enqueuedIDs, id)
}

func (s *ocrRunnerStub) Document(context.Context, document.Document, string, ocrProgressFunc) (string, error) {
	return "", nil
}

func (s *ocrRunnerStub) PrepareDocument(doc document.Document) (string, error) {
	s.prepareDoc = doc
	return s.preparePath, s.prepareErr
}

func TestDocumentProcessingWrappersDelegateToConfiguredServices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	doc := document.Document{ID: 77, OriginalName: "doc.pdf"}

	thumbnailCalled := 0
	thumb := thumbnailRunnerStub{
		ensure: func(runCtx context.Context, got document.Document) error {
			thumbnailCalled++
			if runCtx != ctx {
				t.Errorf("thumbnail context mismatch")
			}
			if got.ID != doc.ID {
				t.Errorf("thumbnail document = %#v", got)
			}
			return nil
		},
	}
	ocr := &ocrRunnerStub{
		preparePath: "/tmp/prepared.pdf",
	}
	server := &Server{
		apps: serverApplications{
			documents: documentApplication{
				thumbnails: thumb,
				ocr:        ocr,
			},
		},
	}

	if err := server.ensureDocumentThumbnail(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if thumbnailCalled != 1 {
		t.Fatalf("thumbnail ensure calls = %d, want 1", thumbnailCalled)
	}

	server.enqueueOCRJob(123)
	if len(ocr.enqueuedIDs) != 1 || ocr.enqueuedIDs[0] != 123 {
		t.Fatalf("enqueued ids = %#v", ocr.enqueuedIDs)
	}

	prepared, err := server.prepareOCRDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if prepared != "/tmp/prepared.pdf" {
		t.Fatalf("prepared path = %q", prepared)
	}
	if ocr.prepareDoc.ID != doc.ID {
		t.Fatalf("prepared document = %#v", ocr.prepareDoc)
	}
}
