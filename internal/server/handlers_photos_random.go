// Datei verarbeitet zufaellige Fotoauslieferung und zugehoerige Response-Header.
package server

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"

	"bearstack/internal/photos"
)

func (s *Server) handlePhotoRandom(w http.ResponseWriter, r *http.Request) {
	sizeMode, ok := parsePhotoRandomDeliverySize(r.URL.Query().Get("size"))
	if !ok {
		s.renderError(w, r, http.StatusBadRequest, errors.New("ungültige Fotogröße"))
		return
	}
	path, err := s.photoService().RandomMediaPath(r.Context(), photoListOptionsFromRequest(r), s.requestPhotoAdminOnlyVisible(r))
	if errors.Is(err, errNoPhotosFound) {
		s.renderError(w, r, http.StatusNotFound, errors.New("keine Fotos gefunden"))
		return
	}
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	media, err := s.photos.MediaContext(r.Context(), path)
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	setPhotoRandomInfoHeaders(w, r, media)
	s.servePhotoRandomMedia(w, r, path, sizeMode)
}

func setPhotoRandomInfoHeaders(w http.ResponseWriter, r *http.Request, media photos.Media) {
	title := strings.TrimSpace(cleanPhotoFrameTitle(media.Name))
	if title == "" {
		title = strings.TrimSpace(media.Name)
	}
	if title != "" {
		w.Header().Set("X-BearStack-Photo-Title", title)
	}
	mediaPath := strings.TrimSpace(media.Path)
	if mediaPath != "" {
		w.Header().Set("X-BearStack-Photo-Path", mediaPath)
	}
	folderPath := normalizePhotoRandomInfoPath(media.Directory)
	folderURL := photoRandomFolderURL(r, folderPath)
	w.Header().Set("X-BearStack-Photo-Folder-Path", folderPath)
	w.Header().Set("X-BearStack-Photo-Folder-URL", folderURL)
	w.Header().Set("X-BearStack-Photo-Folder-Title", photoRandomFolderTitle(folderPath))
	w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"up\"", folderURL))
}

func normalizePhotoRandomInfoPath(value string) string {
	return strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/")
}

func photoRandomFolderURL(r *http.Request, folderPath string) string {
	values := url.Values{}
	if folderPath != "" {
		values.Set("path", folderPath)
	}
	return photoRandomAbsoluteURL(r, photoPageURL(values))
}

func photoRandomAbsoluteURL(r *http.Request, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		return path
	}
	base := &url.URL{
		Scheme: originRequestScheme(r),
		Host:   host,
	}
	relative, err := url.Parse(path)
	if err != nil {
		return path
	}
	return base.ResolveReference(relative).String()
}

func photoRandomFolderTitle(folderPath string) string {
	if folderPath == "" {
		return "Fotos"
	}
	parts := strings.Split(folderPath, "/")
	return parts[len(parts)-1]
}

func (s *Server) servePhotoRandomMedia(w http.ResponseWriter, r *http.Request, path string, sizeMode photoRandomDeliverySize) {
	if sizeMode == photoRandomDeliveryOriginal {
		s.servePhotoMedia(w, r, path, photoMediaCacheNoStore)
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	size := photoRandomThumbnailSize(sizeMode, settings)
	s.servePhotoThumbnail(w, r, path, size, photoMediaCacheNoStore)
}

func parsePhotoRandomDeliverySize(value string) (photoRandomDeliverySize, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "original":
		return photoRandomDeliveryOriginal, true
	case "ordner", "folder":
		return photoRandomDeliveryFolder, true
	case "galerie", "gallery":
		return photoRandomDeliveryGallery, true
	case "groß", "gross", "large":
		return photoRandomDeliveryLarge, true
	case "hd":
		return photoRandomDeliveryHD, true
	default:
		return "", false
	}
}

func photoRandomThumbnailSize(sizeMode photoRandomDeliverySize, settings PhotoSettings) int {
	settings = normalizePhotoPresentationSettings(settings)
	switch sizeMode {
	case photoRandomDeliveryFolder:
		return settings.FolderThumbnailSize
	case photoRandomDeliveryGallery:
		return settings.ThumbnailSize
	case photoRandomDeliveryLarge:
		return settings.PreviewSize
	case photoRandomDeliveryHD:
		return settings.LargePreviewSize
	default:
		return settings.ThumbnailSize
	}
}

func randomIndex(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("empty random range")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}
