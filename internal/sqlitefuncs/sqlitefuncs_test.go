package sqlitefuncs

import (
	"context"
	"database/sql"
	"testing"

	"bearstack/internal/searchtext"

	_ "modernc.org/sqlite"
)

func TestRegisterGermanFoldIdempotentAndQueryable(t *testing.T) {
	if err := RegisterGermanFold(); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := RegisterGermanFold(); err != nil {
		t.Fatalf("second register failed: %v", err)
	}

	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	input := "März Straße 2024"
	var got string
	if err := db.QueryRowContext(context.Background(), `SELECT bearstack_german_fold(?)`, input).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if want := searchtext.GermanFold(input); got != want {
		t.Fatalf("bearstack_german_fold(%q) = %q, want %q", input, got, want)
	}
}

func TestGermanFoldHandlesNull(t *testing.T) {
	if err := RegisterGermanFold(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var got string
	if err := db.QueryRow(`SELECT bearstack_german_fold(NULL)`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("bearstack_german_fold(NULL) = %q, want empty string", got)
	}
}
