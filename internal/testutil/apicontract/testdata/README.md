# OpenAPI validation schema

`openapi-3.1.json` is the unmodified OpenAPI Initiative document schema from
https://spec.openapis.org/oas/3.1/schema/2025-09-15, downloaded on 2026-09-14.
SHA-256: `d0a3955182364c7b5fdebfd0583ecad259a870b4a2fe86a1b0fe8785f8224fed`.
OpenAPI specification material is licensed under Apache-2.0:
https://github.com/OAI/OpenAPI-Specification/blob/main/LICENSE.

It validates OpenAPI 3.1 document structure. Schema Objects are compiled separately
as JSON Schema draft 2020-12. All contract references must resolve locally; tests
never fetch external schemas. Standard JSON Schema `format` assertions (including dates and URIs) are enabled.
