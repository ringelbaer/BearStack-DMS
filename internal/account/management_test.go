package account

import "testing"

func TestUserDelegationHelpersEnforceSubsetAndAdministratorBoundaries(t *testing.T) {
	administrator := ManagementActor{
		Username:     "admin",
		Role:         RoleAdmin,
		Capabilities: Capabilities(AllCapabilities),
	}
	delegate := ManagementActor{
		Username: "manager",
		Role:     RoleCustom,
		Capabilities: Capabilities(
			CapabilitySystemUsersManage |
				CapabilityDocumentsRead |
				CapabilityDocumentsWebDAVRead |
				CapabilityDocumentsUpload,
		),
	}

	tests := []struct {
		name        string
		actor       ManagementActor
		role        string
		permissions []string
		wantErr     bool
	}{
		{name: "administrator assigns administrator", actor: administrator, role: RoleAdmin},
		{name: "administrator assigns user manager", actor: administrator, role: RoleCustom, permissions: []string{PermissionSystemUsersManage}},
		{name: "delegate assigns subset role", actor: delegate, role: RoleDocumentsRead},
		{name: "delegate assigns subset individual permission", actor: delegate, role: RoleCustom, permissions: []string{PermissionDocumentsUpload}},
		{name: "delegate cannot assign administrator", actor: delegate, role: RoleAdmin, wantErr: true},
		{name: "delegate cannot assign user manager", actor: delegate, role: RoleCustom, permissions: []string{PermissionSystemUsersManage}, wantErr: true},
		{name: "delegate cannot assign missing capability", actor: delegate, role: RolePhotosRead, wantErr: true},
		{name: "delegate cannot assign unknown role", actor: delegate, role: "unknown", wantErr: true},
		{name: "delegate cannot assign unknown permission", actor: delegate, role: RoleCustom, permissions: []string{"unknown"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.actor.ValidateDelegatedAccess(tt.role, tt.permissions)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateDelegatedAccess() error = %v, want error %v", err, tt.wantErr)
			}
		})
	}

	if !administrator.CanCreateUser() {
		t.Fatal("administrator cannot create users")
	}
	if !delegate.CanCreateUser() {
		t.Fatal("delegate with assignable domain rights cannot create users")
	}
	usersOnlyDelegate := ManagementActor{
		Username:     "users-only",
		Role:         RoleCustom,
		Capabilities: Capabilities(CapabilitySystemUsersManage),
	}
	if usersOnlyDelegate.CanCreateUser() {
		t.Fatal("delegate without an assignable domain right can create users")
	}
}

func TestActorCanManageUserProtectsSelfAndPrivilegedTargets(t *testing.T) {
	administrator := ManagementActor{
		Username:     "admin",
		Role:         RoleAdmin,
		Capabilities: Capabilities(AllCapabilities),
	}
	delegate := ManagementActor{
		Username: "manager",
		Role:     RoleCustom,
		Capabilities: Capabilities(
			CapabilitySystemUsersManage |
				CapabilityDocumentsRead |
				CapabilityDocumentsWebDAVRead |
				CapabilityDocumentsUpload,
		),
	}

	tests := []struct {
		name   string
		actor  ManagementActor
		target User
		want   bool
	}{
		{name: "administrator manages ordinary user", actor: administrator, target: User{Username: "reader", Role: RoleDocumentsRead}, want: true},
		{name: "administrator manages another administrator", actor: administrator, target: User{Username: "other-admin", Role: RoleAdmin}, want: true},
		{name: "administrator cannot manage self", actor: administrator, target: User{Username: "admin", Role: RoleAdmin}},
		{name: "delegate manages subset user", actor: delegate, target: User{Username: "reader", Role: RoleDocumentsRead}, want: true},
		{name: "delegate manages subset custom user", actor: delegate, target: User{Username: "uploader", Role: RoleCustom, Permissions: []string{PermissionDocumentsUpload}}, want: true},
		{name: "delegate cannot manage self", actor: delegate, target: User{Username: "manager", Role: RoleDocumentsRead}},
		{name: "delegate cannot manage administrator", actor: delegate, target: User{Username: "admin", Role: RoleAdmin}},
		{name: "delegate cannot manage another user manager", actor: delegate, target: User{Username: "other-manager", Role: RoleCustom, Permissions: []string{PermissionSystemUsersManage}}},
		{name: "delegate cannot manage wider access", actor: delegate, target: User{Username: "photos", Role: RolePhotosRead}},
		{name: "anonymous actor cannot manage", target: User{Username: "reader", Role: RoleDocumentsRead}},
		{name: "delegate cannot manage invalid target", actor: delegate, target: User{Username: "broken", Role: "unknown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.actor.CanManageUser(tt.target); got != tt.want {
				t.Fatalf("actorCanManageUser() = %v, want %v", got, tt.want)
			}
		})
	}
}
