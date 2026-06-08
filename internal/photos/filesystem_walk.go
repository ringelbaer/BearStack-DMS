// Datei stellt gemeinsame Dateisystem-Walks fuer Foto-Baeume bereit.
package photos

import (
	"context"
	"os"
	"path/filepath"
)

type photoFilesystemWalkOptions struct {
	Root             string
	IncludeAdminOnly bool
	OnAdminOnlyDir   func()
}

func walkPhotoFilesystem(ctx context.Context, opts photoFilesystemWalkOptions, visit func(path string, entry os.DirEntry, kind string) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return filepath.WalkDir(opts.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != opts.Root && ignoredName(entry.Name()) {
				return filepath.SkipDir
			}
			if !opts.IncludeAdminOnly && path != opts.Root && adminOnlyMarkerExists(path) {
				if opts.OnAdminOnlyDir != nil {
					opts.OnAdminOnlyDir()
				}
				return filepath.SkipDir
			}
			return nil
		}
		if ignoredName(entry.Name()) {
			return nil
		}
		kind, ok := supportedKind(entry.Name())
		if !ok {
			return nil
		}
		return visit(path, entry, kind)
	})
}

func walkFilesystemFiles(ctx context.Context, root string, visit func(entry os.DirEntry) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return visit(entry)
	})
}
