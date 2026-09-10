// Datei initialisiert und haelt die fachlichen Services, die Handler gemeinsam nutzen.
package server

import (
	"context"
	"io"

	"bearstack/internal/document"
)

type settingReader interface {
	GetSetting(context.Context, string) (string, bool, error)
}

type settingWriter interface {
	SaveSettings(context.Context, map[string]string) error
}

type mailImportRunner interface {
	Run(context.Context)
	ImportPDFs(context.Context, document.MailImportSettings) (mailImportRunResult, error)
	CheckSettings(context.Context, document.MailImportSettings) error
	RecordAudit(context.Context, string, string, int)
	ImportMessage(context.Context, io.Reader, string) (mailMessageImportResult, error)
}

type ocrRunner interface {
	RunQueue(context.Context)
	Enqueue(int64)
	Document(context.Context, document.Document, string, ocrProgressFunc) (string, error)
	PrepareDocument(document.Document) (string, error)
}

type thumbnailRunner interface {
	EnsureAll(context.Context) error
	Ensure(context.Context, document.Document) error
}

type officePreviewRunner interface {
	EnsureOfficePreview(context.Context, document.Document) (string, error)
}

type serverApplications struct {
	settings   appSettingsState
	documents  documentApplication
	mail       mailApplication
	photo      photoApplication
	statistics statisticsCacheState
}

type documentApplication struct {
	importer   documentImporter
	postImport *documentPostProcessor
	ocr        ocrRunner
	thumbnails thumbnailRunner
	previews   officePreviewRunner
	trash      trashService
	counts     documentCountCache
}

type mailApplication struct {
	importer mailImportRunner
}

type photoApplication struct {
	jobs     chan struct{}
	settings photoSettingsState
}

// initServices is the single construction path for production and partial test
// servers. It performs no I/O and starts no workers. Overrides must be supplied
// before first use; dependencies and instances then remain stable.
func (s *Server) initServices() {
	s.servicesOnce.Do(func() {
		docs := &s.apps.documents
		if docs.thumbnails == nil {
			docs.thumbnails = newThumbnailService(s.repo, s.store, s.log, make(chan struct{}, 1))
		}
		if docs.previews == nil {
			if previewer, ok := docs.thumbnails.(officePreviewRunner); ok {
				docs.previews = previewer
			} else {
				docs.previews = newThumbnailService(s.repo, s.store, s.log, nil)
			}
		}
		if docs.ocr == nil {
			docs.ocr = newOCRService(s.repo, s.store, s.log, s.invalidateDocumentCountCache, s.recordAuditLog)
		}
		if docs.postImport == nil {
			docs.postImport = newDocumentPostProcessor(s.repo, s.store, docs.thumbnails, s.log, s.invalidateDocumentCountCache)
		}
		if docs.importer.Repo == nil && docs.importer.Store == nil {
			docs.importer = newDocumentImporter(s.repo, s.store, s.log, s.afterDocumentCreate)
		}
		if s.apps.mail.importer == nil {
			s.apps.mail.importer = newMailImportService(s.cfg.MaxUploadBytes, s.repo, s.store, s.log, docs.importer, s.recordAuditLog)
		}
		if docs.trash.repo == nil && docs.trash.store == nil {
			docs.trash = newTrashService(s.repo, s.store, s.log, s.trashRetentionDays, s.invalidateDocumentCountCache)
		}
	})
}

func (s *Server) thumbnailService() thumbnailRunner {
	s.initServices()
	return s.apps.documents.thumbnails
}

func (s *Server) officePreviewService() officePreviewRunner {
	s.initServices()
	return s.apps.documents.previews
}

func (s *Server) ocrService() ocrRunner {
	s.initServices()
	return s.apps.documents.ocr
}

func (s *Server) mailImportService() mailImportRunner {
	s.initServices()
	return s.apps.mail.importer
}

func (s *Server) trashService() *trashService {
	s.initServices()
	return &s.apps.documents.trash
}

func (s *Server) documentImporter() documentImporter {
	s.initServices()
	return s.apps.documents.importer
}

func (s *Server) documentPostProcessor() *documentPostProcessor {
	s.initServices()
	return s.apps.documents.postImport
}
