// Datei speichert die über die Weboberfläche verwalteten Benutzerkonten.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"bearstack/internal/account"
)

var (
	ErrUserAlreadyExists     = errors.New("Benutzername existiert bereits")
	ErrUserConflict          = errors.New("Benutzer wurde zwischenzeitlich geändert")
	ErrLastActiveUserManager = errors.New("der letzte aktive Nutzerverwalter kann nicht geändert werden")
)

type userQueryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type storedUser struct {
	account.User
	passwordHash string
}

func (r *Repository) ListUsers(ctx context.Context) ([]account.User, error) {
	users, err := listStoredUsers(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]account.User, len(users))
	for i := range users {
		result[i] = cloneUser(users[i].User)
	}
	return result, nil
}

func (r *Repository) UserByID(ctx context.Context, id int64) (account.User, error) {
	user, err := storedUserByID(ctx, r.db, id)
	if err != nil {
		return account.User{}, err
	}
	return cloneUser(user.User), nil
}

func (r *Repository) ListAuthenticationAccounts(ctx context.Context) ([]account.AuthenticationRecord, error) {
	users, err := listStoredUsers(ctx, r.db)
	if err != nil {
		return nil, err
	}
	result := make([]account.AuthenticationRecord, 0, len(users))
	for _, user := range users {
		result = append(result, account.AuthenticationRecord{
			ID:             user.ID,
			Username:       user.Username,
			PasswordHash:   user.passwordHash,
			Role:           user.Role,
			Permissions:    append([]string(nil), user.Permissions...),
			Enabled:        user.Enabled,
			SessionVersion: user.SessionVersion,
		})
	}
	return result, nil
}

func (r *Repository) CreateUser(ctx context.Context, params account.CreateUserParams) (account.User, error) {
	username, err := account.NormalizeUsername(params.Username)
	if err != nil {
		return account.User{}, err
	}
	if err := account.ValidatePasswordHash(params.PasswordHash); err != nil {
		return account.User{}, err
	}
	role, permissions, _, err := account.NormalizeAccess(params.Role, params.Permissions)
	if err != nil {
		return account.User{}, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return account.User{}, err
	}
	defer tx.Rollback()

	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, username).Scan(&existingID)
	if err == nil {
		return account.User{}, ErrUserAlreadyExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return account.User{}, err
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO users(username, password_hash, role, enabled, session_version, row_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, 1, ?, ?)`,
		username, params.PasswordHash, role, boolInt(params.Enabled), formatTime(now), formatTime(now))
	if err != nil {
		return account.User{}, mapUserWriteError(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return account.User{}, err
	}
	if err := replaceUserPermissions(ctx, tx, id, permissions); err != nil {
		return account.User{}, err
	}
	created, err := storedUserByID(ctx, tx, id)
	if err != nil {
		return account.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return account.User{}, err
	}
	return cloneUser(created.User), nil
}

func (r *Repository) UpdateUserAccess(ctx context.Context, id int64, params account.UpdateUserAccessParams, externalActiveManagers int) (account.User, error) {
	role, permissions, capabilities, err := account.NormalizeAccess(params.Role, params.Permissions)
	if err != nil {
		return account.User{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return account.User{}, err
	}
	defer tx.Rollback()

	current, err := storedUserByID(ctx, tx, id)
	if err != nil {
		return account.User{}, err
	}
	if current.RowVersion != params.ExpectedRowVersion {
		return account.User{}, ErrUserConflict
	}
	currentManager := current.Enabled && account.IsUserManager(current.Role, current.Permissions)
	newManager := current.Enabled && capabilities.HasAll(account.CapabilitySystemUsersManage)
	if currentManager && !newManager {
		if err := ensureAnotherActiveUserManager(ctx, tx, id, externalActiveManagers); err != nil {
			return account.User{}, err
		}
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET role = ?, session_version = session_version + 1,
			row_version = row_version + 1, updated_at = ?
		WHERE id = ? AND row_version = ?`,
		role, formatTime(time.Now().UTC()), id, params.ExpectedRowVersion)
	if err != nil {
		return account.User{}, err
	}
	if err := requireUserUpdate(result); err != nil {
		return account.User{}, err
	}
	if err := replaceUserPermissions(ctx, tx, id, permissions); err != nil {
		return account.User{}, err
	}
	updated, err := storedUserByID(ctx, tx, id)
	if err != nil {
		return account.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return account.User{}, err
	}
	return cloneUser(updated.User), nil
}

