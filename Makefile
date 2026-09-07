GO ?= go
NODE ?= node
NPM ?= npm

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
.PHONY: test-android test-android-integration
test-android:
	./scripts/check-android.sh

test-android-integration:
	./scripts/test-android-integration.sh
