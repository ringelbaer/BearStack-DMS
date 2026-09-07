package server

import (
	"bytes"
	"strings"
	"testing"
)

func TestNavigationPlacesLinksAndActionsAccordingToPermissions(t *testing.T) {
	templates, err := parseTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		auth       AuthPermissions
		photos     bool
		settings   string
		api, audit bool
		account    bool
	}{
		{name: "anonymous"},
		{name: "principal without capabilities", auth: authPermissionsFromCapabilities(0, authPrincipal{Username: "restricted"})},
		{name: "metadata editor without read permission", auth: authPermissionsFromCapabilities(authCapDocumentsEdit, authPrincipal{Username: "editor"}), account: true},
		{name: "webdav only", auth: authPermissionsFromCapabilities(authCapDocumentsWebDAVRead, authPrincipal{Username: "webdav"}), api: true, account: true},
		{name: "uploader", auth: authPermissionsFromCapabilities(authCapDocumentsUpload, authPrincipal{Username: "uploader"}), api: true, account: true},
		{name: "auditor", auth: authPermissionsFromCapabilities(authCapSystemAudit, authPrincipal{Username: "auditor"}), audit: true, account: true},
		{name: "photo reader", auth: authPermissionsFromCapabilities(authCapPhotosRead, authPrincipal{Username: "reader"}), photos: true, api: true, account: true},
		{name: "photo manager", auth: authPermissionsFromCapabilities(authCapPhotosManage, authPrincipal{Username: "manager"}), photos: true, settings: "/settings/photos", account: true},
		{name: "photo module disabled", auth: authPermissionsFromCapabilities(authCapPhotosManage, authPrincipal{Username: "manager"}), account: true},
		{name: "user manager", auth: authPermissionsFromCapabilities(authCapSystemUsersManage, authPrincipal{Username: "manager"}), settings: "/settings/users", account: true},
		{name: "photo manager precedes user settings", auth: authPermissionsFromCapabilities(authCapPhotosManage|authCapSystemUsersManage, authPrincipal{Username: "manager"}), photos: true, settings: "/settings/photos", account: true},
		{name: "disabled photos fall back to user settings", auth: authPermissionsFromCapabilities(authCapPhotosManage|authCapSystemUsersManage, authPrincipal{Username: "manager"}), settings: "/settings/users", account: true},
		{name: "system manager", auth: authPermissionsFromCapabilities(authCapSystemManage|authCapPhotosManage|authCapSystemUsersManage, authPrincipal{Username: "manager"}), photos: true, settings: "/settings/general", account: true},
		{name: "admin", auth: authPermissionsFromCapabilities(authCapSystemManage|authCapDocumentsRead|authCapSystemAudit, authPrincipal{Username: "admin"}), settings: "/settings/general", api: true, audit: true, account: true},
		{name: "local without account", auth: authPermissionsFromCapabilities(authCapSystemManage|authCapDocumentsRead|authCapSystemAudit, authPrincipal{}), settings: "/settings/general", api: true, audit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := templates.ExecuteTemplate(&out, "help.html", PageData{
				Title: "Hilfe", Active: "help", Auth: tc.auth, PhotoModuleEnabled: tc.photos,
			}); err != nil {
				t.Fatal(err)
			}
			body := out.String()
			_, menu, _ := strings.Cut(body, `<nav class="system-menu-list" aria-label="Systemmenü">`)
			menu, _, _ = strings.Cut(menu, "</nav>")
			_, footer, _ := strings.Cut(body, `<footer class="app-footer"`)
			footer, _, _ = strings.Cut(footer, "</footer>")
			for _, link := range []struct {
				path    string
				visible bool
			}{{"/api", tc.api}, {"/log", tc.audit}} {
				href := `href="` + link.path + `"`
				if strings.Contains(menu, href) {
					t.Errorf("%s remains in system menu", link.path)
				}
				if got := strings.Contains(footer, href); got != link.visible {
					t.Errorf("footer %s visible = %v, want %v", link.path, got, link.visible)
				}
			}
			_, actions, _ := strings.Cut(menu, `<div class="system-menu-actions"`)
			actions, _, _ = strings.Cut(actions, "</div>")
			if tc.settings != "" {
				if !strings.Contains(actions, `href="`+tc.settings+`"`) || !strings.Contains(actions, `aria-label="Einstellungen"`) {
					t.Errorf("settings icon missing or has wrong target: %s", actions)
				}
			} else if strings.Contains(actions, `aria-label="Einstellungen"`) {
				t.Error("settings icon visible without an allowed settings target")
			}
			if got := strings.Contains(actions, `href="/account"`); got != tc.account {
				t.Errorf("account icon visible = %v, want %v", got, tc.account)
			}
			if got := strings.Contains(actions, `form method="post" action="/logout"`); got != tc.account {
				t.Errorf("logout POST form visible = %v, want %v", got, tc.account)
			}
			if tc.account && (!strings.Contains(actions, `aria-label="Konto · `+tc.auth.Username+`"`) || !strings.Contains(actions, `aria-label="Logout"`)) {
				t.Error("account or logout icon has no accessible name")
			}
		})
	}
}
