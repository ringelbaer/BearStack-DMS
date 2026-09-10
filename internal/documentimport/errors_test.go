package documentimport

import (
	"errors"
	"strings"
	"testing"
)

func TestFriendlyUploadErrorMasksUnexpectedDetails(t *testing.T) {
	got := UploadErrorMessage(errors.New("open /private/tmp/bearstack-secret/documents/.tmp: permission denied"))
	if got != "Datei konnte nicht verarbeitet werden" {
		t.Fatalf("error = %q", got)
	}
	if strings.Contains(got, "/private/tmp") || strings.Contains(got, "permission denied") {
		t.Fatalf("internal detail leaked: %q", got)
	}
}
