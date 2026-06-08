// Datei definiert gemeinsame Fehlerwerte fuer Fotozugriff, Pfadauflosung und Mediensuche.
package photos

import "errors"

var (
	errPhotoSearchTooBroad  = errors.New("Fotosuche ist zu breit; bitte Suchbegriff oder Filter verfeinern")
	errThumbnailUnavailable = errors.New("thumbnail is not available for this media type")
)

func ErrSearchTooBroad() error {
	return errPhotoSearchTooBroad
}

func ErrThumbnailUnavailable() error {
	return errThumbnailUnavailable
}
