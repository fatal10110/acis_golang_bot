# Agent tooling policy

Use the smallest local tool that answers the question and bound its output. Run commands against
a worktree with absolute paths (`git -C <worktree>`, `go -C <worktree>`); do not change the
persistent shell directory.

Choose the search primitive before writing a command. Do not default to `find`, POSIX `grep`, or
`sed` for repository discovery, code search, or file reading:

- filenames or path patterns: `rg --files` with bounded globs;
- known exact text: `rg` in a constrained path;
- Go definitions, references, implementations, diagnostics, or calls: `gopls`;
- behavior with unknown names or locations in the server: grepai, inside the server checkout;
- syntax-shaped matches or repetitive structural rewrites: ast-grep;
- known file and range: the native file-read tool.

After one exact `rg` miss, change tool categories; do not retry variants. RTK compresses noisy
tests, builds, vet, lint, git and logs; it is not a search tool.

| Tool | Use it for | Do not use it for | Fallback and output bound |
| --- | --- | --- | --- |
| `rg` | exact identifiers, opcodes, strings, constants, errors; `rg --files` for filenames | semantic guesses or call graphs | constrain path/glob and line range |
| `gopls` | Go definitions, references, implementations, diagnostics, call hierarchy | bulk text replacement | `rg` plus targeted reads; one symbol or file at a time |
| grepai | semantic location in the server checkout, caller/callee traces | known exact text or whole-file dumping | `rg`, `gopls`, targeted reads; about five results |
| `rtk` | noisy tests, builds, vet, lint, git, and CLI output | discovery, search or reading | the purpose-built tool |
| `jq` | compact JSON selection | unstructured text | a short script only when the query is complex |
| ast-grep | structural search and safe repetitive rewrites | ordinary text search or one-off edits | `rg` and manual targeted edits |

## gopls

Prefer language-server operations over reading several Go files:

```bash
gopls definition <file.go:line:column>
gopls references <file.go:line:column>
gopls implementation <file.go:line:column>
gopls call_hierarchy <file.go:line:column>
gopls workspace_symbol <Name>
gopls check <file.go>
```

Run it from the module root of the checkout you are asking about (this repository's worktree, or
the server checkout for server code); it sees uncommitted edits.

## grepai (server checkout)

```bash
(cd <server> && grepai search "<behavior>" --toon --compact --limit 5)
(cd <server> && grepai trace callers "<symbol>" --toon)
(cd <server> && grepai trace graph "<symbol>" --depth 2 --toon)
```

`--compact` requires `--json` or `--toon`. The index reflects the server's primary checkout. On an
error or a stale index, fall back to `rg` and `gopls` without retrying.

## Batched lookups

`scripts/agent-tools/look.py` runs several lookups in one call with capped output; use it whenever
the next step needs more than one:

```bash
python3 scripts/agent-tools/look.py --root <worktree> \
  internal/wire/reader.go:40-80 go:internal/wire:Reader.ReadS rg:internal:OpcodeUserInfo
```

Specs: `FILE:A-B`, `go:PATH:Name,Type.Method`, `gotest:PATH:TestName`, `rg:PATH:PATTERN`,
`rgl:PATH:PATTERN` (files with counts), `rgg:GLOB:PATH:PATTERN`. Absolute paths reach the server
checkout.

## rtk, jq, ast-grep

`rtk go -C <worktree> test|build|vet` for noisy Go commands, `rtk err <command>` for other verbose
checks, `rtk git diff|log|show` for long git output. `jq` selects only the JSON fields a decision
needs. `ast-grep run` or `ast-grep scan` only when AST structure makes text search or repetitive
edits unsafe.

## GitHub

GitHub MCP tools first, `gh -R fatal10110/acis_golang_bot` as fallback. GitHub is the only
authority for issue and PR state.
