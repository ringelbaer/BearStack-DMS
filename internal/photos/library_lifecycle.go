// Datei verwaltet Oeffnen, Initialisierung, Hintergrundjobs und Schliessen der Foto-Library.
package photos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"bearstack/internal/photos/photopath"
)

func ErrPathEscapesRoot() error {
	return photopath.ErrEscapesRoot
}

func New(root, cacheDir, dbPath string, pageSize int) (*Library, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("photo root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("photo root is not a directory: %s", absRoot)
	}

	cacheDir = strings.TrimSpace(cacheDir)
	if cacheDir == "" {
		cacheDir = filepath.Join(absRoot, ".bearstack-cache")
	}
	absCache, err := filepath.Abs(cacheDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absCache, 0o750); err != nil {
		return nil, err
	}
	index, absDBPath, err := openPhotoIndexStore(dbPath)
	if err != nil {
		return nil, err
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	library := &Library{
		root:      absRoot,
		cacheDir:  absCache,
		dbPath:    absDBPath,
		index:     index,
		pageSize:  pageSize,
		thumbnail: newThumbnailRuntime(1),
		gpxCache:  map[string]cachedGPXTrack{},
	}
	if err := library.refreshAdminOnlyIndexFlags(context.Background()); err != nil {
		_ = library.Close()
		return nil, err
	}
	return library, nil
}

func (l *Library) Root() string {
	if l == nil {
		return ""
	}
	return l.root
}

func (l *Library) CacheDir() string {
	if l == nil {
		return ""
	}
	return l.cacheDir
}

func (l *Library) DBPath() string {
	if l == nil {
		return ""
	}
	return l.dbPath
}

func (l *Library) Close() error {
	if l == nil {
		return nil
	}
	return l.index.close()
}

func (l *Library) PageSize() int {
	if l == nil || l.pageSize <= 0 {
		return defaultPageSize
	}
	return l.pageSize
}

func (l *Library) SetThumbnailConcurrency(concurrency int) {
	if l == nil {
		return
	}
	l.thumbnail.setConcurrency(concurrency)
}
