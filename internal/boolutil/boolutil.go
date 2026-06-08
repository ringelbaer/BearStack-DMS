// Package boolutil normalisiert boolesche Formular-, Query- und Env-Werte.
package boolutil

import "strings"

func Parse(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func Truthy(value string) bool {
	parsed, ok := Parse(value)
	return ok && parsed
}

func Falsey(value string) bool {
	parsed, ok := Parse(value)
	return ok && !parsed
}
