GO ?= go

.PHONY: hooks build test test-race lint e2e-live e2e-live-loginfail

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

# The live e2e scenarios against the server BOT_E2E_LOGIN names
# (docs/e2e-live.md); never in CI. One package after the other, the second
# only when the first passed: each test process has its own login gate, two
# dialing at once would burst the accept filter, and a refused credential
# leaves the breaker's marker that fails every later run until it is
# deleted. The wrong-password scenario trips the breaker, so it runs alone
# (e2e-live-loginfail).
E2E_LIVE_RUN = BOT_REQUIRE_E2E=1 $(GO) test -count=1 -v -timeout 60m -run '^TestLive' -skip '^TestLiveWrongPassword$$'

e2e-live:
	$(E2E_LIVE_RUN) ./internal/e2e/ && $(E2E_LIVE_RUN) ./cmd/bots/

e2e-live-loginfail:
	BOT_REQUIRE_E2E=1 BOT_E2E_ALLOW_LOGINFAIL=1 $(GO) test -count=1 -v -timeout 5m -run '^TestLiveWrongPassword$$' ./internal/e2e/
