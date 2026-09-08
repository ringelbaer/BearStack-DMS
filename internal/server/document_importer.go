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
