package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestSaveSettingsRollsBackWholeForm(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	old := map[string]string{"a": "old", "z": "old"}
	if err := repo.SaveSettings(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.Exec(`CREATE TRIGGER reject_setting BEFORE INSERT ON settings WHEN new.key='z' BEGIN SELECT RAISE(ABORT,'write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSettings(ctx, map[string]string{"a": "new", "new": "insert", "z": "new"}); err == nil {
		t.Fatal("expected write failure")
	}
	got, err := repo.GetSettings(ctx, "a", "new", "z")
	if err != nil || !reflect.DeepEqual(got, old) {
		t.Fatalf("partial save: %v, %v", got, err)
	}
}

func TestSaveSettingsRejectsInvalidKeysAndCancellation(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	for _, values := range []map[string]string{{"a": "value", " ": "invalid"}, {"a": "one", " a ": "two"}} {
		if err := repo.SaveSettings(ctx, values); err == nil {
			t.Fatal("accepted invalid keys")
		}
		got, err := repo.GetSettings(ctx, "a")
		if err != nil || len(got) != 0 {
			t.Fatalf("invalid save wrote values: %v %v", got, err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := repo.SaveSettings(canceled, map[string]string{"a": "new"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if err := repo.SaveSettings(ctx, map[string]string{" a ": "valid"}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetSettings(ctx, " a ", "missing")
	if err != nil || !reflect.DeepEqual(got, map[string]string{"a": "valid"}) {
		t.Fatalf("normalized keys: %v %v", got, err)
	}
}

func TestSettingsSnapshotsUnderConcurrentSaves(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(ctx, filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.SaveSettings(ctx, map[string]string{"a": "0", "b": "0"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for worker := range 3 {
		wg.Go(func() {
			for i := range 20 {
				value := fmt.Sprintf("%d/%d", worker, i)
				if err := repo.SaveSettings(ctx, map[string]string{"a": value, "b": value}); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	for range 100 {
		values, err := repo.GetSettings(ctx, "a", "b")
		if err != nil || values["a"] != values["b"] {
			t.Errorf("mixed snapshot: %v %v", values, err)
			break
		}
	}
	wg.Wait()
}
