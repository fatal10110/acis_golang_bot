# Go style and design guide

This guide is loaded when a task needs detailed Go design guidance. Repository-wide behavioral
requirements remain in `CLAUDE.md`.

## Naming

- Package names are short, lowercase, and describe a domain or responsibility. Avoid stutter at call
  sites.
- Types are nouns; functions and methods are verbs or value names. Keep initialisms consistent:
  `ID`, `HP`, `MP`, `NPC`, `GM`, `URL`.
- Receivers are short and consistent. Do not use `this` or `self`.
- Treat `Manager`, `Holder`, `Impl`, `Base`, `Abstract`, `Helper`, `Util`, and `Info` as design-smell
  suffixes when they hide an unclear responsibility. Use a precise domain name instead.
- Name interfaces for behavior (`Reader`, `Attacker`, `Persister`), never with an `I` prefix.
- Avoid getter/setter walls. Export plain data when appropriate or expose a method such as `Name()`
  when access has behavior.

## Data and type shape

- Prefer composition to simulated inheritance. Embed or hold only genuinely shared concerns; do not
  build chains of base structs.
- Make the zero value useful where practical. Use a constructor when validation, dependencies, or
  initialization are required.
- Small immutable data is usually a value. Identity-bearing mutable entities are usually pointers.
  Do not make every value a pointer by habit.
- Use named IDs and enums when mixing raw values would be error-prone (object ids, template ids,
  opcodes). Preserve the concrete integer width the wire contract uses.
- Use `(T, bool)` for ordinary in-memory lookup absence and `(T, error)` for fallible work. Nil is
  idiomatic when it has one clear meaning; do not create nullable object graphs by default.
- Keep structs cohesive. Split independent responsibilities into fields or neighboring types when
  the aggregate becomes difficult to reason about.

## Interfaces

- Define interfaces in the consuming package, with only the methods that consumer calls.
- Accept interfaces and return concrete values.
- Do not add an interface for one implementation unless a real consumer needs a seam, such as a
  focused test double. A hypothetical future implementation is not enough.
- Prefer direct function parameters to service locators, registries of arbitrary values, or
  reflection-driven discovery.

## Errors and panic isolation

- Return `error` for expected failure and wrap it with useful operation context using `%w`.
- Use sentinel or typed errors only when callers must branch on the category; inspect with
  `errors.Is` or `errors.As`.
- Panic only for programmer errors and impossible invariants. Invalid packets, files, or database
  data return errors to the appropriate isolation boundary. A server frame in the world that
  fails to decode is logged (once per opcode per session, never its payload), counted and
  skipped: the session plays on (D45). Before the world a packet that fails to decode ends that
  bot's attempt with `errDecode`, a not-ready ending, never a server abort. Never the process.
- Recover at infrastructure isolation boundaries such as a bot's session loop or worker dispatch. Log the stack and terminate the affected unit of work.
  Do not recover merely to continue after an invariant violation, and do not require recovery in
  every short-lived goroutine.

## Concurrency

- State the owner of shared mutable state on the owning type or field: one goroutine, one mutex, or
  another explicit synchronization mechanism.
- Use channels to transfer ownership or coordinate work; use a mutex for a short critical section
  around shared state. Choose the simpler correct model.
- Prefer `sync.Mutex`. Use `sync.RWMutex` only for genuinely read-heavy state where contention or
  critical-section cost makes it useful; the existence of both readers and writers is not enough.
- A read path that mutates, including lazy eviction, takes the write lock.
- One goroutine writes a connection. Long-running work takes a `context.Context` or a `done`
  channel, stops on cancellation, and has no orphaned goroutines. A send on a channel that may be
  full also selects on `done`, so teardown never deadlocks.
- A buffer reused across reads never crosses goroutines: decode into values before handing data
  to another goroutine.
- Keep periodic callbacks short. Move blocking database or network work out of a scheduler's critical
  path while preserving lifecycle ownership.

### Lock order

When a lock is needed at all (most per-bot state is owned by the bot's brain goroutine and needs
none), a lock taken in the wrong order deadlocks two goroutines. The rules:

- **Leaf by default.** A leaf lock's critical section touches only the fields it guards, atomics
  and pure helpers. It calls no other object, takes no other lock, sends on no channel, and emits
  no event.
- **Defer the follow-ups.** Events, sends and refreshes that a change triggers run after the lock
  is released: collect them while locked, run them after `Unlock`.
