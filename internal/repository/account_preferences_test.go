package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"bearstack/internal/account"
	"bearstack/internal/sqlutil"
)

func TestAccountPreferencesVersionedPersistenceAndValidation(t *testing.T) {
	ctx := context.Background()
	repo := openUserTestRepository(t)

	if preferences, err := repo.ListAccountPreferences(ctx); err != nil || len(preferences) != 0 {
		t.Fatalf("initial preferences = %#v, err %v", preferences, err)
	}
	created, err := repo.SaveAccountPreference(ctx, SaveAccountPreferenceParams{
		Source: AccountSourceConfig, Subject: "config-admin", CustomPDFPreviewEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.CustomPDFPreviewEnabled || created.RowVersion != 1 || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("created preference = %#v", created)
	}
	if _, err := repo.SaveAccountPreference(ctx, SaveAccountPreferenceParams{
		Source: AccountSourceConfig, Subject: "config-admin", ExpectedRowVersion: 0,
	}); !errors.Is(err, ErrAccountPreferenceConflict) {
		t.Fatalf("duplicate create error = %v", err)
	}
	updated, err := repo.SaveAccountPreference(ctx, SaveAccountPreferenceParams{
		Source: AccountSourceConfig, Subject: "config-admin", ExpectedRowVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CustomPDFPreviewEnabled || updated.RowVersion != 2 || !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("updated preference = %#v", updated)
	}
	if _, err := repo.SaveAccountPreference(ctx, SaveAccountPreferenceParams{
		Source: AccountSourceConfig, Subject: "config-admin", ExpectedRowVersion: 1,
	}); !errors.Is(err, ErrAccountPreferenceConflict) {
		t.Fatalf("stale update error = %v", err)
	}
	for _, params := range []SaveAccountPreferenceParams{{Source: "other", Subject: "x"}, {Source: AccountSourceConfig}} {
		if _, err := repo.SaveAccountPreference(ctx, params); err == nil {
			t.Fatalf("invalid preference accepted: %#v", params)
		}
	}
}

func TestRepositoryMigratesVersion15ToAccountPreferences(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "preferences-migration.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlutil.RecordSchemaVersion(ctx, db, repositorySchemaComponent, 15); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	version, found, err := sqlutil.CurrentSchemaVersion(ctx, repo.db, repositorySchemaComponent)
	if err != nil || !found || version != 16 {
		t.Fatalf("schema version=%d found=%v err=%v", version, found, err)
	}
	if _, err := repo.SaveAccountPreference(ctx, SaveAccountPreferenceParams{
		Source: AccountSourceConfig, Subject: "migrated-config", CustomPDFPreviewEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAccountPreferenceConcurrentUpdateAndDatabaseUserCleanup(t *testing.T) {
	ctx := context.Background()
	repo := openUserTestRepository(t)
	user := createUserForTest(t, repo, "preference-user", mustUserHash(t, "preference user password"), account.RoleDocumentsRead, nil, true)
	subject := strconv.FormatInt(user.ID, 10)
	created, err := repo.SaveAccountPreference(ctx, SaveAccountPreferenceParams{
		Source: AccountSourceDatabase, Subject: subject, CustomPDFPreviewEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errorsSeen := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, updateErr := repo.SaveAccountPreference(ctx, SaveAccountPreferenceParams{
				Source: AccountSourceDatabase, Subject: subject,
				CustomPDFPreviewEnabled: false, ExpectedRowVersion: created.RowVersion,
			})
			errorsSeen <- updateErr
		}()
	}
	wg.Wait()
	close(errorsSeen)
	succeeded, conflicted := 0, 0
	for err := range errorsSeen {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrAccountPreferenceConflict):
			conflicted++
		default:
			t.Fatalf("concurrent update error = %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent results succeeded=%d conflicted=%d", succeeded, conflicted)
	}

	current, err := repo.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DeleteUser(ctx, user.ID, current.RowVersion, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AccountPreference(ctx, AccountSourceDatabase, subject); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("preference after user deletion error = %v", err)
	}
}
