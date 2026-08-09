// Package account defines the shared user, role, and permission model.
package account

import "time"

// User is the safe projection used by administration and account pages. It
// deliberately never contains password material.
type User struct {
	ID             int64
	Username       string
	Role           string
	Permissions    []string
	Enabled        bool
	SessionVersion int64
	RowVersion     int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AuthenticationRecord is the privileged projection used to build the
// in-memory authentication snapshot. It must never be passed to templates.
type AuthenticationRecord struct {
	ID             int64
	Username       string
	PasswordHash   string
	Role           string
	Permissions    []string
	Enabled        bool
	SessionVersion int64
}

type CreateUserParams struct {
	Username     string
	PasswordHash string
	Role         string
	Permissions  []string
	Enabled      bool
}

type UpdateUserAccessParams struct {
	Role               string
	Permissions        []string
	ExpectedRowVersion int64
}

type UpdateUserPasswordParams struct {
	PasswordHash       string
	ExpectedRowVersion int64
}
