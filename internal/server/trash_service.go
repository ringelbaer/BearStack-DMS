// Datei verwaltet Papierkorb-Operationen und sichere Wiederherstellung oder endgueltiges Loeschen.
package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

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
	retry := time.NewTicker(time.Minute)
	defer retry.Stop()
	t.retryFileDeletions(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-retry.C:
			t.retryFileDeletions(ctx)
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
		if err := t.DeletePurgedDocumentFiles(ctx, doc.ID); err != nil {
			logWarn(t.log, "purged files queued for retry", "id", doc.ID, "error", err)
		}
	}
	return len(docs), nil
}

func (t *trashService) retryFileDeletions(ctx context.Context) {
	if err := t.RetryFileDeletions(ctx); err != nil && ctx.Err() == nil {
		logWarn(t.log, "purged file cleanup failed", "error", err)
	}
}

func (t *trashService) RetryFileDeletions(ctx context.Context) error {
	var afterID int64
	var firstErr error
	for {
		jobs, err := t.repo.PendingFileDeletions(ctx, afterID, 100)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			return firstErr
		}
		for _, job := range jobs {
			if err := ctx.Err(); err != nil {
				return err
			}
			afterID = job.DocumentID
			if err := t.DeletePurgedDocumentFiles(ctx, job.DocumentID); err != nil {
				logWarn(t.log, "purged files queued for retry", "id", job.DocumentID, "error", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
}

func (t *trashService) DeletePurgedDocumentFiles(ctx context.Context, id int64) error {
	// The per-document gate serializes cleanup and renderers without blocking
	// unrelated documents. Waiting for it also honors request cancellation.
	releaseFiles, err := t.store.AcquireDocumentFiles(ctx, id)
	if err != nil {
		return err
	}
	defer releaseFiles()
	job, err := t.repo.FileDeletion(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := filepath.ToSlash(filepath.Join(".purge", fmt.Sprint(id)))
	paths := []string{job.StoredPath, job.ThumbnailPath, documentOfficePreviewPath(id), filepath.ToSlash(filepath.Join(".thumbnails", fmt.Sprintf("%d.jpg", id)))}
	if !job.Staged {
		for i, path := range paths {
			if path == "" {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := t.store.StageDeletion(path, fmt.Sprintf("%s/%d", dir, i)); err != nil {
				return err
			}
		}
		// Persist this phase before removing quarantine files. On restart, even
		// an empty quarantine must never cause another lookup of original paths.
		if err := t.repo.StageFileDeletion(ctx, id); err != nil {
			return err
		}
	}
	for i := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.store.Delete(fmt.Sprintf("%s/%d", dir, i)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := t.store.Delete(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return t.repo.CompleteFileDeletion(ctx, id)
}
