package bearstack

import (
	"os"
	"testing"

	"bearstack/internal/testutil/apicontract"
)

func TestOpenAPISpecMatchesApplicationVersion(t *testing.T) {
	spec, err := apicontract.Load([]byte(OpenAPISpec()))
	if err != nil {
		t.Fatal(err)
	}
	if got := spec.Data["info"].(map[string]any)["version"]; got != Version() {
		t.Fatalf("OpenAPI version %v, application %s", got, Version())
	}
}

func TestFaceServiceOpenAPISpec(t *testing.T) {
	data, err := os.ReadFile("services/faces/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apicontract.Load(data); err != nil {
		t.Fatal(err)
	}
}
