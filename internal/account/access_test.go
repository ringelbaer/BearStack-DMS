package account

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizeAccessCombinesRoleAndNormalizedAdditionalPermissions(t *testing.T) {
	role, additional, capabilities, err := NormalizeAccess("  "+RoleDocumentsRead+"  ", []string{
		" " + PermissionPhotosRead + " ",
		PermissionDocumentsUpload,
		PermissionPhotosRead,
		"",
	})
	if err != nil {
		t.Fatal(err)
	}
	if role != RoleDocumentsRead {
		t.Fatalf("role = %q, want %q", role, RoleDocumentsRead)
	}
	wantAdditional := []string{PermissionDocumentsUpload, PermissionPhotosRead}
	if !reflect.DeepEqual(additional, wantAdditional) {
		t.Fatalf("additional permissions = %#v, want %#v", additional, wantAdditional)
	}
	want := CapabilityDocumentsRead | CapabilityDocumentsWebDAVRead | CapabilityDocumentsUpload | CapabilityPhotosRead
	if capabilities != want {
		t.Fatalf("capabilities = %b, want %b", capabilities, want)
	}
}

func TestNormalizeAccessRejectsUnknownOrEmptyAccess(t *testing.T) {
	tests := []struct {
		name        string
		role        string
		permissions []string
		wantErr     error
	}{
		{name: "unknown role", role: "owner", permissions: []string{PermissionDocumentsRead}, wantErr: ErrUnknownRole},
		{name: "unknown permission", role: RoleCustom, permissions: []string{"documents.fly"}, wantErr: ErrUnknownPermission},
		{name: "empty custom access", role: RoleCustom, wantErr: ErrAccessRequired},
		{name: "implicit custom access", role: "", permissions: []string{""}, wantErr: ErrAccessRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := NormalizeAccess(tt.role, tt.permissions)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NormalizeAccess() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigCapabilitiesForPreservesLegacyAdministratorDefault(t *testing.T) {
	capabilities, role, err := ConfigCapabilitiesFor("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if role != RoleAdmin {
		t.Fatalf("default config role = %q, want %q", role, RoleAdmin)
	}
	if capabilities != AllCapabilities {
		t.Fatalf("default config capabilities = %b, want all %b", capabilities, AllCapabilities)
	}
	if !capabilities.HasAll(CapabilitySystemUsersManage) {
		t.Fatal("default config administrator cannot manage users")
	}

	capabilities, role, err = ConfigCapabilitiesFor("", []string{PermissionPhotosRead})
	if err != nil {
		t.Fatal(err)
	}
	if role != RoleCustom || capabilities != CapabilityPhotosRead {
		t.Fatalf("explicit config permission produced role %q and capabilities %b", role, capabilities)
	}
}

func TestAdministratorAndExplicitPermissionAreUserManagers(t *testing.T) {
	if !IsAdministratorRole("  " + RoleAdmin + " ") {
		t.Fatal("admin role was not recognized")
	}
	if !IsUserManager(RoleAdmin, nil) {
		t.Fatal("administrator was not recognized as user manager")
	}
	if !IsUserManager(RoleDocumentsRead, []string{PermissionSystemUsersManage}) {
		t.Fatal("additional users.manage permission was not recognized")
	}
	if IsUserManager(RoleDocumentsRead, []string{PermissionSystemManage}) {
		t.Fatal("system.manage unexpectedly grants user management")
	}
}

func TestAccessDescriptorsReturnDefensiveCopies(t *testing.T) {
	permissions := PermissionDescriptors()
	roles := RoleDescriptors()
	if len(permissions) == 0 || len(roles) == 0 || len(roles[0].Permissions) == 0 {
		t.Fatal("access descriptors are unexpectedly empty")
	}
	originalPermission := permissions[0].Name
	originalRolePermission := roles[0].Permissions[0]
	permissions[0].Name = "modified"
	roles[0].Permissions[0] = "modified"
	if got := PermissionDescriptors()[0].Name; got != originalPermission {
		t.Fatalf("permission descriptor was mutated through returned slice: %q", got)
	}
	if got := RoleDescriptors()[0].Permissions[0]; got != originalRolePermission {
		t.Fatalf("role descriptor permissions were mutated through returned slice: %q", got)
	}
}
