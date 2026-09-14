package apicontract

import (
	"net/http"
	"strings"
	"testing"
)

const fixture = `openapi: 3.1.0
info: {title: Test, version: 1.0.0}
paths:
  /items:
    get:
      responses:
        '200':
          description: Items
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Item'}
        '400':
          description: Error
          content:
            application/json:
              schema: {type: object, required: [code], properties: {code: {const: invalid}}}
components:
  schemas:
    Item:
      type: object
      required: [id, name]
      properties:
        id: {type: integer, minimum: 1}
        name: {type: [string, 'null']}
`

func TestInvalidSpecificationsAreRejected(t *testing.T) {
	for name, data := range map[string]string{
		"null document":         "null",
		"yaml":                  "paths: [",
		"duplicate key":         fixture + "paths: {}\n",
		"multiple documents":    fixture + "---\n{}\n",
		"missing info":          strings.Replace(fixture, "info: {title: Test, version: 1.0.0}\n", "", 1),
		"unresolved ref":        strings.Replace(fixture, "schemas/Item", "schemas/Missing", 1),
		"remote ref":            strings.Replace(fixture, "#/components/schemas/Item", "https://example.invalid/schema", 1),
		"invalid schema":        strings.Replace(fixture, "type: integer", "type: nonsense", 1),
		"invalid example ref":   strings.Replace(fixture, "            application/json:", "            application/json:\n              examples: {sample: {$ref: '#/components/examples/missing'}}", 1),
		"invalid parameter ref": strings.Replace(fixture, "    get:", "    parameters: [{$ref: '#/components/parameters/missing'}]\n    get:", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load([]byte(data)); err == nil {
				t.Fatal("invalid specification accepted")
			}
		})
	}
}

func TestResponsesUseStatusMediaTypeAndSchema(t *testing.T) {
	doc, err := Load([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		status      int
		media, body string
		valid       bool
	}{
		{"success", 200, "application/json", `{"id":1,"name":"A"}`, true},
		{"nullable", 200, "application/json; charset=utf-8", `{"id":1,"name":null}`, true},
		{"error", 400, "application/json", `{"code":"invalid"}`, true},
		{"required", 200, "application/json", `{"id":1}`, false},
		{"type", 200, "application/json", `{"id":"1","name":"A"}`, false},
		{"range", 200, "application/json", `{"id":0,"name":"A"}`, false},
		{"error code", 400, "application/json", `{"code":"wrong"}`, false},
		{"status", 201, "application/json", `{"id":1,"name":"A"}`, false},
		{"media", 200, "text/html", `{"id":1,"name":"A"}`, false},
		{"trailing JSON", 200, "application/json", `{"id":1,"name":"A"}{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := doc.ValidateResponse("/items", "GET", tc.status, http.Header{"Content-Type": {tc.media}}, []byte(tc.body))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestExampleAndConstantReferencesAreInstanceData(t *testing.T) {
	data := strings.Replace(fixture, "id: {type: integer, minimum: 1}", "id: {type: integer, minimum: 1, examples: [1]}", 1)
	data = strings.Replace(data, "name: {type: [string, 'null']}", "name: {const: {$ref: '#/components/schemas/Item'}}", 1)
	doc, err := Load([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.ValidateResponse("/items", "GET", 200, http.Header{"Content-Type": {"application/json"}}, []byte(`{"id":1,"name":{"$ref":"#/components/schemas/Item"}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestResponsesPreserveInt64Precision(t *testing.T) {
	doc, err := Load([]byte(strings.Replace(fixture, "id: {type: integer, minimum: 1}", "id: {const: 9007199254740993}", 1)))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"9007199254740993", "9007199254740992"} {
		err := doc.ValidateResponse("/items", "GET", 200, http.Header{"Content-Type": {"application/json"}}, []byte(`{"id":`+id+`,"name":"A"}`))
		if (err == nil) != (id == "9007199254740993") {
			t.Fatalf("id %s: %v", id, err)
		}
	}
}