func (r *Repository) UpdateUserPassword(ctx context.Context, id int64, params account.UpdateUserPasswordParams) (account.User, error) {
	if err := account.ValidatePasswordHash(params.PasswordHash); err != nil {
		return account.User{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return account.User{}, err
	}
	defer tx.Rollback()
	if _, err := userWithExpectedVersion(ctx, tx, id, params.ExpectedRowVersion); err != nil {
		return account.User{}, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, session_version = session_version + 1,
			row_version = row_version + 1, updated_at = ?
		WHERE id = ? AND row_version = ?`,
		params.PasswordHash, formatTime(time.Now().UTC()), id, params.ExpectedRowVersion)
	if err != nil {
		return account.User{}, err
	}
	if err := requireUserUpdate(result); err != nil {
		return account.User{}, err
	}
	updated, err := storedUserByID(ctx, tx, id)
	if err != nil {
		return account.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return account.User{}, err
	}
	return cloneUser(updated.User), nil
}

func (r *Repository) SetUserEnabled(ctx context.Context, id int64, enabled bool, expectedRowVersion int64, externalActiveManagers int) (account.User, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return account.User{}, err
	}
	defer tx.Rollback()
	current, err := userWithExpectedVersion(ctx, tx, id, expectedRowVersion)
	if err != nil {
		return account.User{}, err
	}
	if current.Enabled && !enabled && account.IsUserManager(current.Role, current.Permissions) {
		if err := ensureAnotherActiveUserManager(ctx, tx, id, externalActiveManagers); err != nil {
			return account.User{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE users
		SET enabled = ?, session_version = session_version + 1,
			row_version = row_version + 1, updated_at = ?
		WHERE id = ? AND row_version = ?`,
		boolInt(enabled), formatTime(time.Now().UTC()), id, expectedRowVersion)
	if err != nil {
		return account.User{}, err
	}
	if err := requireUserUpdate(result); err != nil {
		return account.User{}, err
	}
	updated, err := storedUserByID(ctx, tx, id)
	if err != nil {
		return account.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return account.User{}, err
	}
	return cloneUser(updated.User), nil
}

func (r *Repository) DeleteUser(ctx context.Context, id, expectedRowVersion int64, externalActiveManagers int) (account.User, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return account.User{}, err
	}
	defer tx.Rollback()
	current, err := userWithExpectedVersion(ctx, tx, id, expectedRowVersion)
	if err != nil {
		return account.User{}, err
	}
	if current.Enabled && account.IsUserManager(current.Role, current.Permissions) {
		if err := ensureAnotherActiveUserManager(ctx, tx, id, externalActiveManagers); err != nil {
			return account.User{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_preferences WHERE account_source = ? AND account_subject = ?`, AccountSourceDatabase, fmt.Sprint(id)); err != nil {
		return account.User{}, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ? AND row_version = ?`, id, expectedRowVersion)
	if err != nil {
		return account.User{}, err
	}
	if err := requireUserUpdate(result); err != nil {
		return account.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return account.User{}, err
	}
	return cloneUser(current.User), nil
}

func listStoredUsers(ctx context.Context, q userQueryer) ([]storedUser, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, username, password_hash, role, enabled, session_version, row_version, created_at, updated_at
		FROM users
		ORDER BY lower(username) ASC, username ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]storedUser, 0)
	byID := make(map[int64]int)
	for rows.Next() {
		user, err := scanStoredUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
		byID[user.ID] = len(users) - 1
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	permissionRows, err := q.QueryContext(ctx, `SELECT user_id, permission FROM user_permissions ORDER BY user_id, permission`)
	if err != nil {
		return nil, err
	}
	defer permissionRows.Close()
	for permissionRows.Next() {
		var userID int64
		var permission string
		if err := permissionRows.Scan(&userID, &permission); err != nil {
			return nil, err
		}
		if index, ok := byID[userID]; ok {
			users[index].Permissions = append(users[index].Permissions, permission)
		}
	}
	return users, permissionRows.Err()
}

func storedUserByID(ctx context.Context, q userQueryer, id int64) (storedUser, error) {
	if id <= 0 {
		return storedUser{}, sql.ErrNoRows
	}
	row := q.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, enabled, session_version, row_version, created_at, updated_at
		FROM users WHERE id = ?`, id)
	user, err := scanStoredUser(row)
	if err != nil {
		return storedUser{}, err
	}
	rows, err := q.QueryContext(ctx, `SELECT permission FROM user_permissions WHERE user_id = ? ORDER BY permission`, id)
	if err != nil {
		return storedUser{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return storedUser{}, err
		}
		user.Permissions = append(user.Permissions, permission)
	}
	return user, rows.Err()
}

type userScanner interface {
	Scan(...any) error
}

func scanStoredUser(scanner userScanner) (storedUser, error) {
	var user storedUser
	var enabled int
	var createdAt, updatedAt string
	if err := scanner.Scan(&user.ID, &user.Username, &user.passwordHash, &user.Role, &enabled,
		&user.SessionVersion, &user.RowVersion, &createdAt, &updatedAt); err != nil {
		return storedUser{}, err
	}
	user.Enabled = enabled != 0
	var err error
	user.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return storedUser{}, fmt.Errorf("parse user created_at: %w", err)
	}
	user.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return storedUser{}, fmt.Errorf("parse user updated_at: %w", err)
	}
	return user, nil
}

func replaceUserPermissions(ctx context.Context, q userQueryer, id int64, permissions []string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM user_permissions WHERE user_id = ?`, id); err != nil {
		return err
	}
	for _, permission := range permissions {
		if _, err := q.ExecContext(ctx, `INSERT INTO user_permissions(user_id, permission) VALUES (?, ?)`, id, permission); err != nil {
			return err
		}
	}
	return nil
}

