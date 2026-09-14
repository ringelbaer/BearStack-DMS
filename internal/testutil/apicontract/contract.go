// Package apicontract validates the shipped API contracts and recorded HTTP
// responses in tests. Production code must not import this package.
package apicontract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
)

//go:embed testdata/openapi-3.1.json
var openAPISchema []byte

func compiler() *jsonschema.Compiler {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(jsonschema.SchemeURLLoader{}) // No network or filesystem loading.
	c.AssertFormat()
	return c
}

var specificationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	var schema any
	if err := json.Unmarshal(openAPISchema, &schema); err != nil {
		return nil, err
	}
	c := compiler()
	const url = "https://spec.openapis.org/oas/3.1/schema/2025-09-15"
	if err := c.AddResource(url, schema); err != nil {
		return nil, err
	}
	return c.Compile(url)
})

type Document struct {
	Data        map[string]any
	definitions map[string]any
}

func Load(data []byte) (*Document, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("expected exactly one YAML document: %v", err)
	}
	// Normalize YAML numbers and maps to JSON values, also rejecting non-JSON keys.
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	normalized, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	var ok bool
	raw, ok = normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected OpenAPI object")
	}
	schema, err := specificationSchema()
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(raw); err != nil {
		return nil, fmt.Errorf("OpenAPI document: %w", err)
	}
	d := &Document{Data: raw, definitions: object(object(raw["components"])["schemas"])}
	// The official document schema deliberately excludes Schema Object validation.
	// Compile all Schema Objects as draft 2020-12 as a separate check.
	for name, value := range d.definitions {
		if _, err := d.compile(value); err != nil {
			return nil, fmt.Errorf("schema %s: %w", name, err)
		}
	}
	if err := d.check(raw); err != nil {
		return nil, err
	}
	return d, nil
}

func object(value any) map[string]any {
	valueMap, _ := value.(map[string]any)
	return valueMap
}

func (d *Document) pointer(ref string) (any, error) {
	// Contracts are intentionally self-contained. Validation never fetches URLs.
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("nonlocal reference %q", ref)
	}
	var value any = d.Data
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		var ok bool
		value, ok = object(value)[part]
		if !ok {
			return nil, fmt.Errorf("unresolved reference %q", ref)
		}
	}
	return value, nil
}

func (d *Document) check(value any) error {
	switch value := value.(type) {
	case map[string]any:
		if ref, ok := value["$ref"].(string); ok {
			if _, err := d.pointer(ref); err != nil {
				return err
			}
		}
		for key, child := range value {
			// Example/default values are application data, not OpenAPI references.
			if key == "examples" {
				// OpenAPI Examples Objects may be references. JSON Schema
				// examples are arrays of instance values and have no references.
				for _, example := range object(child) {
					if _, err := d.resolveObject(example); err != nil {
						return err
					}
				}
				continue
			}
			if key == "example" || key == "default" || key == "enum" || key == "const" {
				continue
			}
			if key == "schema" {
				if _, err := d.compile(child); err != nil {
					return err
				}
			}
			if err := d.check(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := d.check(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func rewriteReferences(value any) any {
	switch value := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(value))
		for key, child := range value {
			if key == "$ref" {
				if ref, ok := child.(string); ok {
					child = strings.Replace(ref, "#/components/schemas/", "#/$defs/", 1)
				}
			}
			if key == "example" || key == "examples" || key == "default" || key == "enum" || key == "const" {
				copy[key] = child
			} else {
				copy[key] = rewriteReferences(child)
			}
		}
		return copy
	case []any:
		copy := make([]any, len(value))
		for i, child := range value {
			copy[i] = rewriteReferences(child)
		}
		return copy
	default:
		return value
	}
}

func (d *Document) compile(value any) (*jsonschema.Schema, error) {
	wrapper := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$defs": d.definitions, "allOf": []any{value}}
	c := compiler()
	const url = "https://bearstack.invalid/contract-schema"
	if err := c.AddResource(url, rewriteReferences(wrapper)); err != nil {
		return nil, err
	}
	return c.Compile(url)
}

func (d *Document) resolveObject(value any) (map[string]any, error) {
	seen := map[string]bool{}
	for {
		obj := object(value)
		if obj == nil {
			return nil, fmt.Errorf("expected contract object")
		}
		ref, ok := obj["$ref"].(string)
		if !ok {
			return obj, nil
		}
		if seen[ref] {
			return nil, fmt.Errorf("reference cycle %s", ref)
		}
		seen[ref] = true
		var err error
		value, err = d.pointer(ref)
		if err != nil {
			return nil, err
		}
	}
}

// ValidateResponse checks a canonical OpenAPI path (including {parameters}),
// method, documented status, media type and the complete JSON response body.
func (d *Document) ValidateResponse(path, method string, status int, header http.Header, body []byte) error {
	operation := object(object(object(d.Data["paths"])[path])[strings.ToLower(method)])
	if operation == nil {
		return fmt.Errorf("undocumented operation %s %s", method, path)
	}
	responses := object(operation["responses"])
	response := responses[strconv.Itoa(status)]
	if response == nil {
		response = responses[fmt.Sprintf("%dXX", status/100)]
	}
	if response == nil {
		response = responses["default"]
	}
	if response == nil {
		return fmt.Errorf("undocumented response %d for %s %s", status, method, path)
	}
	definition, err := d.resolveObject(response)
	if err != nil {
		return err
	}
	content := object(definition["content"])
	if len(content) == 0 {
		if len(body) != 0 {
			return fmt.Errorf("unexpected response body")
		}
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		return err
	}
	media := object(content[mediaType])
	if media == nil {
		media = object(content["*/*"])
	}
	if media == nil {
		return fmt.Errorf("undocumented response media type %q", mediaType)
	}
	if mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return nil
	}
	payload, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return err
	}
	schema, err := d.compile(media["schema"])
	if err != nil {
		return err
	}
	return schema.Validate(payload)
}
