package photos

import (
	"errors"
	"os"
	"strings"
)

// OpenOriginal pins every parent directory and refuses symlinks, including links
// to other folders inside the library. Validate identity before reading bytes;
// pathname checks followed by os.Open cannot provide this guarantee.
// The returned descriptor is always read-only and owned by the caller.
func (l *Library) OpenOriginal(rel string, includeAdminOnly bool) (*os.File, error) {
	clean, err := CleanPath(rel)
	if err != nil {
		return nil, err
	}
	kind, ok := supportedKind(clean)
	if !ok || !isMediaKind(kind) || l == nil || l.originals == nil {
		return nil, os.ErrNotExist
	}
	return openOriginal(l.originals, strings.Split(clean, "/"), includeAdminOnly)
}

func openOriginal(root *os.Root, parts []string, includeAdminOnly bool) (*os.File, error) {
	// Keep only the current parent open, independent of path depth.
	var owned *os.Root
	defer func() {
		if owned != nil {
			owned.Close()
		}
	}()
	for i, name := range parts {
		if !includeAdminOnly {
			marker, err := root.Lstat(AdminOnlyMarkerName)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			if err == nil && !marker.IsDir() {
				return nil, errAdminOnly
			}
		}
		before, err := root.Lstat(name)
		if err != nil {
			return nil, err
		}
		if before.Mode()&os.ModeSymlink != 0 {
			return nil, ErrPathEscapesRoot()
		}
		if i < len(parts)-1 {
			if !before.IsDir() {
				return nil, os.ErrNotExist
			}
			child, err := root.OpenRoot(name)
			if err != nil {
				return nil, err
			}
			after, err := child.Stat(".")
			if err != nil || !os.SameFile(before, after) {
				child.Close()
				return nil, errors.Join(ErrPathEscapesRoot(), err)
			}
			if owned != nil {
				owned.Close()
			}
			owned, root = child, child
			continue
		}
		if !before.Mode().IsRegular() {
			return nil, os.ErrNotExist
		}
		// Unix flags also prevent a swapped FIFO from blocking the request.
		file, err := root.OpenFile(name, os.O_RDONLY|sidecarOpenFlags, 0)
		if err != nil {
			return nil, err
		}
		after, err := file.Stat()
		if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
			file.Close()
			return nil, errors.Join(ErrPathEscapesRoot(), err)
		}
		return file, nil
	}
	return nil, os.ErrNotExist
}
