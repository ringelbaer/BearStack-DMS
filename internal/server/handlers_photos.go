// Datei rendert Fotoansichten und verarbeitet Frame- und Admin-Visibility-Aktionen.
package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"bearstack/internal/boolutil"
	"bearstack/internal/photos"
)

type photoMediaCacheMode int

const (
	photoMediaCacheDefault photoMediaCacheMode = iota
	photoMediaCacheNoStore
)

type photoRandomDeliverySize string

const (
	photoRandomDeliveryOriginal photoRandomDeliverySize = "original"
	photoRandomDeliveryFolder   photoRandomDeliverySize = "folder"
	photoRandomDeliveryGallery  photoRandomDeliverySize = "gallery"
	photoRandomDeliveryLarge    photoRandomDeliverySize = "large"
	photoRandomDeliveryHD       photoRandomDeliverySize = "hd"
)

func (s *Server) handlePhotos(w http.ResponseWriter, r *http.Request) {
	r, trace := s.withPhotoListTrace(r)
	if trace != nil {
		defer s.logPhotoListTrace(r, trace)
	}
	canEditPhotos := s.requestHasCapabilities(r, authCapPhotosEdit)
	listing, settings, err := s.photoService().Listing(r.Context(), photoListingRequestFromRequest(r, s.requestPhotoAdminOnlyVisible(r), false, canEditPhotos))
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	filter := photoFilterFromRequest(r, listing)
	finishDecorate := photos.StartListTraceStep(r.Context(), "photos.handler.decorate_filter")
	s.decoratePhotoAdminOnlyFilter(r, &filter)
	finishDecorate(photos.ListTraceBool("admin_toggle", filter.AdminOnlyToggleVisible), photos.ListTraceBool("show_admin_only", filter.ShowAdminOnly))
	finishView := photos.StartListTraceStep(r.Context(), "photos.handler.presenter")
	photoView := newPhotoListingView(r.Context(), s.photos, listing, settings)
	annotatePhotoTraceFolderURLs(&photoView, photoTraceQueryValue(r.URL.Query()))
	finishView(
		photos.ListTraceInt("folders", len(photoView.Folders)),
		photos.ListTraceInt("media", len(photoView.Media)),
		photos.ListTraceInt("folder_previews", photoFolderPreviewViewCount(photoView.Folders)),
	)
	var mediaGroups []PhotoMediaGroup
	if !filter.MapView {
		finishGroups := photos.StartListTraceStep(r.Context(), "photos.handler.media_groups", photos.ListTraceInt("media", len(photoView.Media)))
		mediaGroups = photoMediaGroups(photoView)
		finishGroups(photos.ListTraceInt("groups", len(mediaGroups)))
	}
	var traceSnapshot photos.ListTraceSnapshot
	if trace != nil {
		traceSnapshot = trace.Snapshot()
	}
	s.render(w, r, "photos.html", PageData{
		Title:            "Fotos",
		Active:           "photos",
		Assets:           photoPageAssets(canEditPhotos),
		Photos:           photoView,
		PhotoFilter:      filter,
		PhotoPage:        true,
		PhotoSettings:    settings,
		PhotoMediaGroups: mediaGroups,
		PhotoListTrace:   traceSnapshot,
		ReturnURL:        currentReturnURL(r),
	})
}

func (s *Server) handlePhotoFrame(w http.ResponseWriter, r *http.Request) {
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	opts := photoListOptionsFromRequest(r)
	cleanPath, err := photos.CleanPath(opts.Path)
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	if s.blockAdminOnlyPhotoFolder(w, r, cleanPath) {
		return
	}
	listing := photos.Listing{
		Path:      cleanPath,
		Query:     strings.TrimSpace(opts.Query),
		MediaType: opts.MediaType,
		GPSOnly:   opts.GPSOnly,
		Sort:      opts.Sort,
		Page:      1,
	}
	photoView := newPhotoListingView(r.Context(), s.photos, listing, settings)
	s.render(w, r, "photo_frame.html", PageData{
		Title:         "Fotoframe",
		Active:        "photos",
		Assets:        photoFrameAssets(),
		Photos:        photoView,
		PhotoFilter:   s.photoFilterForListing(r, listing),
		PhotoFrame:    true,
		PhotoSettings: settings,
	})
}

