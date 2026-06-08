// Datei verdrahtet den Dokumentimport mit Serverkontext, Repository und Hintergrundverarbeitung.
package server

import (
	"log/slog"

	"bearstack/internal/document"
	"bearstack/internal/documentimport"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

type documentImporter = documentimport.Importer
type documentPostProcessor = documentimport.PostProcessor

func (s *Server) documentImporter() documentImporter {
	if s.apps.documents.importer.Repo != nil || s.apps.documents.importer.Store != nil {
		return s.apps.documents.importer
	}
	return newDocumentImporter(s.repo, s.store, s.log, s.afterDocumentCreate)
}

func (s *Server) documentPostProcessor() *documentPostProcessor {
	if s.apps.documents.postImport != nil {
		return s.apps.documents.postImport
	}
	s.apps.documents.postImport = newDocumentPostProcessor(s.repo, s.store, s.thumbnailService(), s.log, s.invalidateDocumentCountCache)
	return s.apps.documents.postImport
}

func (s *Server) afterDocumentCreate(doc document.Document) {
	s.invalidateDocumentCountCache()
	s.documentPostProcessor().Enqueue(doc)
}

func newDocumentImporter(repo *repository.Repository, store *storage.Store, log *slog.Logger, afterCreate func(document.Document)) documentImporter {
	return documentimport.NewImporter(repo, store, log, afterCreate)
}

func newDocumentPostProcessor(repo *repository.Repository, store *storage.Store, thumbnails thumbnailRunner, log *slog.Logger, invalidateDocumentCountCache func()) *documentPostProcessor {
	return documentimport.NewPostProcessor(repo, store, thumbnails, log, invalidateDocumentCountCache)
}
