// Datei verwaltet Papierkorb-Operationen und sichere Wiederherstellung oder endgueltiges Loeschen.
package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"bearstack/internal/document"
	"bearstack/internal/repository"
	"bearstack/internal/storage"
)

type trashService struct {
	repo                         *repository.Repository
	store                        *storage.Store
	log                          *slog.Logger
	retentionDays                func(context.Context) (int, error)
	invalidateDocumentCountCache func()
}

func (s *Server) trashService() *trashService {
	if s.apps.documents.trash.repo == nil && s.apps.documents.trash.store == nil {
		s.apps.documents.trash = newTrashService(s.repo, s.store, s.log, s.trashRetentionDays, s.invalidateDocumentCountCache)
	}
	return &s.apps.documents.trash
}

func newTrashService(repo *repository.Repository, store *storage.Store, log *slog.Logger, retentionDays func(context.Context) (int, error), invalidateDocumentCountCache func()) trashService {
	return trashService{
		repo:                         repo,
		store:                        store,
		log:                          log,
		retentionDays:                retentionDays,
		invalidateDocumentCountCache: invalidateDocumentCountCache,
	}
}

func (t *trashService) RunRetention(ctx context.Context) {
	if _, err := t.PurgeByRetention(ctx); err != nil {
		logWarn(t.log, "trash retention purge failed", "error", err)
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purged, err := t.PurgeByRetention(ctx)
			if err != nil {
				logWarn(t.log, "trash retention purge failed", "error", err)
			} else if purged > 0 {
				logInfo(t.log, "trash retention purged documents", "count", purged)
			}
		}
	}
}

func (t *trashService) PurgeByRetention(ctx context.Context) (int, error) {
	days, err := t.retentionDays(ctx)
	if err != nil {
		return 0, err
	}
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	docs, err := t.repo.PurgeTrashBefore(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	if len(docs) == 0 {
		return 0, nil
	}
	if t.invalidateDocumentCountCache != nil {
		t.invalidateDocumentCountCache()
	}
	for _, doc := range docs {
		t.DeletePurgedDocumentFiles(doc)
	}
	return len(docs), nil
}

func (t *trashService) DeletePurgedDocumentFiles(doc document.Document) {
	if err := t.store.Delete(doc.StoredPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		logWarn(t.log, "failed to delete purged file", "id", doc.ID, "path", doc.StoredPath, "error", err)
	}
	if doc.ThumbnailPath != "" {
		if err := t.store.Delete(doc.ThumbnailPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			logWarn(t.log, "failed to delete purged thumbnail", "id", doc.ID, "path", doc.ThumbnailPath, "error", err)
		}
	}
	if doc.ID > 0 {
		if err := t.store.Delete(documentOfficePreviewPath(doc.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			logWarn(t.log, "failed to delete purged preview", "id", doc.ID, "error", err)
		}
	}
}