func (s *Server) handlePhotoFrameItems(w http.ResponseWriter, r *http.Request) {
	if s.photos == nil {
		s.renderJSONError(w, http.StatusInternalServerError, errPhotoModuleMissing.Error())
		return
	}
	settings, err := s.photoSettings(r.Context())
	if err != nil {
		s.renderJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	opts := photoListOptionsFromRequest(r)
	cleanPath, err := photos.CleanPath(opts.Path)
	if err != nil {
		s.renderJSONError(w, http.StatusBadRequest, "ungültiger Fotopfad")
		return
	}
	if s.blockAdminOnlyPhotoFolder(w, r, cleanPath) {
		return
	}
	opts.Path = cleanPath
	opts.IncludeAdminOnly = s.requestPhotoAdminOnlyVisible(r)
	opts.Recursive = true
	opts.Page = parsePositiveInt(r.URL.Query().Get("page"), 1)
	opts.PageSize = parsePositiveInt(r.URL.Query().Get("page_size"), 200)
	opts.LeanMetadata = true
	opts.IncludeMapData = false
	if opts.PageSize > 500 {
		opts.PageSize = 500
	}
	listing, err := s.photos.List(r.Context(), opts)
	if err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	if err = s.photos.AddAutomaticFaces(r.Context(), listing.Media); err != nil {
		s.renderPhotoError(w, r, err)
		return
	}
	photoView := newPhotoListingView(r.Context(), s.photos, listing, settings)
	if err := writeJSON(w, http.StatusOK, struct {
		Media    []photoMediaAPIResponse `json:"media"`
		Page     int                     `json:"page"`
		Total    int                     `json:"total"`
		HasNext  bool                    `json:"has_next"`
		PageSize int                     `json:"page_size"`
	}{
		Media:    photoFrameMediaAPIResponsesFrom(photoView.Media),
		Page:     listing.Page,
		Total:    listing.Total,
		HasNext:  listing.HasNext,
		PageSize: listing.PageSize,
	}); err != nil {
		s.log.Warn("photo frame items response failed", "path", cleanPath, "error", err)
	}
}

func (s *Server) requestIsPhotoAdmin(r *http.Request) bool {
	if s == nil || !s.authEnabled() {
		return true
	}
	principal, ok := authPrincipalFromContext(r.Context())
	return ok && principal.Role == "admin"
}

func (s *Server) requestPhotoAdminOnlyVisible(r *http.Request) bool {
	if !s.requestIsPhotoAdmin(r) {
		return false
	}
	payload, ok := s.authSessionFromRequest(r)
	return ok && payload.PhotoAdminOnlyShown
}

func (s *Server) requestCanTogglePhotoAdminOnly(r *http.Request) bool {
	return s != nil && len(s.authKey) > 0 && s.requestIsPhotoAdmin(r)
}

func (s *Server) photoFilterForListing(r *http.Request, listing photos.Listing) PhotoFilter {
	filter := photoFilterFromRequest(r, listing)
	s.decoratePhotoAdminOnlyFilter(r, &filter)
	return filter
}

func (s *Server) decoratePhotoAdminOnlyFilter(r *http.Request, filter *PhotoFilter) {
	if filter == nil {
		return
	}
	filter.AdminOnlyToggleVisible = s.requestCanTogglePhotoAdminOnly(r)
	filter.ShowAdminOnly = s.requestPhotoAdminOnlyVisible(r)
}

func (s *Server) handlePhotoAdminOnlyVisibility(w http.ResponseWriter, r *http.Request) {
	if !s.requestIsPhotoAdmin(r) {
		s.renderForbidden(w, r)
		return
	}
	if !s.parseFormOrRenderError(w, r) {
		return
	}
	show := truthy(r.FormValue("show"))
	if !s.setPhotoAdminOnlyVisibilitySession(w, r, show) {
		s.renderError(w, r, http.StatusBadRequest, errors.New("Foto-Session konnte nicht gespeichert werden"))
		return
	}
	redirect(w, r, formReturnURL(r))
}

func (s *Server) setPhotoAdminOnlyVisibilitySession(w http.ResponseWriter, r *http.Request, show bool) bool {
	if s == nil || len(s.authKey) == 0 {
		return false
	}
	now := time.Now()
	payload, ok := s.authSessionFromRequest(r)
	principal, principalOK := authPrincipalFromContext(r.Context())
	if !principalOK {
		return false
	}
	credential := s.authCredentialForPrincipal(principal)
	if credential == nil {
		return false
	}
	if !ok {
		payload = authSessionPayloadForCredential(credential, now.Add(authSessionDuration))
	} else if payload.Source != principal.Source || payload.Subject != principal.Subject || payload.Revision != principal.Revision || payload.User != principal.Username {
		return false
	}
	if payload.Expires <= now.Unix() {
		payload.Expires = now.Add(authSessionDuration).Unix()
	}
	payload.PhotoAdminOnlyShown = show
	s.writeAuthSessionCookie(w, r, payload)
	return true
}

func photoListOptionsFromRequest(r *http.Request) photos.ListOptions {
	q := r.URL.Query()
	sortValue := q.Get("sort")
	if _, ok := q["sort"]; !ok {
		sortValue = "ascending_date"
	}
	path := q.Get("path")
	query := q.Get("q")
	if strings.TrimSpace(query) != "" {
		path = ""
	}
	return photos.ListOptions{
		Path:      path,
		Query:     query,
		MediaType: q.Get("type"),
		GPSOnly:   truthy(q.Get("gps")),
		Sort:      sortValue,
		Page:      parsePositiveInt(q.Get("page"), 1),
	}
}

func photoListingRequestFromRequest(r *http.Request, includeAdminOnly, frame, canEdit bool) photoListingRequest {
	return photoListingRequest{
		Options:          photoListOptionsFromRequest(r),
		IncludeAdminOnly: includeAdminOnly,
		Frame:            frame,
		CanEdit:          canEdit,
		MapRequested:     r.URL.Query().Get("view") == "map",
	}
}

func truthy(value string) bool {
	return boolutil.Truthy(value)
}
