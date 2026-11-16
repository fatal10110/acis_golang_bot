GO ?= go

.PHONY: hooks build test test-race lint

# Enable the formatting and lint pre-commit hook for this clone.
hooks:
	git config core.hooksPath .githooks

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
