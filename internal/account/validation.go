package account

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	PasswordHashCost = 10
	MinPasswordRunes = 12
	MaxPasswordBytes = 72
	MaxUsernameRunes = 64
)

var (
	ErrUsernameRequired = errors.New("username is required")
	ErrUsernameTooLong  = errors.New("username is too long")
	ErrUsernameInvalid  = errors.New("username contains invalid characters")
	ErrPasswordTooShort = errors.New("password is too short")
	ErrPasswordTooLong  = errors.New("password is too long")
	ErrPasswordInvalid  = errors.New("password is not valid UTF-8")
	ErrPasswordHash     = errors.New("invalid password hash")
)

func NormalizeUsername(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrUsernameRequired
	}
	if !utf8.ValidString(value) {
		return "", ErrUsernameInvalid
	}
	if utf8.RuneCountInString(value) > MaxUsernameRunes {
		return "", ErrUsernameTooLong
	}
	for _, r := range value {
		if r == ':' || unicode.IsControl(r) {
			return "", ErrUsernameInvalid
		}
	}
	return value, nil
}

func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrPasswordInvalid
	}
	if utf8.RuneCountInString(password) < MinPasswordRunes {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordHashCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func ValidatePasswordHash(hash string) error {
	if hash == "" {
		return ErrPasswordHash
	}
	if _, err := bcrypt.Cost([]byte(hash)); err != nil {
		return fmt.Errorf("%w: %v", ErrPasswordHash, err)
	}
	return nil
}
