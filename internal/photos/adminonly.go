// Datei erkennt und bewertet Admin-only-Markierungen fuer Fotoordner und Medien.
package photos

import (
	"errors"
	"os"
	"path/filepath"
)

const AdminOnlyMarkerName = ".adminonly"

var errAdminOnly = errors.New("photo path is admin-only")

func ErrAdminOnly() error {
	return errAdminOnly
}

func (l *Library) FolderAdminOnly(rel string) (bool, error) {
	clean, err := CleanPath(rel)
	if err != nil {
		return false, err
	}
	abs, err := l.Resolve(clean)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, os.ErrNotExist
	}
	return l.directoryAdminOnlyFromAbs(clean, abs), nil
}

func (l *Library) MediaAdminOnly(rel string) (bool, error) {
	return l.fileAdminOnly(rel)
}

func (l *Library) MediaAdminOnlyBatch(paths []string) (map[string]bool, error) {
	result := make(map[string]bool, len(paths))
	if l == nil || len(paths) == 0 {
		return result, nil
	}
	cache := map[string]bool{}
	for _, rel := range paths {
		clean, err := CleanPath(rel)
		if err != nil {
			return nil, err
		}
		if clean == "" {
			return nil, os.ErrNotExist
		}
		parent := parentPath(clean)
		if adminOnly, ok := cache[parent]; ok {
			result[clean] = adminOnly
			continue
		}
		abs, err := l.Resolve(parent)
		if err != nil {
			return nil, err
		}
		result[clean] = directoryAdminOnlyFromAbsCached(parent, abs, cache)
	}
	return result, nil
}

func (l *Library) fileAdminOnly(rel string) (bool, error) {
	clean, err := CleanPath(rel)
	if err != nil {
		return false, err
	}
	if clean == "" {
		return false, os.ErrNotExist
	}
	abs, err := l.Resolve(parentPath(clean))
	if err != nil {
		return false, err
	}
	return l.directoryAdminOnlyFromAbs(parentPath(clean), abs), nil
}

func (l *Library) directoryAdminOnly(rel string) bool {
	abs, err := l.Resolve(rel)
	if err != nil {
		return false
	}
	return l.directoryAdminOnlyFromAbs(rel, abs)
}

func (l *Library) directoryAdminOnlyFromAbs(rel, abs string) bool {
	for {
		if adminOnlyMarkerExists(abs) {
			return true
		}
		if rel == "" {
			return false
		}
		rel = parentPath(rel)
		abs = filepath.Dir(abs)
	}
}

func directoryAdminOnlyFromAbsCached(rel, abs string, cache map[string]bool) bool {
	visited := make([]string, 0, 4)
	for {
		if adminOnly, ok := cache[rel]; ok {
			for _, path := range visited {
				cache[path] = adminOnly
			}
			return adminOnly
		}
		visited = append(visited, rel)
		if adminOnlyMarkerExists(abs) {
			for _, path := range visited {
				cache[path] = true
			}
			return true
		}
		if rel == "" {
			for _, path := range visited {
				cache[path] = false
			}
			return false
		}
		rel = parentPath(rel)
		abs = filepath.Dir(abs)
	}
}

func adminOnlyMarkerExists(absDir string) bool {
	info, err := os.Stat(filepath.Join(absDir, AdminOnlyMarkerName))
	return err == nil && !info.IsDir()
}
