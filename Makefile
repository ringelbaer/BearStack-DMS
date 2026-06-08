GO ?= go
NODE ?= node
NPM ?= npm
PLAYWRIGHT_TEST_VERSION ?= 1.60.0

.PHONY: test test-go test-js test-playwright build

test: test-go test-js

test-go:
	$(GO) test ./...

test-js:
	NODE="$(NODE)" ./scripts/check-js.sh

test-playwright:
	$(NPM) exec --yes --package=@playwright/test@$(PLAYWRIGHT_TEST_VERSION) -- playwright test

build:
	$(GO) build -trimpath -ldflags="-s -w" -o bearstack ./cmd/bearstack
