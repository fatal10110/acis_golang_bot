GO ?= go

.PHONY: hooks ui ui-check build test test-race lint

# Enable the formatting and lint pre-commit hook for this clone.
hooks:
	git config core.hooksPath .githooks

# The control panel's page: builds web/ (React, TypeScript, Vite) into
# internal/botpanel/web/app, which go:embed picks up. Needs Node.js 22 and
# npm. Run it before build: a binary built without it serves a placeholder.
ui:
	cd web && npm ci && npm run build

# The page's checks, as the CI web job runs them.
ui-check:
	cd web && npm ci && npm run typecheck && npm run lint && npm test

# Run `make ui` first for a binary that embeds the panel's page.
build:
	$(GO) build ./...

# The unit tier; needs no network, database or server stack.
test:
	$(GO) test ./...

# The unit tier with the race detector, as CI runs it.
test-race:
	$(GO) test -race ./...

# Formatting check and linters from .golangci.yml; both must report nothing.
lint:
	golangci-lint fmt --diff
	golangci-lint run
