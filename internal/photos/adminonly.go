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
	return checkDirectoryAdminOnly(clean, abs)
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
		private, err := directoryAdminOnlyFromAbsCached(parent, abs, cache)
		if err != nil {
			return nil, err
		}
		result[clean] = private
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
	return checkDirectoryAdminOnly(parentPath(clean), abs)
}

func (l *Library) directoryAdminOnly(rel string) bool {
	abs, err := l.Resolve(rel)
	if err != nil {
		return true
	}
	return l.directoryAdminOnlyFromAbs(rel, abs)
}

func (l *Library) directoryAdminOnlyFromAbs(rel, abs string) bool {
	private, err := checkDirectoryAdminOnly(rel, abs)
	return private || err != nil
}

func checkDirectoryAdminOnly(rel, abs string) (bool, error) {
	for {
		private, err := checkAdminOnlyMarker(abs)
		if private || err != nil {
			return private, err
		}
		if rel == "" {
			return false, nil
		}
		rel = parentPath(rel)
		abs = filepath.Dir(abs)
	}
}

func directoryAdminOnlyFromAbsCached(rel, abs string, cache map[string]bool) (bool, error) {
	visited := make([]string, 0, 4)
	for {
		if adminOnly, ok := cache[rel]; ok {
			for _, path := range visited {
				cache[path] = adminOnly
			}
			return adminOnly, nil
		}
		visited = append(visited, rel)
		private, err := checkAdminOnlyMarker(abs)
		if err != nil {
			return false, err
		}
		if private {
			for _, path := range visited {
				cache[path] = true
			}
			return true, nil
		}
		if rel == "" {
			for _, path := range visited {
				cache[path] = false
			}
			return false, nil
		}
		rel = parentPath(rel)
		abs = filepath.Dir(abs)
	}
}

func adminOnlyMarkerExists(absDir string) bool {
	private, err := checkAdminOnlyMarker(absDir)
	return private || err != nil
}

func checkAdminOnlyMarker(absDir string) (bool, error) {
	// A marker symlink protects the directory even if its target is missing or
	// inaccessible. Only a definitely absent marker permits public access.
	info, err := os.Lstat(filepath.Join(absDir, AdminOnlyMarkerName))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !info.IsDir(), nil
}
