package bearstack

import _ "embed"

//go:embed openapi.yaml
var openAPISpec string

// OpenAPISpec returns the OpenAPI description shipped with BearStack.
func OpenAPISpec() string {
	return openAPISpec
}
