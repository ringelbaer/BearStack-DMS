// Datei bestimmt erreichbare Startseiten und Navigationsziele aus Einstellungen und Rechten.
package server

import "context"

func normalizeAvailableHomePage(value string, photosEnabled, cloudEnabled bool) string {
	page := normalizeHomePage(value)
	if page == homePageCloud && !cloudEnabled {
		return homePageDocuments
	}
	if page == homePagePhotos && !photosEnabled {
		return homePageDocuments
	}
	return page
}

func homePageURL(page string) string {
	switch normalizeHomePage(page) {
	case homePageFolders:
		return "/folders"
	case homePageCloud:
		return "/cloud"
	case homePagePhotos:
		return "/photos"
	default:
		return "/documents"
	}
}

func homeURLForPermissions(page string, auth AuthPermissions, photosEnabled bool) string {
	if auth.CanDocumentsRead {
		return homePageURL(page)
	}
	if photosEnabled && auth.CanPhotosRead {
		return "/photos"
	}
	if auth.CanSystemManage {
		return "/settings/general"
	}
	if photosEnabled && auth.CanPhotosManage {
		return "/settings/photos"
	}
	if auth.CanSystemUsersManage {
		return "/settings/users"
	}
	if auth.CanSystemAudit {
		return "/log"
	}
	return "/help"
}

func (s *Server) resolvedHomePage(ctx context.Context, auth AuthPermissions) (string, error) {
	page := homePageDocuments
	cloudEnabled := false
	if s != nil {
		configured, err := s.homePage(ctx)
		if err != nil {
			return "", err
		}
		page = configured
		enabled, err := s.documentCloudEnabled(ctx)
		if err != nil {
			return "", err
		}
		cloudEnabled = enabled
	}
	return resolveHomePage(page, auth, s != nil && s.photos != nil, cloudEnabled), nil
}

func resolveHomePage(page string, auth AuthPermissions, photosEnabled, cloudEnabled bool) string {
	page = normalizeAvailableHomePage(page, photosEnabled, cloudEnabled)
	if homePageAllowed(page, auth, photosEnabled, cloudEnabled) {
		return page
	}
	if auth.CanDocumentsRead {
		return homePageDocuments
	}
	if photosEnabled && auth.CanPhotosRead {
		return homePagePhotos
	}
	return homePageDocuments
}

func homePageAllowed(page string, auth AuthPermissions, photosEnabled, cloudEnabled bool) bool {
	switch normalizeHomePage(page) {
	case homePagePhotos:
		return photosEnabled && auth.CanPhotosRead
	case homePageCloud:
		return cloudEnabled && auth.CanDocumentsRead
	case homePageDocuments, homePageFolders:
		return auth.CanDocumentsRead
	default:
		return auth.CanDocumentsRead
	}
}
