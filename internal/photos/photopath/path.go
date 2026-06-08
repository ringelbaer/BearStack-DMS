// Datei normalisiert und validiert Foto-Pfade unabhaengig vom Betriebssystempfadformat.
package photopath

import (
	"errors"

	"bearstack/internal/fsutil"
)

var ErrEscapesRoot = errors.New("photo path escapes root")

func Clean(value string) (string, error) {
	return fsutil.CleanRelativePath(value, true, ErrEscapesRoot)
}

func Resolve(root, rel string) (string, error) {
	_, abs, err := fsutil.ResolveWithinRoot(root, rel, true, ErrEscapesRoot)
	return abs, err
}

func RejectSymlinkPath(root, rel string) error {
	return fsutil.RejectSymlinkPath(root, rel, ErrEscapesRoot)
}
