// Datei übersetzt sichere Konto-Projektionen in Viewmodels ohne Passwortmaterial.
package server

import (
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"bearstack/internal/account"
)

func (s *Server) userManagementListView(users []account.User, actor authPrincipal) UserManagementView {
	configuredAccounts := s.authConfigAccountViews()
	result := UserManagementView{
		Users:             make([]ManagedUserView, 0, len(users)+len(configuredAccounts)),
		CanCreate:         actorCanCreateUser(actor),
		CurrentUsername:   actor.Username,
		CurrentUserSource: actor.Source,
		CurrentUserID:     actor.AccountID,
	}
	for _, user := range users {
		result.Users = append(result.Users, s.managedUserView(user, actor))
	}
	for _, configured := range configuredAccounts {
		effective := permissionLabelsForCapabilities(configured.Capabilities)
		preference := s.accountPreference(configured.Source, configured.Subject)
		result.Users = append(result.Users, ManagedUserView{
			Username:    configured.Username,
			Source:      authSourceConfig,
			Subject:     configured.Subject,
			SourceLabel: "Konfiguration",
			Role:        configured.Role,
			RoleLabel:   roleLabel(configured.Role),
			Active:      configured.Enabled,
			Current: actor.Username == configured.Username &&
				(actor.Source == "" || (actor.Source == authSourceConfig && actor.Subject == configured.Subject)),
			Editable:                false,
			CanManagePreferences:    actorCanManageConfigPreference(actor, configured),
			CustomPDFPreviewEnabled: preference.CustomPDFPreviewEnabled,
			PreferenceVersion:       preference.RowVersion,
			ExtraPermissions:        permissionLabels(configured.Permissions),
			EffectivePermissions:    effective,
		})
	}
	sort.Slice(result.Users, func(i, j int) bool {
		if result.Users[i].Username == result.Users[j].Username {
			return result.Users[i].Source < result.Users[j].Source
		}
		return result.Users[i].Username < result.Users[j].Username
	})
	return result
}

func (s *Server) managedUserView(user account.User, actor authPrincipal) ManagedUserView {
	effective, _ := account.EffectivePermissionNames(user.Role, user.Permissions)
	subject := strconv.FormatInt(user.ID, 10)
	preference := s.accountPreference(authSourceDatabase, subject)
	current := actor.Username == user.Username &&
		(actor.Source == "" || (actor.Source == authSourceDatabase && actor.AccountID == user.ID))
	return ManagedUserView{
		ID:                      user.ID,
		Username:                user.Username,
		Source:                  authSourceDatabase,
		Subject:                 subject,
		SourceLabel:             "BearStack-Datenbank",
		Role:                    user.Role,
		RoleLabel:               roleLabel(user.Role),
		Active:                  user.Enabled,
		Current:                 current,
		Editable:                actorCanManageUser(actor, user),
		CanManagePreferences:    current || actorCanManageUser(actor, user),
		CustomPDFPreviewEnabled: preference.CustomPDFPreviewEnabled,
		PreferenceVersion:       preference.RowVersion,
		Version:                 user.RowVersion,
		ExtraPermissions:        permissionLabels(user.Permissions),
		EffectivePermissions:    permissionLabels(effective),
	}
}

func userManagementFormView(actor authPrincipal, form ManagedUserFormView, creating, bootstrap bool) UserManagementView {
	roles := make([]UserRoleOptionView, 0, len(account.RoleDescriptors()))
	for _, descriptor := range account.RoleDescriptors() {
		disabled := false
		if bootstrap {
			disabled = descriptor.Name != account.RoleAdmin
		} else if descriptor.Name != account.RoleCustom && validateDelegatedAccess(actor, descriptor.Name, nil) != nil {
			disabled = true
		}
		roles = append(roles, UserRoleOptionView{
			Value:              descriptor.Name,
			Label:              descriptor.Label,
			Description:        descriptor.Description,
			GrantedPermissions: strings.Join(descriptor.Permissions, ","),
			Disabled:           disabled,
		})
	}
	roleGranted := map[string]bool{}
	for _, descriptor := range account.RoleDescriptors() {
		if descriptor.Name == form.Role {
			for _, permission := range descriptor.Permissions {
				roleGranted[permission] = true
			}
			break
		}
	}
	groupsByName := map[string][]UserPermissionOptionView{}
	groupOrder := []string{}
	for _, descriptor := range account.PermissionDescriptors() {
		if _, ok := groupsByName[descriptor.Group]; !ok {
			groupOrder = append(groupOrder, descriptor.Group)
		}
		assignable := !bootstrap
		if !account.IsAdministratorRole(actor.Role) {
			actorCapabilities := account.Capabilities(actor.capabilities)
			if descriptor.Name == account.PermissionSystemUsersManage || !actorCapabilities.HasAll(descriptor.Capability) {
				assignable = false
			}
		}
		disabled := !assignable
		groupsByName[descriptor.Group] = append(groupsByName[descriptor.Group], UserPermissionOptionView{
			Value:       descriptor.Name,
			Label:       descriptor.Label,
			Description: descriptor.Description,
			RoleGranted: roleGranted[descriptor.Name],
			Selected:    form.PermissionSelected(descriptor.Name),
			Assignable:  assignable,
			Disabled:    disabled,
		})
	}
	groups := make([]UserPermissionGroupView, 0, len(groupOrder))
	for _, name := range groupOrder {
		groups = append(groups, UserPermissionGroupView{Label: name, Permissions: groupsByName[name]})
	}
	return UserManagementView{
		Form:             form,
		Roles:            roles,
		PermissionGroups: groups,
		Creating:         creating,
		Bootstrap:        bootstrap,
		CanCreate:        bootstrap || actorCanCreateUser(actor),
		CurrentUsername:  actor.Username,
	}
}

