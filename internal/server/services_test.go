package server

import (
	"context"
	"strings"
	"sync"
	"testing"

	"bearstack/internal/document"
)

func TestServicesShareInstancesAcrossConcurrentFirstUse(t *testing.T) {
	for _, eager := range []bool{false, true} {
		name := "partial server"
		if eager {
			name = "production constructor"
		}
		t.Run(name, func(t *testing.T) {
			s := &Server{}
			if eager {
				s = faceTestServer(t)
			}
			type instances struct {
				thumbnail thumbnailRunner
				preview   officePreviewRunner
				ocr       ocrRunner
				mail      mailImportRunner
				post      *documentPostProcessor
				trash     *trashService
			}
			results := make(chan instances, 32)
			start := make(chan struct{})
			var callers sync.WaitGroup
			for i := range 32 {
				callers.Go(func() {
					<-start
					// Exercise different entry points racing to construct dependencies.
					if i%2 == 0 {
						s.documentImporter()
					} else {
						s.mailImportService()
					}
					results <- instances{s.thumbnailService(), s.officePreviewService(), s.ocrService(), s.mailImportService(), s.documentPostProcessor(), s.trashService()}
				})
			}
			close(start)
			callers.Wait()
			close(results)
			first := <-results
			for got := range results {
				if got != first {
					t.Fatal("services changed between callers")
				}
			}
			thumb := first.thumbnail.(*thumbnailService)
			if first.preview != thumb {
				t.Fatal("default office previews do not share the thumbnail service")
			}
			if cap(thumb.jobs) != 1 {
				t.Fatal("thumbnail job channel missing")
			}
			if s.background.active != 0 || s.jobCtx != nil {
				t.Fatal("construction started background work")
			}
		})
	}
}

func TestServicesPreserveInjectedDependenciesAndImportHook(t *testing.T) {
	thumb := &thumbnailRunnerStub{}
	ocr := &ocrRunnerStub{}
	fixture := faceTestServer(t)
	s := &Server{repo: fixture.repo, store: fixture.store, cfg: fixture.cfg}
	s.apps.documents.thumbnails = thumb
	s.apps.documents.ocr = ocr
	created := make(chan int64, 1)
	s.apps.documents.importer = documentImporter{
		Repo:        s.repo,
		Store:       s.store,
		AfterCreate: func(doc document.Document) { created <- doc.ID },
	}
	if s.thumbnailService() != thumb || s.ocrService() != ocr {
		t.Fatal("injected service replaced")
	}
	candidate, err := s.store.ReceiveReader("hook.pdf", strings.NewReader("%PDF-1.4\nhook test"), 1024)
	if err != nil {
		t.Fatal(err)
	}
	// Both the HTTP importer and mail service must preserve the injected hook.
	httpResult := s.documentImporter().ImportCandidate(context.Background(), candidate, document.UploadWayWeb)
	if httpResult.Error != nil {
		t.Fatal(httpResult.Error)
	}
	if httpResult.Created == nil {
		t.Fatalf("HTTP import = %#v", httpResult)
	}
	select {
	case <-created:
	default:
		t.Fatal("HTTP importer did not call the injected hook")
	}
	message := "From: billing@example.com\r\nSubject: hook\r\nMIME-Version: 1.0\r\nContent-Type: application/pdf; name=hook.pdf\r\nContent-Disposition: attachment; filename=hook.pdf\r\n\r\n%PDF-1.4\nmail hook test"
	result, err := s.mailImportService().ImportMessage(context.Background(), strings.NewReader(message), "")
	if err != nil || result.Uploaded != 1 {
		t.Fatalf("mail import = %#v, %v", result, err)
	}
	select {
	case id := <-created:
		if id <= 0 || id == httpResult.Created.Document.ID {
			t.Fatalf("import hook ID = %d", id)
		}
	default:
		t.Fatal("injected importer callback replaced")
	}
	if err := s.ensureDocumentThumbnail(context.Background(), document.Document{}); err != nil {
		t.Fatal(err)
	}
}