- **Read prerequisites first.** Values a locked step needs from elsewhere are read before the lock
  is taken.
- **One owner at a time.** Never hold one object's lock while taking another's. Read from one,
  release, then lock the other.
- **Callbacks under a lock say so.** An interface method called with the caller's lock held
  documents it, and its implementations take only leaves below that lock.
- **A new nesting is documented** on the outer type, with a test that drives both paths
  concurrently and fails on a timeout.

Process-wide registries shared by many bots (the spot registry, the store registry, the login
rate limiter, the credential breaker) have one owner each and say so on the type.

## Numeric and contract fidelity

- Use explicit integer widths when overflow, truncation, signedness, or wire size is observable.
- Use explicit byte order and framing. Standard-library encoders are preferred when they express the
  exact contract clearly.
- Strings on the wire are the server's encoding (UTF-16LE, NUL-terminated) and length limits are
  counted the way the server counts them (UTF-16 units).
- Name compatibility algorithms (ciphers, key scrambling, checksums) for what they compute and
  document their contract.
- Code copied from the server (wire codecs, crypt) is owned here: adapt it to this module's
  boundaries, keep its byte-level behavior, and keep its tests or frozen vectors with it.

## Packages, files, and wiring

- Organize by responsibility under `internal/` (wire, crypt, client sessions, world model, brain,
  personas); keep `cmd/<binary>` as the composition and lifecycle boundary.
- Put a helper beside the type or concept it operates on. A storage or network package must not own a
  domain codec merely because it was the first caller.
- Keep files cohesive and navigable. Split when responsibilities can be understood independently or
  the file is difficult to work with; do not enforce one concern or one type per file mechanically.
- Avoid import cycles by repairing the boundary rather than adding indirection.
- Prefer explicit constructor wiring at the composition root; no dependency-injection container.
- Mutable package globals are a last resort. Constants and pure package functions are fine.

## Dependencies

Stop at the first rung that completely solves the real requirement:

1. The feature is unnecessary: do not add it.
2. The standard library covers it: use it.
3. An existing dependency covers it: reuse it.
4. A small established package owns a non-trivial subsystem better than local code would: evaluate
   it deliberately.
5. Otherwise write the smallest maintainable local implementation.

Use one logger across the repository (the one the module skeleton adopts). `golang.org/x/crypto` is
the expected crypto dependency. Do not build custom logging, dependency-injection, parsing, retry,
or pooling frameworks when the adopted library or standard library already owns the problem.

## Comments and documentation

- Exported declarations have doc comments beginning with the declaration name when required by the
  repository's lint policy.
- Comments explain an invariant, unit, ownership rule, compatibility contract, or non-obvious reason;
  they do not narrate syntax.
- Comments state the behavior or invariant, including the server rule a sender respects ("the
  server drops bypasses closer than 100 ms"). Server file paths belong in tests, issues and PRs,
  where they can be re-checked, not in production comments that rot.

## Anti-patterns

| Smell | Prefer |
| --- | --- |
| singleton `XManager` | a precisely named value constructed at the composition root |
| getter/setter for every field | plain data or one behavior-bearing accessor |
| one-implementation interface | the concrete type until a consumer needs a seam |
| base-struct inheritance chain | composition of small shared concerns |
| every value is `*T` | values for immutable data; pointers for mutable identity |
| panic for external input | a contextual error handled at an isolation boundary |
| unguarded shared map | one documented owner or mutex |
| `Util` grab bag | functions beside the domain concept they serve |
| decisions inside the reader goroutine | the reader decodes to typed events; the brain decides |
| hidden registration in `init` | explicit composition-root registration |
| `func`-typed callback field + `Set<Thing>Hook` setter per notification | small typed event values over one channel, handled in one type switch |
| `time.Now()` / `time.Sleep` in brain logic | the injected clock and timer |

## Worked example

This registry has explicit ownership, ordinary absence, and no speculative interface:

```go
type Registry struct {
	mu      sync.Mutex // guards objects
	objects map[ObjectID]Object
}

func NewRegistry() *Registry {
	return &Registry{objects: make(map[ObjectID]Object)}
}

func (r *Registry) Find(id ObjectID) (Object, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.objects[id]
	return o, ok
}
```

If profiling later shows read-lock contention is material and reads dominate, replace the mutex with
an `RWMutex` and add the corresponding race/performance evidence then.
