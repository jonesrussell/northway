# Project layout and Go boundaries

Status: implementation blueprint. Directories shown below are created when their first implementation lands; empty packages and placeholder services are not required.

```text
cmd/northway/              # thin main; serve, ingest, migrate subcommands
internal/
  app/                    # configuration, dependency wiring, lifecycle
  httpapi/                # routing, auth middleware, JSON DTOs, error mapping
  identity/               # authenticated tenant/principal, scopes, key lifecycle
  source/                 # source identity, policy, observations and poll state
  article/                # canonical item identity, versions, provenance
  feed/                   # tenant feed definition and immutable revisions
  ingest/                 # fetch/parse/store orchestration and leases
  schedule/               # persisted due work, coalescing, discovery/digest jobs
  query/                  # context validation, retrieval, snapshot orchestration
  ranking/                # reranking contract, evidence validation, fallback
  feedback/               # scoped, reversible, idempotent preference events
  usage/                  # reservations, quota reconciliation, usage records
  sqlite/                 # concrete repositories and short transaction boundary
    sqlc/                 # generated query bindings; never hand-edited
  providers/anthropic/     # separate ranker/discovery adapters; SDK types stop here
  fetch/                  # bounded HTTP, DNS/redirect validation, gofeed adapter
api/
  schemas/                # versioned transport JSON schemas
  examples/               # synthetic, schema-checked examples
db/
  migrations/             # versioned Goose SQL migrations
  queries/                # sqlc source SQL
docs/
  specs/                  # behavior/invariants, stable requirement IDs
  decisions/              # ADRs for cross-cutting decisions
  delivery/               # issue index, project links and gates
evals/                    # permitted labelled fixtures and evaluation config
tests/integration/        # real file-backed SQLite, HTTP and isolation checks
scripts/                  # reproducible checks; no hidden deploy side effects
.github/                  # CI, dependency updates, issue and PR templates
```

One module: `github.com/jonesrussell/northway`. One executable, initially `northway serve`, `northway ingest --once`, and a migration command. No go.work, cross-service modules, generic repository framework, or public pkg/ tree. A public SDK is an explicit Phase 2 contract, not an accidental export of internal types. This follows Go's [server module layout guidance](https://go.dev/doc/modules/layout).

## Import rules

1. cmd imports app. app wires concrete dependencies and may import all internal packages.
2. Transport imports feature services and identity, never SQL or provider SDKs.
3. query orchestrates feed/article/ranking/usage/identity; ingest orchestrates source/article. Features do not import app, httpapi, sqlite, fetch or providers.
4. sqlite implements feature-owned consumer interfaces and may import feature types. Generated sqlc types remain inside sqlite adapters.
5. providers/anthropic implements ranking's small interface and optionally a separate ingest discovery interface; fetch implements feed acquisition interfaces. schedule coordinates ingest/query through narrow interfaces. None knows HTTP handlers or tenant UI models. Ranking remains tool-free; only discovery may use capped provider-side search.
6. No import of North Cloud, Claudriel or Waaseyaa application packages. Upstream taxonomy may be considered for a later domain import with a pinned version.

Feature services accept narrow interfaces only where substitutable behavior or a test seam is useful. Return concrete structs. Avoid an interface for every struct, a universal BaseRepository, service locators, reflection-based dependency injection, and horizontal utils/common packages. Cross-feature dependencies must be explicit and cycle-free; the runtime milestone adds an import-boundary check.

## Idiomatic Go baseline

- Go 1.27.0 is the stable baseline verified on 2026-08-30; update to patch releases through a tested dependency PR. Use gofmt and go vet, not a custom formatting doctrine.
- Standard net/http ServeMux, context, log/slog, errors, flag, testing/httptest, slices/maps and time. Prefer standard behavior to a web framework or logging library unless a documented gap appears.
- Pass context as the first parameter to I/O operations. Do not store it in long-lived structs; never replace a request/job context with context.Background inside a call chain.
- Use errors.Is/As and wrapping with %w for caller-actionable errors; map domain errors at the HTTP boundary. No panic for routine failure; no credential-bearing errors in responses/logs.
- Each goroutine has an owner, cancellation and termination path. Use errgroup.WithContext and SetLimit for bounded concurrent work; do not launch one goroutine per unbounded input item. Do not hold a database transaction while waiting on a model or network request.
- Reuse bounded HTTP clients/transports and database pools. Set body/time limits and close response bodies. Retry only eligible operations, with bounded jitter and respect for Retry-After and cancellation.
- Make invalid configuration fail startup. Explicit zero/false values must not be overwritten by magic defaults. Env/config parsing belongs in app, not scattered feature code.
- Use table-driven behavior tests, t.Cleanup, httptest, fuzzing for parsers/normalizers, the race detector and representative benchmarks. Test database authorization against the real file-backed database, not mocked SQL.
- Use newer language features where they simplify code. Generic methods are available in 1.27 but are not a reason to create an abstraction. encoding/json remains the compatibility default; adopting json/v2 requires explicit wire-compatibility review and duplicate-key/unknown-field tests.

References: [Go review comments](https://go.dev/wiki/CodeReviewComments), [Go 1.27 notes](https://go.dev/doc/go1.27), and [Go security tooling](https://go.dev/doc/security/). These inform the choices; no dependency compatibility or runtime test is implied by this blueprint.
