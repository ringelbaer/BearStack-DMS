package server

import (
	"context"
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
			ocr := first.ocr.(*ocrService)
			mail := first.mail.(*mailImportService)
			if cap(thumb.jobs) != 1 || cap(ocr.wake) != 1 {
				t.Fatal("worker notification channels missing")
			}
			if mail.importer.Repo != s.apps.documents.importer.Repo || mail.importer.Store != s.apps.documents.importer.Store || mail.importer.AfterCreate == nil {
				t.Fatal("mail and HTTP imports do not share their dependencies")
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
		AfterCreate: func(doc document.Document) { created <- doc.ID },
	}
	if s.thumbnailService() != thumb || s.ocrService() != ocr {
		t.Fatal("injected service replaced")
	}
	s.mailImportService().(*mailImportService).importer.AfterCreate(document.Document{ID: 42})
	select {
	case id := <-created:
		if id != 42 {
			t.Fatalf("import hook ID = %d", id)
		}
	default:
		t.Fatal("injected importer callback replaced")
	}
	if err := s.ensureDocumentThumbnail(context.Background(), document.Document{}); err != nil {
		t.Fatal(err)
	}
}
