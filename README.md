# acis_golang_bot

Playable bots for the [acis_golang](https://github.com/fatal10110/acis_golang) game server. Each
bot logs in and plays through the real client protocol. The design, owner decisions and milestones
are in [`docs/plan.md`](docs/plan.md).

## Build and run

Needs the Go version named in `go.mod`.

```bash
go build ./cmd/bots
./bots
```

The command does nothing yet; the client and personas arrive with the milestones in the plan.

## Deploy

Every published GitHub release carries `bots-linux-amd64`, `bots-windows-amd64.exe` and
`SHA256SUMS`, built by `.github/workflows/release.yml` from the release's tag.

Home run (provider range on the accept-filter allowlist, public game address, swarm flags, secret
file, GM control over SSH, WSL2): [`docs/home.md`](docs/home.md). The retired same-host run is
archived in [`docs/hetzner.md`](docs/hetzner.md).

## Develop

```bash
make hooks      # once per clone: format and lint staged Go files on commit
make test       # unit tests
make test-race  # unit tests with the race detector, as CI runs them
make lint       # gofumpt check and golangci-lint (v2, .golangci.yml); must print nothing
```

CI (`.github/workflows/go.yml`) runs build, vet, the unit tests with `-race`, and lint on every
pull request. The e2e scenarios against a server stack are not gated by PR CI
(`docs/agents/test-strategy.md`).
