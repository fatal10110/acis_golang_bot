GO ?= go

.PHONY: hooks ui ui-check build test test-race lint e2e-live e2e-live-loginfail

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

# The live e2e scenarios against the server BOT_E2E_LOGIN names
# (docs/e2e-live.md); never in CI. Inside a package, the scenarios that call
# t.Parallel run MaxScenarios (2) at once, each with a GM of its own from the
# pool (a lock per GM): the GMs the run mints, rooted in .e2e/gm, gm2, ...,
# or the BOT_E2E_GM_FILE list. The packages run in parallel only
# from an allowlisted address (BOT_E2E_ALLOWLISTED=1): each test process has
# its own login gate, and two spaced gates dialing at once would burst the
# accept filter. Every process refuses to log in while the breaker's marker
# (.e2e/halted) exists, checked again before each scenario. The
# wrong-password scenario trips the breaker, so it runs alone
# (e2e-live-loginfail).
E2E_LIVE_P = $(if $(filter 1,$(BOT_E2E_ALLOWLISTED)),2,1)
E2E_LIVE_RUN = BOT_REQUIRE_E2E=1 $(GO) test -count=1 -v -timeout 60m -p $(E2E_LIVE_P) -parallel 2 -run '^TestLive' -skip '^TestLiveWrongPassword$$'

e2e-live:
	$(E2E_LIVE_RUN) ./internal/e2e/ ./cmd/bots/

e2e-live-loginfail:
	BOT_REQUIRE_E2E=1 BOT_E2E_ALLOW_LOGINFAIL=1 $(GO) test -count=1 -v -timeout 5m -run '^TestLiveWrongPassword$$' ./internal/e2e/
