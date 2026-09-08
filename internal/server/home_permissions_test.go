package server

import "testing"

func TestResolveHomePageUsesExplicitModuleStateAndPermissions(t *testing.T) {
	tests := []struct {
		name, page    string
		auth          AuthPermissions
		photos, cloud bool
		want          string
	}{
		{name: "configured folders", page: homePageFolders, auth: AuthPermissions{CanDocumentsRead: true}, want: homePageFolders},
		{name: "cloud enabled", page: homePageCloud, auth: AuthPermissions{CanDocumentsRead: true}, cloud: true, want: homePageCloud},
		{name: "cloud disabled", page: homePageCloud, auth: AuthPermissions{CanDocumentsRead: true}, want: homePageDocuments},
		{name: "photos enabled", page: homePagePhotos, auth: AuthPermissions{CanPhotosRead: true}, photos: true, want: homePagePhotos},
		{name: "photos disabled", page: homePagePhotos, auth: AuthPermissions{CanDocumentsRead: true, CanPhotosRead: true}, want: homePageDocuments},
		{name: "photos without permission", page: homePagePhotos, auth: AuthPermissions{CanDocumentsRead: true}, photos: true, want: homePageDocuments},
		{name: "photo reader leaves document home", page: homePageFolders, auth: AuthPermissions{CanPhotosRead: true}, photos: true, want: homePagePhotos},
		{name: "no content rights", page: homePagePhotos, auth: AuthPermissions{CanSystemManage: true}, photos: true, want: homePageDocuments},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveHomePage(test.page, test.auth, test.photos, test.cloud); got != test.want {
				t.Fatalf("home page = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHomeURLForPermissionsUsesAnAccessibleLandingPage(t *testing.T) {
	tests := []struct {
		name          string
		page          string
		auth          AuthPermissions
		photosEnabled bool
		want          string
	}{
		{name: "configured document home", page: homePageFolders, auth: AuthPermissions{CanDocumentsRead: true}, want: "/folders"},
		{name: "photos", page: homePageDocuments, auth: AuthPermissions{CanPhotosRead: true}, photosEnabled: true, want: "/photos"},
		{name: "system settings", page: homePageDocuments, auth: AuthPermissions{CanSystemManage: true}, want: "/settings/general"},
		{name: "photo settings", page: homePageDocuments, auth: AuthPermissions{CanPhotosManage: true}, photosEnabled: true, want: "/settings/photos"},
		{name: "users only", page: homePageDocuments, auth: AuthPermissions{CanSystemUsersManage: true}, want: "/settings/users"},
		{name: "photo manager falls through to users when module disabled", page: homePageDocuments, auth: AuthPermissions{CanPhotosManage: true, CanSystemUsersManage: true}, want: "/settings/users"},
		{name: "audit only", page: homePageDocuments, auth: AuthPermissions{CanSystemAudit: true}, want: "/log"},
		{name: "no browser area", page: homePageDocuments, auth: AuthPermissions{CanDocumentsUpload: true}, want: "/help"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := homeURLForPermissions(test.page, test.auth, test.photosEnabled); got != test.want {
				t.Fatalf("home URL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSystemManagerAuthLandingOpensGeneralSettings(t *testing.T) {
	principal := authPrincipal{capabilities: authCapSystemManage}
	if got := defaultAuthLandingURL(principal); got != "/settings/general" {
		t.Fatalf("auth landing URL = %q, want /settings/general", got)
	}
}