func roleLabel(role string) string {
	for _, descriptor := range account.RoleDescriptors() {
		if descriptor.Name == role {
			return descriptor.Label
		}
	}
	if strings.TrimSpace(role) == "" {
		return "Benutzerdefiniert"
	}
	return role
}

func permissionLabels(names []string) []UserPermissionLabelView {
	labels := make(map[string]string, len(account.PermissionDescriptors()))
	for _, descriptor := range account.PermissionDescriptors() {
		labels[descriptor.Name] = descriptor.Label
	}
	seen := map[string]bool{}
	result := make([]UserPermissionLabelView, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		label := labels[name]
		if label == "" {
			label = name
		}
		result = append(result, UserPermissionLabelView{Value: name, Label: label})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Label < result[j].Label })
	return result
}

func permissionLabelsForCapabilities(capabilities account.Capabilities) []UserPermissionLabelView {
	names := make([]string, 0, len(account.PermissionDescriptors()))
	for _, descriptor := range account.PermissionDescriptors() {
		if capabilities.HasAll(descriptor.Capability) {
			names = append(names, descriptor.Name)
		}
	}
	return permissionLabels(names)
}

func (s *Server) accountViewForPrincipal(r *http.Request, principal authPrincipal, fieldErrors map[string]string) (AccountView, error) {
	user, found, err := s.databaseUserForPrincipal(r, principal)
	if err != nil {
		return AccountView{}, err
	}
	if found {
		effective, _ := account.EffectivePermissionNames(user.Role, user.Permissions)
		preference := s.accountPreference(principal.Source, principal.Subject)
		return AccountView{
			Username:                user.Username,
			Source:                  "database",
			SourceLabel:             "BearStack-Datenbank",
			Role:                    user.Role,
			RoleLabel:               roleLabel(user.Role),
			CanChangePassword:       true,
			CustomPDFPreviewEnabled: preference.CustomPDFPreviewEnabled,
			PreferenceVersion:       preference.RowVersion,
			EffectivePermissions:    permissionLabels(effective),
			FieldErrors:             fieldErrors,
		}, nil
	}
	for _, configured := range s.authConfigAccountViews() {
		if configured.Source != principal.Source || configured.Subject != principal.Subject {
			continue
		}
		preference := s.accountPreference(principal.Source, principal.Subject)
		return AccountView{
			Username:                configured.Username,
			Source:                  authSourceConfig,
			SourceLabel:             "Konfiguration",
			Role:                    configured.Role,
			RoleLabel:               roleLabel(configured.Role),
			CanChangePassword:       false,
			CustomPDFPreviewEnabled: preference.CustomPDFPreviewEnabled,
			PreferenceVersion:       preference.RowVersion,
			EffectivePermissions:    permissionLabelsForCapabilities(configured.Capabilities),
			FieldErrors:             fieldErrors,
		}, nil
	}
	return AccountView{}, sqlErrNoAccount
}

var sqlErrNoAccount = &accountViewError{"Konto nicht gefunden."}

type accountViewError struct{ message string }

func (e *accountViewError) Error() string { return e.message }

func (s *Server) databaseUserForPrincipal(r *http.Request, principal authPrincipal) (account.User, bool, error) {
	if principal.Source != authSourceDatabase || principal.AccountID < 1 {
		return account.User{}, false, nil
	}
	user, err := s.repo.UserByID(r.Context(), principal.AccountID)
	if errors.Is(err, sql.ErrNoRows) {
		return account.User{}, false, nil
	}
	if err != nil {
		return account.User{}, false, err
	}
	if user.Username != principal.Username {
		return account.User{}, false, errAuthPrincipalStale
	}
	return user, true, nil
}
