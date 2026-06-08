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
	SaveSetting(context.Context, string, string) error
}

type mailImportRunner interface {
	Run(context.Context)
	ImportPDFs(context.Context, document.MailImportSettings) (mailImportRunResult, error)
	CheckSettings(context.Context, document.MailImportSettings) error
	RecordAudit(context.Context, string, string, int)
	importPDFsFromMail(context.Context, io.Reader, string) (mailMessageImportResult, error)
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
