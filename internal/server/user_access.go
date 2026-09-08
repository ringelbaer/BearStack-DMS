package server

import "bearstack/internal/account"

func (actor authPrincipal) managementActor() account.ManagementActor {
	return account.ManagementActor{Username: actor.Username, Role: actor.Role, Capabilities: account.Capabilities(actor.capabilities)}
}
