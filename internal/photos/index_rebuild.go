// Datei steuert Neuaufbau und Hintergrundaktualisierung des Fotoindex.
package photos

import "context"

func (l *Library) RebuildIndex(ctx context.Context) (IndexStats, error) {
	return l.RebuildIndexWithOptions(ctx, IndexOptions{})
}

func (l *Library) RebuildIndexWithOptions(ctx context.Context, opts IndexOptions) (IndexStats, error) {
	if opts.LowPriority {
		return withLowIndexPriority(func() (IndexStats, error) {
			return l.rebuildIndex(ctx, opts)
		})
	}
	return l.rebuildIndex(ctx, opts)
}

func (l *Library) rebuildIndex(ctx context.Context, opts IndexOptions) (stats IndexStats, err error) {
	if l == nil || !l.index.available() {
		return IndexStats{}, nil
	}
	if err := ctx.Err(); err != nil {
		return IndexStats{}, err
	}
	telemetry := l.startIndexTelemetry()
	defer func() {
		l.finishIndexTelemetry(telemetry, stats, err)
	}()

	runCache, err := l.loadIndexRunCache(ctx)
	if err != nil {
		return IndexStats{}, err
	}
	queue := []indexQueueItem{{Path: "", IndexedDirCount: unknownIndexedDirCount}}
	var affectedFolders map[string]struct{}
	scannedAny := false
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		result, err := l.indexDirectoryStep(ctx, item, runCache)
		if err != nil {
			if item.Path == "" {
				return IndexStats{}, err
			}
			telemetry.SkippedFolders++
			telemetry.addError(item.Path, err)
			continue
		}
		telemetry.Files += result.Files
		telemetry.DBWrites += result.DBWrites
		if result.RootEmptySkipped {
			telemetry.SkippedFolders++
			return runCache.stats, nil
		}
		if result.Skipped {
			telemetry.SkippedFolders++
		}
		if result.Scanned {
			scannedAny = true
			telemetry.ScannedFolders++
			if affectedFolders == nil {
				affectedFolders = make(map[string]struct{})
			}
			markAffectedFolders(affectedFolders, item.Path)
			if err := slowIndexStep(ctx, opts.EntryDelay); err != nil {
				return IndexStats{}, err
			}
		}
		queue = append(queue, result.Children...)
	}
	if err := l.refreshFolderRecursiveCounts(ctx, affectedFolders); err != nil {
		return IndexStats{}, err
	}
	if err := l.refreshFolderPreviewIndex(ctx, affectedFolders); err != nil {
		return IndexStats{}, err
	}
	if scannedAny {
		if err := l.refreshPhotoStats(ctx); err != nil {
			return IndexStats{}, err
		}
	}
	if !scannedAny {
		return runCache.stats, nil
	}
	return l.cachedIndexStats(ctx)
}
