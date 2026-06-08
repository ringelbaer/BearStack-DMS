// Datei prueft Fotozugriffe und Admin-only-Sichtbarkeit fuer HTTP-Anfragen.
package server

import (
	"errors"
	"net/http"

	"bearstack/internal/photos"
)

type photoAccessPolicy struct {
	library        *photos.Library
	allowAdminOnly bool
}

func (p photoAccessPolicy) RequireMedia(path string) error {
	if p.allowAdminOnly {
		return nil
	}
	if p.library == nil {
		return errPhotoModuleMissing
	}
	adminOnly, err := p.library.MediaAdminOnly(path)
	if err != nil {
		return err
	}
	if adminOnly {
		return photos.ErrAdminOnly()
	}
	return nil
}

func (p photoAccessPolicy) RequireMediaBatch(paths []string) error {
	if p.allowAdminOnly {
		return nil
	}
	if p.library == nil {
		return errPhotoModuleMissing
	}
	adminOnlyByPath, err := p.library.MediaAdminOnlyBatch(paths)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if adminOnlyByPath[path] {
			return photos.ErrAdminOnly()
		}
	}
	return nil
}

func (p photoAccessPolicy) RequireFolder(path string) error {
	if p.allowAdminOnly {
		return nil
	}
	if p.library == nil {
		return errPhotoModuleMissing
	}
	adminOnly, err := p.library.FolderAdminOnly(path)
	if err != nil {
		return err
	}
	if adminOnly {
		return photos.ErrAdminOnly()
	}
	return nil
}

func (s *Server) photoAccessPolicy(allowAdminOnly bool) photoAccessPolicy {
	return photoAccessPolicy{
		library:        s.photos,
		allowAdminOnly: allowAdminOnly,
	}
}

func (s *Server) blockPhotoAccess(w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, photos.ErrAdminOnly()) {
		s.renderForbidden(w, r)
		return true
	}
	if err != nil {
		s.renderPhotoError(w, r, err)
		return true
	}
	return false
}

func (s *Server) blockAdminOnlyPhotoMedia(w http.ResponseWriter, r *http.Request, path string) bool {
	err := s.photoAccessPolicy(s.requestIsPhotoAdmin(r)).RequireMedia(path)
	return s.blockPhotoAccess(w, r, err)
}

func (s *Server) blockAdminOnlyPhotoFolder(w http.ResponseWriter, r *http.Request, path string) bool {
	err := s.photoAccessPolicy(s.requestIsPhotoAdmin(r)).RequireFolder(path)
	return s.blockPhotoAccess(w, r, err)
}
