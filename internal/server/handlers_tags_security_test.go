package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/config"
)

func TestTagsWithoutPhotoModuleRequiresDocumentRead(t *testing.T) {
	repo := openAuthSecurityRepository(t)
	if _, err := repo.SaveTag(context.Background(), "private-document-tag", "confidential description", "#176b87", false, false); err != nil {
		t.Fatal(err)
	}
	server := newAuthSecurityServer(t, config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{
		{Username: "photos", Password: "secret", Role: account.RolePhotosRead},
		{Username: "manager", Password: "secret", Role: account.RolePhotosManager},
		{Username: "documents", Password: "secret", Role: account.RoleDocumentsRead},
	}}}, repo)
	for _, username := range []string{"photos", "manager", "documents"} {
		for _, path := range []string{"/tags", "/tags?tab=photos"} {
			t.Run(username+path, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				req.SetBasicAuth(username, "secret")
				rec := httptest.NewRecorder()
				server.Handler().ServeHTTP(rec, req)
				want := http.StatusForbidden
				if username == "documents" {
					want = http.StatusOK
				}
				if rec.Code != want {
					t.Errorf("status = %d, want %d", rec.Code, want)
				}
				for _, value := range []string{"private-document-tag", "confidential description"} {
					if strings.Contains(rec.Body.String(), value) != (username == "documents") {
						t.Errorf("document metadata %q visibility does not match read permission", value)
					}
				}
			})
		}
	}
}
