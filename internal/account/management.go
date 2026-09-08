package account

import "errors"

// ManagementActor contains the authenticated identity and effective rights needed
// for user delegation. Route admission and session validation remain with the caller.
type ManagementActor struct {
	Username     string
	Role         string
	Capabilities Capabilities
}

func (actor ManagementActor) ValidateDelegatedAccess(role string, permissions []string) error {
	if IsAdministratorRole(actor.Role) {
		return nil
	}
	if IsAdministratorRole(role) || IsUserManager(role, permissions) {
		return errors.New("Nur Administratoren dürfen Administratoren oder weitere Nutzerverwalter anlegen und verwalten.")
	}
	requested, err := CapabilitiesFor(role, permissions)
	if err != nil {
		return err
	}
	actorCapabilities := actor.Capabilities
	if requested&^actorCapabilities != 0 {
		return errors.New("Sie dürfen nur Rechte vergeben, die Sie selbst besitzen.")
	}
	return nil
}

func (actor ManagementActor) CanCreateUser() bool {
	if IsAdministratorRole(actor.Role) {
		return true
	}
	capabilities := actor.Capabilities &^ CapabilitySystemUsersManage
	return capabilities != 0
}

func (actor ManagementActor) CanManageUser(target User) bool {
	if actor.Username == "" || actor.Username == target.Username {
		return false
	}
	if IsAdministratorRole(actor.Role) {
		return true
	}
	if IsAdministratorRole(target.Role) || IsUserManager(target.Role, target.Permissions) {
		return false
	}
	targetCapabilities, err := CapabilitiesFor(target.Role, target.Permissions)
	if err != nil {
		return false
	}
	return targetCapabilities&^actor.Capabilities == 0
}
