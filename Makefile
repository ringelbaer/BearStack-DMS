GO ?= go
NODE ?= node
NPM ?= npm
PYTHON ?= python3
export BEARSTACK_TEST_FACE_PYTHON ?= $(PYTHON)
FUZZTIME ?= 10s

.PHONY: test test-fast test-release test-race test-readonly test-go test-openapi test-js test-playwright build

# The standard gate must fail, rather than silently skip, missing toolchains or
# a genuinely read-only photo root. Use test-fast explicitly for the local loop.
test: test-fast test-playwright test-faces test-android test-readonly

test-fast: test-go test-js

test-release: test test-race test-android-release

test-race:
	$(GO) test -race ./internal/photos ./internal/server

test-readonly:
	GO="$(GO)" ./scripts/test-photos-readonly.sh

test-go:
	$(GO) test ./...

test-openapi:
	$(GO) test . ./internal/testutil/apicontract ./internal/server -run 'Test(OpenAPI|FaceServiceOpenAPI|InvalidSpecifications|Responses|ExampleAndConstantReferences)'

test-js:
	NODE="$(NODE)" ./scripts/check-js.sh

test-playwright: node_modules/@playwright/test/package.json
	GO="$(GO)" BEARSTACK_FACE_SUGGESTION_PERF=1 $(NPM) exec -- playwright test

node_modules/@playwright/test/package.json: package.json package-lock.json
	$(NPM) ci --ignore-scripts

build:
	$(GO) build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack

# Independent Android checks; Go and Docker builds never invoke Gradle.
.PHONY: test-android test-android-integration test-android-release test-android-release-integration test-parsers fuzz-parsers test-faces
test-android:
	./scripts/check-android.sh

test-android-integration:
	./scripts/test-android-integration.sh

test-android-release:
	./scripts/check-android.sh --release

test-android-release-integration:
	./scripts/test-android-integration.sh --release

# Seed corpora run deterministically in normal Go tests; mutation runs are opt-in.
test-parsers:
	$(GO) test ./internal/photos -run '^Fuzz(EXIF|GPX|XMP)$$'

fuzz-parsers:
	$(GO) test ./internal/photos -run '^$$' -fuzz '^FuzzEXIF$$' -fuzztime=$(FUZZTIME)
	$(GO) test ./internal/photos -run '^$$' -fuzz '^FuzzGPX$$' -fuzztime=$(FUZZTIME)
	$(GO) test ./internal/photos -run '^$$' -fuzz '^FuzzXMP$$' -fuzztime=$(FUZZTIME)

test-faces:
	$(PYTHON) -m unittest discover -s services/faces/tests -p 'test_*.py'
