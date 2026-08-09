package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/config"
)

func TestManagedUserFormDoesNotRetainPasswords(t *testing.T) {
	form := url.Values{
		"username":                  {"  test-user  "},
		"role":                      {account.RoleDocumentsRead},
		"permissions":               {account.PermissionDocumentsUpload},
		"new_password":              {"never-render-this-password"},
		"new_password_confirmation": {"never-render-this-password"},
		"current_password":          {"never-render-this-admin-password"},
	}
	req := httptest.NewRequest(http.MethodPost, "/settings/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}
	view := managedUserFormFromRequest(req, true)
	got := view.Username + "\n" + view.Role + "\n" + strings.Join(selectedPermissionNames(view.SelectedPermissions), "\n")
	if strings.Contains(got, "never-render") {
		t.Fatalf("view retained password material: %#v", view)
	}
	if view.Username != "test-user" {
		t.Fatalf("username = %q", view.Username)
	}
}

func TestDelegatedUserManagerCanOnlyGrantOwnedOrdinaryRights(t *testing.T) {
	manager := authPrincipal{
		Username: "manager",
		Role:     account.RoleCustom,
		capabilities: authCapabilities(account.CapabilitySystemUsersManage |
			account.CapabilityPhotosRead),
	}
	if err := validateDelegatedAccess(manager, account.RolePhotosRead, nil); err != nil {
		t.Fatalf("grant owned right: %v", err)
	}
	if err := validateDelegatedAccess(manager, account.RolePhotosEditor, nil); err == nil {
		t.Fatal("granting unowned photo edit right succeeded")
	}
	if err := validateDelegatedAccess(manager, account.RoleAdmin, nil); err == nil {
		t.Fatal("granting admin role succeeded")
	}
	if err := validateDelegatedAccess(manager, account.RoleCustom, []string{account.PermissionSystemUsersManage}); err == nil {
		t.Fatal("granting user-manager permission succeeded")
	}
}

func TestConfiguredUsersAreReadOnlyInManagementView(t *testing.T) {
	server := &Server{cfg: config.Config{Auth: config.AuthConfig{Credentials: []config.AuthCredential{{
		Username: "configured-admin",
		Password: "not-exposed",
		Role:     account.RoleAdmin,
	}}}}}
	view := server.userManagementListView(nil, authPrincipal{Username: "configured-admin", Role: account.RoleAdmin})
	if len(view.Users) != 1 {
		t.Fatalf("users = %#v", view.Users)
	}
	user := view.Users[0]
	if user.Source != "config" || user.Editable || !user.Current {
		t.Fatalf("configured user = %#v", user)
	}
	if strings.Contains(strings.Join(permissionViewLabels(user.EffectivePermissions), " "), "not-exposed") {
		t.Fatal("password reached the safe user view")
	}
}

func TestUserBootstrapRequiresLoopbackAndEmptyAccountSources(t *testing.T) {
	server := &Server{}
	loopback := httptest.NewRequest(http.MethodGet, "http://localhost/settings/users/new", nil)
	loopback.RemoteAddr = "127.0.0.1:43120"
	if !server.userBootstrapAllowed(loopback, nil) {
		t.Fatal("loopback bootstrap should be allowed")
	}
	remote := httptest.NewRequest(http.MethodGet, "http://example.test/settings/users/new", nil)
	remote.RemoteAddr = "192.0.2.10:43120"
	if server.userBootstrapAllowed(remote, nil) {
		t.Fatal("remote bootstrap should be denied")
	}
	rebinding := httptest.NewRequest(http.MethodGet, "http://attacker.example/settings/users/new", nil)
	rebinding.RemoteAddr = "127.0.0.1:43120"
	if server.userBootstrapAllowed(rebinding, nil) {
		t.Fatal("loopback bootstrap with a non-loopback Host should be denied")
	}
	if server.userBootstrapAllowed(loopback, []account.User{{ID: 1}}) {
		t.Fatal("bootstrap with database user should be denied")
	}
	server.cfg.Auth.Username = "configured"
	server.cfg.Auth.Password = "secret"
	if server.userBootstrapAllowed(loopback, nil) {
		t.Fatal("bootstrap with configured user should be denied")
	}
}

func TestDelegatedManagerCanSelectCustomRole(t *testing.T) {
	actor := authPrincipal{
		Username:     "manager",
		Role:         account.RoleCustom,
		capabilities: authCapSystemUsersManage | authCapDocumentsRead,
	}
	view := userManagementFormView(actor, ManagedUserFormView{
		Role:                account.RoleCustom,
		SelectedPermissions: map[string]bool{account.PermissionDocumentsRead: true},
	}, true, false)
	for _, role := range view.Roles {
		if role.Value == account.RoleCustom {
			if role.Disabled {
				t.Fatal("delegated manager must be able to select a custom role with an allowed permission subset")
			}
			return
		}
	}
	t.Fatal("custom role option is missing")
}

func permissionViewLabels(views []UserPermissionLabelView) []string {
	labels := make([]string, len(views))
	for i := range views {
		labels[i] = views[i].Label
	}
	return labels
}
