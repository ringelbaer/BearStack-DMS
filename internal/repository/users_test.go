package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/sqlutil"
)

func TestRepositoryUserCRUDVersionsAndPasswordIsolation(t *testing.T) {
	ctx := context.Background()
	repo := openUserTestRepository(t)

	firstHash := mustUserHash(t, "initial password for Alice")
	created, err := repo.CreateUser(ctx, account.CreateUserParams{
		Username:     "  Alice  ",
		PasswordHash: firstHash,
		Role:         account.RoleDocumentsRead,
		Permissions: []string{
			account.PermissionPhotosRead,
			account.PermissionDocumentsUpload,
			account.PermissionPhotosRead,
		},
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID <= 0 || created.Username != "Alice" || !created.Enabled {
		t.Fatalf("created user = %#v", created)
	}
	wantPermissions := []string{account.PermissionDocumentsUpload, account.PermissionPhotosRead}
	if !reflect.DeepEqual(created.Permissions, wantPermissions) {
		t.Fatalf("created permissions = %#v, want %#v", created.Permissions, wantPermissions)
	}
	assertUserVersions(t, created, 1, 1)
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("created timestamps are zero: %#v", created)
	}
	assertSafeUserProjection(t, created, firstHash)

	listed, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("ListUsers() = %#v", listed)
	}
	assertSafeUserProjection(t, listed[0], firstHash)
	listed[0].Permissions[0] = "mutated-by-caller"
	reloaded, err := repo.UserByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reloaded.Permissions, wantPermissions) {
		t.Fatalf("stored permissions changed through safe projection: %#v", reloaded.Permissions)
	}

	authRecords, err := repo.ListAuthenticationAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(authRecords) != 1 || authRecords[0].PasswordHash != firstHash {
		t.Fatalf("authentication records do not contain the stored hash: %#v", authRecords)
	}

	updated, err := repo.UpdateUserAccess(ctx, created.ID, account.UpdateUserAccessParams{
		Role:               account.RolePhotosEditor,
		Permissions:        []string{account.PermissionSystemAudit},
		ExpectedRowVersion: created.RowVersion,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != account.RolePhotosEditor || !reflect.DeepEqual(updated.Permissions, []string{account.PermissionSystemAudit}) {
		t.Fatalf("updated access = %#v", updated)
	}
	assertUserVersions(t, updated, 2, 2)

	secondHash := mustUserHash(t, "replacement password for Alice")
	updated, err = repo.UpdateUserPassword(ctx, created.ID, account.UpdateUserPasswordParams{
		PasswordHash:       secondHash,
		ExpectedRowVersion: updated.RowVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertUserVersions(t, updated, 3, 3)
	assertSafeUserProjection(t, updated, firstHash)
	assertSafeUserProjection(t, updated, secondHash)
	authRecords, err = repo.ListAuthenticationAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(authRecords) != 1 || authRecords[0].PasswordHash != secondHash || authRecords[0].SessionVersion != 3 {
		t.Fatalf("authentication record after password update = %#v", authRecords)
	}

	updated, err = repo.SetUserEnabled(ctx, created.ID, false, updated.RowVersion, 0)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled {
		t.Fatal("disabled user is still enabled")
	}
	assertUserVersions(t, updated, 4, 4)

	deleted, err := repo.DeleteUser(ctx, created.ID, updated.RowVersion, 0)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != created.ID || deleted.RowVersion != 4 {
		t.Fatalf("deleted user = %#v", deleted)
	}
	if _, err := repo.UserByID(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("UserByID(deleted) error = %v, want sql.ErrNoRows", err)
	}
	var permissionCount int
	if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM user_permissions WHERE user_id = ?`, created.ID).Scan(&permissionCount); err != nil {
		t.Fatal(err)
	}
	if permissionCount != 0 {
		t.Fatalf("deleted user has %d permission rows", permissionCount)
	}
}

func TestRepositoryUsernamesAreCaseSensitiveAndDuplicatesAreMapped(t *testing.T) {
	ctx := context.Background()
	repo := openUserTestRepository(t)
	hash := mustUserHash(t, "a sufficiently long password")

	first := createUserForTest(t, repo, "Alice", hash, account.RoleDocumentsRead, nil, true)
	if _, err := repo.CreateUser(ctx, account.CreateUserParams{
		Username:     " Alice ",
		PasswordHash: hash,
		Role:         account.RoleDocumentsRead,
		Enabled:      true,
	}); !errors.Is(err, ErrUserAlreadyExists) {
		t.Fatalf("duplicate CreateUser() error = %v, want %v", err, ErrUserAlreadyExists)
	}
	second := createUserForTest(t, repo, "alice", hash, account.RoleDocumentsRead, nil, true)
	if first.ID == second.ID {
		t.Fatal("case-distinct usernames received the same ID")
	}

	users, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 {
		t.Fatalf("ListUsers() returned %d users, want 2", len(users))
	}
	gotNames := []string{users[0].Username, users[1].Username}
	sort.Strings(gotNames)
	if !reflect.DeepEqual(gotNames, []string{"Alice", "alice"}) {
		t.Fatalf("usernames = %#v", gotNames)
	}
}

func TestRepositoryUserMutationsRejectStaleRowVersionsWithoutChanges(t *testing.T) {
	ctx := context.Background()
	repo := openUserTestRepository(t)
	hash := mustUserHash(t, "stale version original password")
	user := createUserForTest(t, repo, "stale-user", hash, account.RoleDocumentsRead, nil, true)

	current, err := repo.UpdateUserAccess(ctx, user.ID, account.UpdateUserAccessParams{
		Role:               account.RoleDocumentsEditor,
		ExpectedRowVersion: user.RowVersion,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertUserVersions(t, current, 2, 2)

	newHash := mustUserHash(t, "stale version replacement password")
	mutations := []struct {
		name string
		call func() error
	}{
		{name: "access", call: func() error {
			_, err := repo.UpdateUserAccess(ctx, user.ID, account.UpdateUserAccessParams{
				Role:               account.RolePhotosRead,
				ExpectedRowVersion: user.RowVersion,
			}, 0)
			return err
		}},
		{name: "password", call: func() error {
			_, err := repo.UpdateUserPassword(ctx, user.ID, account.UpdateUserPasswordParams{
				PasswordHash:       newHash,
				ExpectedRowVersion: user.RowVersion,
			})
			return err
		}},
		{name: "enabled", call: func() error {
			_, err := repo.SetUserEnabled(ctx, user.ID, false, user.RowVersion, 0)
			return err
		}},
		{name: "delete", call: func() error {
			_, err := repo.DeleteUser(ctx, user.ID, user.RowVersion, 0)
			return err
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			if err := mutation.call(); !errors.Is(err, ErrUserConflict) {
				t.Fatalf("mutation error = %v, want %v", err, ErrUserConflict)
			}
		})
	}

	reloaded, err := repo.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Role != current.Role || !reloaded.Enabled {
		t.Fatalf("stale mutations changed user: %#v", reloaded)
	}
	assertUserVersions(t, reloaded, 2, 2)
	authRecords, err := repo.ListAuthenticationAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(authRecords) != 1 || authRecords[0].PasswordHash != hash {
		t.Fatalf("stale password mutation changed hash: %#v", authRecords)
	}
}

func TestRepositoryProtectsLastActiveUserManager(t *testing.T) {
	operations := []struct {
		name string
		call func(context.Context, *Repository, account.User, int) error
	}{
		{name: "remove management access", call: func(ctx context.Context, repo *Repository, user account.User, external int) error {
			_, err := repo.UpdateUserAccess(ctx, user.ID, account.UpdateUserAccessParams{
				Role:               account.RoleDocumentsRead,
				ExpectedRowVersion: user.RowVersion,
			}, external)
			return err
		}},
		{name: "disable", call: func(ctx context.Context, repo *Repository, user account.User, external int) error {
			_, err := repo.SetUserEnabled(ctx, user.ID, false, user.RowVersion, external)
			return err
		}},
		{name: "delete", call: func(ctx context.Context, repo *Repository, user account.User, external int) error {
			_, err := repo.DeleteUser(ctx, user.ID, user.RowVersion, external)
			return err
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name+" without external manager", func(t *testing.T) {
			ctx := context.Background()
			repo := openUserTestRepository(t)
			user := createUserForTest(t, repo, "only-manager", mustUserHash(t, "only manager password"), account.RoleAdmin, nil, true)
			if err := operation.call(ctx, repo, user, 0); !errors.Is(err, ErrLastActiveUserManager) {
				t.Fatalf("operation error = %v, want %v", err, ErrLastActiveUserManager)
			}
			reloaded, err := repo.UserByID(ctx, user.ID)
			if err != nil {
				t.Fatalf("protected user missing after rollback: %v", err)
			}
			if reloaded.Role != account.RoleAdmin || !reloaded.Enabled {
				t.Fatalf("protected user changed: %#v", reloaded)
			}
			assertUserVersions(t, reloaded, 1, 1)
		})

		t.Run(operation.name+" with external manager", func(t *testing.T) {
			ctx := context.Background()
			repo := openUserTestRepository(t)
			user := createUserForTest(t, repo, "db-manager", mustUserHash(t, "database manager password"), account.RoleAdmin, nil, true)
			if err := operation.call(ctx, repo, user, 1); err != nil {
				t.Fatalf("operation with external manager: %v", err)
			}
		})
	}
}

func TestRepositoryAnotherActiveDatabaseManagerAllowsChange(t *testing.T) {
	ctx := context.Background()
	repo := openUserTestRepository(t)
	hash := mustUserHash(t, "another manager password")
	first := createUserForTest(t, repo, "first-manager", hash, account.RoleAdmin, nil, true)
	createUserForTest(t, repo, "delegated-manager", hash, account.RoleCustom, []string{account.PermissionSystemUsersManage}, true)

	updated, err := repo.SetUserEnabled(ctx, first.ID, false, first.RowVersion, 0)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled {
		t.Fatal("first manager remains enabled")
	}
	assertUserVersions(t, updated, 2, 2)
}

func TestRepositoryConcurrentManagerDemotionsPreserveActiveManager(t *testing.T) {
	ctx := context.Background()
	repo := openUserTestRepository(t)
	hash := mustUserHash(t, "concurrent manager password")
	first := createUserForTest(t, repo, "concurrent-manager-one", hash, account.RoleAdmin, nil, true)
	second := createUserForTest(t, repo, "concurrent-manager-two", hash, account.RoleAdmin, nil, true)

	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, user := range []account.User{first, second} {
		user := user
		go func() {
			ready.Done()
			<-start
			_, err := repo.UpdateUserAccess(ctx, user.ID, account.UpdateUserAccessParams{
				Role:               account.RoleDocumentsRead,
				ExpectedRowVersion: user.RowVersion,
			}, 0)
			results <- err
		}()
	}
	ready.Wait()
	close(start)

	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful concurrent demotions = %d, want exactly one", successes)
	}
	users, err := repo.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	activeManagers := 0
	for _, user := range users {
		if user.Enabled && account.IsUserManager(user.Role, user.Permissions) {
			activeManagers++
		}
	}
	if activeManagers != 1 {
		t.Fatalf("active managers after concurrent demotions = %d, users = %#v", activeManagers, users)
	}
}

func TestRepositoryMigratesVersion14AndCreatesUserSchema(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "migration.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlutil.RecordSchemaVersion(ctx, db, repositorySchemaComponent, 14); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	repo, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	version, found, err := sqlutil.CurrentSchemaVersion(ctx, repo.db, repositorySchemaComponent)
	if err != nil {
		t.Fatal(err)
	}
	if !found || version != 15 || repositorySchemaVersion != 15 {
		t.Fatalf("repository schema version = %d, found %v, supported %d; want 15", version, found, repositorySchemaVersion)
	}

	wantColumns := map[string]bool{
		"id": false, "username": false, "password_hash": false, "role": false,
		"enabled": false, "session_version": false, "row_version": false,
		"created_at": false, "updated_at": false,
	}
	rows, err := repo.db.QueryContext(ctx, `PRAGMA table_info(users)`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if _, ok := wantColumns[name]; ok {
			wantColumns[name] = true
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for name, found := range wantColumns {
		if !found {
			t.Errorf("users table is missing column %q", name)
		}
	}

	var permissionsTable string
	if err := repo.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'user_permissions'`).Scan(&permissionsTable); err != nil {
		t.Fatal(err)
	}
	if permissionsTable != "user_permissions" {
		t.Fatalf("permissions table = %q", permissionsTable)
	}

	user := createUserForTest(t, repo, "migrated-user", mustUserHash(t, "migrated schema password"), account.RoleCustom, []string{account.PermissionDocumentsRead}, true)
	if _, err := repo.db.ExecContext(ctx, `INSERT INTO user_permissions(user_id, permission) VALUES (?, ?)`, user.ID, account.PermissionPhotosRead); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, user.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := repo.db.QueryRowContext(ctx, `SELECT count(*) FROM user_permissions WHERE user_id = ?`, user.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("ON DELETE CASCADE left %d permission rows", count)
	}
}

func openUserTestRepository(t *testing.T) *Repository {
	t.Helper()
	repo, err := Open(context.Background(), filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repo.Close(); err != nil {
			t.Errorf("close repository: %v", err)
		}
	})
	return repo
}

func mustUserHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := account.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func createUserForTest(t *testing.T, repo *Repository, username, hash, role string, permissions []string, enabled bool) account.User {
	t.Helper()
	user, err := repo.CreateUser(context.Background(), account.CreateUserParams{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Permissions:  permissions,
		Enabled:      enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func assertUserVersions(t *testing.T, user account.User, wantSession, wantRow int64) {
	t.Helper()
	if user.SessionVersion != wantSession || user.RowVersion != wantRow {
		t.Fatalf("versions = session %d, row %d; want session %d, row %d", user.SessionVersion, user.RowVersion, wantSession, wantRow)
	}
}

func assertSafeUserProjection(t *testing.T, user account.User, secret string) {
	t.Helper()
	userType := reflect.TypeOf(user)
	for i := 0; i < userType.NumField(); i++ {
		name := strings.ToLower(userType.Field(i).Name)
		if strings.Contains(name, "password") || strings.Contains(name, "hash") {
			t.Fatalf("safe account.User exposes sensitive field %q", userType.Field(i).Name)
		}
	}
	if strings.Contains(fmt.Sprintf("%#v", user), secret) {
		t.Fatal("safe account.User projection contains password hash")
	}
}
