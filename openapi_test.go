package bearstack

import (
	"fmt"
	"strings"
	"testing"
)

func TestOpenAPISpecMatchesApplicationVersion(t *testing.T) {
	spec := OpenAPISpec()
	if strings.TrimSpace(spec) == "" {
		t.Fatal("OpenAPISpec() is empty")
	}
	for _, want := range []string{
		"openapi: 3.1.0",
		fmt.Sprintf("  version: %s", Version()),
		"  /api/openapi.yaml:",
		"  /api/documents:",
		"  /api/upload:",
	} {
		if !strings.Contains(spec, want) {
			t.Fatalf("OpenAPI description is missing %q", want)
		}
	}
}
