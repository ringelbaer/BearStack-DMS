// Datei kapselt Root-Pfad-Logik, damit relative Pfade kontrolliert innerhalb eines Basisverzeichnisses bleiben.
package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CleanRelativePath(value string, allowEmpty bool, escapeErr error) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" || value == "." || value == "/" {
		if allowEmpty {
			return "", nil
		}
		return "", errors.New("invalid path")
	}
	if strings.HasPrefix(value, "/") {
		return "", escapeErr
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == "." {
		if allowEmpty {
			return "", nil
		}
		return "", errors.New("invalid path")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", escapeErr
	}
	return clean, nil
}

func ResolveWithinRoot(root, rel string, allowEmpty bool, escapeErr error) (string, string, error) {
	clean, err := CleanRelativePath(rel, allowEmpty, escapeErr)
	if err != nil {
		return "", "", err
	}
	abs := filepath.Join(root, filepath.FromSlash(clean))
	if abs != root && !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		return "", "", escapeErr
	}
	if err := RejectSymlinkPath(root, clean, escapeErr); err != nil {
		return "", "", err
	}
	return clean, abs, nil
}

func RejectSymlinkPath(root, rel string, escapeErr error) error {
	if rel == "" {
		return nil
	}
	current := root
	for _, part := range strings.Split(filepath.FromSlash(rel), string(os.PathSeparator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return escapeErr
		}
	}
	return nil
}

func EnsureDirWithinRoot(root, rel string, perm os.FileMode, escapeErr error) (string, error) {
	clean, err := CleanRelativePath(rel, true, escapeErr)
	if err != nil {
		return "", err
	}
	if clean == "" {
		return root, nil
	}

	current := root
	for _, part := range strings.Split(filepath.FromSlash(clean), string(os.PathSeparator)) {
		if part == "" {
			continue
		}
		next := filepath.Join(current, part)
		info, err := os.Lstat(next)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(next, perm); err != nil {
				if !errors.Is(err, os.ErrExist) {
					return "", err
				}
				info, err = os.Lstat(next)
				if err != nil {
					return "", err
				}
			} else {
				current = next
				continue
			}
		} else if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", escapeErr
		}
		if !info.IsDir() {
			return "", fmt.Errorf("path is not a directory: %s", next)
		}
		current = next
	}
	return current, nil
}