func userWithExpectedVersion(ctx context.Context, q userQueryer, id, expected int64) (storedUser, error) {
	user, err := storedUserByID(ctx, q, id)
	if err != nil {
		return storedUser{}, err
	}
	if expected <= 0 || user.RowVersion != expected {
		return storedUser{}, ErrUserConflict
	}
	return user, nil
}

func ensureAnotherActiveUserManager(ctx context.Context, q userQueryer, excludedID int64, externalActiveManagers int) error {
	if externalActiveManagers > 0 {
		return nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT u.id, u.role, p.permission
		FROM users u
		LEFT JOIN user_permissions p ON p.user_id = u.id
		WHERE u.enabled = 1 AND u.id != ?
		ORDER BY u.id, p.permission`, excludedID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type access struct {
		role        string
		permissions []string
	}
	users := map[int64]*access{}
	for rows.Next() {
		var id int64
		var role string
		var permission sql.NullString
		if err := rows.Scan(&id, &role, &permission); err != nil {
			return err
		}
		item := users[id]
		if item == nil {
			item = &access{role: role}
			users[id] = item
		}
		if permission.Valid {
			item.permissions = append(item.permissions, permission.String)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, user := range users {
		if account.IsUserManager(user.role, user.permissions) {
			return nil
		}
	}
	return ErrLastActiveUserManager
}

func requireUserUpdate(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrUserConflict
	}
	return nil
}

func mapUserWriteError(err error) error {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed: users.username") {
		return ErrUserAlreadyExists
	}
	return err
}

func cloneUser(user account.User) account.User {
	user.Permissions = append([]string(nil), user.Permissions...)
	sort.Strings(user.Permissions)
	return user
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
