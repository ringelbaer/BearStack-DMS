package mailservice

import (
	"strings"
	"testing"
)

func TestMailMessageAuditTargetIncludesFailureDetails(t *testing.T) {
	target := mailMessageAuditTarget(42, MessageResult{
		From:    "sender@example.com",
		Subject: "Import",
		PDFs:    1,
		EMLs:    1,
		Errors:  1,
		Details: []string{
			"rechnung.pdf: importiert",
			"kunde.eml: chromium ist lokal nicht installiert oder nicht im PATH",
		},
	})

	if !strings.Contains(target, "1 Fehler") {
		t.Fatalf("target misses error count: %q", target)
	}
	if !strings.Contains(target, "Details rechnung.pdf: importiert; kunde.eml: chromium ist lokal nicht installiert oder nicht im PATH") {
		t.Fatalf("target misses details: %q", target)
	}
}
