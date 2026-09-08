package photos

import (
	"context"
	"os"
	"path/filepath"
)

// MediaBatchContext preserves input order while sharing index reads and parent
// privacy checks. Files and sidecars are still checked for changes on each call.
func (l *Library) MediaBatchContext(ctx context.Context, paths []string) ([]Media, error) {
	cleanPaths := make([]string, len(paths))
	unique := make([]string, 0, len(paths))
	byPath := make(map[string]Media, len(paths))
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		clean, err := CleanPath(path)
		if err != nil {
			return nil, err
		}
		kind, ok := supportedKind(clean)
		if !ok || !isMediaKind(kind) {
			return nil, os.ErrNotExist
		}
		cleanPaths[i] = clean
		if _, ok := byPath[clean]; !ok {
			unique = append(unique, clean)
			byPath[clean] = Media{}
		}
	}
	cache, err := l.index.mediaCacheForPaths(ctx, unique)
	if err != nil {
		return nil, err
	}
	privateDirs := make(map[string]bool)
	var changed []Media
	for _, path := range unique {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		abs, err := l.Resolve(path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		private, err := directoryAdminOnlyFromAbsCached(parentPath(path), filepath.Dir(abs), privateDirs)
		if err != nil {
			return nil, err
		}
		kind, _ := supportedKind(path)
		media, refresh, err := l.mediaFromPathInfo(path, abs, info, kind, cache, private, true)
		if err != nil {
			return nil, err
		}
		byPath[path] = media
		if refresh {
			changed = append(changed, media)
		}
	}
	// The writer retains current manual tags and invalidates stale face results.
	if err := l.saveMediaBatch(ctx, changed); err != nil {
		return nil, err
	}
	for _, media := range changed {
		byPath[media.Path] = media
	}
	items := make([]Media, len(cleanPaths))
	for i, path := range cleanPaths {
		items[i] = byPath[path]
	}
	if err := l.AddAutomaticFaces(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}
