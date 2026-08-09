package server

import "testing"

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
		{name: "system settings", page: homePageDocuments, auth: AuthPermissions{CanSystemManage: true}, want: "/settings"},
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
