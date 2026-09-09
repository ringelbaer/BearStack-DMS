GO ?= go
NODE ?= node
NPM ?= npm
PYTHON ?= python3
FUZZTIME ?= 10s

.PHONY: test test-go test-js test-playwright build

test: test-go test-js

test-go:
	$(GO) test ./...

test-js:
	NODE="$(NODE)" ./scripts/check-js.sh

test-playwright: node_modules/@playwright/test/package.json
	GO="$(GO)" $(NPM) exec -- playwright test

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
