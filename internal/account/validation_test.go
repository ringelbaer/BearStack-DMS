package account

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "trim whitespace", input: "  Alice Example  ", want: "Alice Example"},
		{name: "case preserved", input: "Alice", want: "Alice"},
		{name: "maximum unicode runes", input: strings.Repeat("ä", MaxUsernameRunes), want: strings.Repeat("ä", MaxUsernameRunes)},
		{name: "empty", input: " \t ", wantErr: ErrUsernameRequired},
		{name: "too many unicode runes", input: strings.Repeat("ä", MaxUsernameRunes+1), wantErr: ErrUsernameTooLong},
		{name: "colon", input: "alice:admin", wantErr: ErrUsernameInvalid},
		{name: "control character", input: "alice\tbob", wantErr: ErrUsernameInvalid},
		{name: "invalid utf8", input: string([]byte{'a', 0xff, 'b'}), wantErr: ErrUsernameInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeUsername(tt.input)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("NormalizeUsername() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("NormalizeUsername() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidatePasswordCountsRunesAndLimitsUTF8Bytes(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr error
	}{
		{name: "minimum ascii runes", value: strings.Repeat("a", MinPasswordRunes)},
		{name: "unicode minimum", value: strings.Repeat("界", MinPasswordRunes)},
		{name: "exactly seventy two bytes", value: strings.Repeat("界", MaxPasswordBytes/len("界"))},
		{name: "too few unicode runes", value: strings.Repeat("界", MinPasswordRunes-1), wantErr: ErrPasswordTooShort},
		{name: "more than seventy two utf8 bytes", value: strings.Repeat("界", MaxPasswordBytes/len("界")+1), wantErr: ErrPasswordTooLong},
		{name: "invalid utf8", value: string([]byte{0xff, 'a', 'b'}), wantErr: ErrPasswordInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(tt.value); tt.wantErr == nil && got > MaxPasswordBytes {
				t.Fatalf("test password uses %d bytes, maximum is %d", got, MaxPasswordBytes)
			}
			if tt.wantErr == nil && utf8.RuneCountInString(tt.value) < MinPasswordRunes {
				t.Fatalf("test password has fewer than %d runes", MinPasswordRunes)
			}
			if err := ValidatePassword(tt.value); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidatePassword() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashPasswordUsesConfiguredCostAndValidatesInput(t *testing.T) {
	password := "correct horse battery staple"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if hash == password {
		t.Fatal("HashPassword returned the clear-text password")
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatal(err)
	}
	if cost != PasswordHashCost {
		t.Fatalf("bcrypt cost = %d, want %d", cost, PasswordHashCost)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("compare password: %v", err)
	}
	if err := ValidatePasswordHash(hash); err != nil {
		t.Fatalf("ValidatePasswordHash(valid) = %v", err)
	}
	if err := ValidatePasswordHash("not-a-bcrypt-hash"); !errors.Is(err, ErrPasswordHash) {
		t.Fatalf("ValidatePasswordHash(invalid) = %v, want %v", err, ErrPasswordHash)
	}
	if _, err := HashPassword("too short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("HashPassword(short) = %v, want %v", err, ErrPasswordTooShort)
	}
}
