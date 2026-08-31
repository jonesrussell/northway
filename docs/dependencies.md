# Dependency decisions

Verified 2026-08-30 using official Go release metadata and proxy.golang.org module metadata. Exact baseline versions are recorded in [the dependency manifest](../dependencies.json). This is an approved selection, not an installed dependency graph: add a module only in the first PR that uses it, and commit go.mod/go.sum then. Build, vulnerability and license checks still precede acceptance.

| Purpose | Decision / baseline | Reason |
|---|---|---|
| Language/toolchain | Go 1.27.0 | Current stable release; single module; no experimental flags. |
| HTTP routing, JSON, logging, CLI | Standard net/http, encoding/json, log/slog, flag | Enough for the first API and three command modes; no Gin, Echo, Cobra or Zap by default. |
| SQLite driver/pool | modernc.org/sqlite v1.57.0 with database/sql | CGo-free SQLite; verify FTS5 and connection pragmas; one writer connection and bounded read pool. No ORM. |
| Typed SQL generation | github.com/sqlc-dev/sqlc v1.31.1 (tool) | SQL remains reviewable; generate SQLite/database/sql bindings; CI checks regeneration drift. |
| Migrations | github.com/pressly/goose/v3 v3.27.3 (tool) | Versioned SQL with recorded migration state; exclusive controlled migration step against the SQLite file. Install only the SQLite tool path; do not embed every database driver into the service. |
| Feed parsing | github.com/mmcdole/gofeed v1.4.2 | RSS/Atom parsing; fetch bytes through our controlled transport, then parse. Do not let the parser fetch arbitrary URLs. |
| Bounded concurrency | golang.org/x/sync v0.22.0 | errgroup for cancellation and bounded parallel work. |
| Process-local throttling | golang.org/x/time v0.15.0 | rate for host/source pacing; SQLite transaction reservations remain authoritative for tenant spend and multi-process quotas. |
| First model provider | github.com/anthropics/anthropic-sdk-go v1.68.0 | Official SDK behind ranking.Model; aligns with the first client ecosystem without coupling its runtime. No model call or spend is authorized merely by adding the dependency. |
| Agent transport, Phase 2 | github.com/modelcontextprotocol/go-sdk v1.7.0 | Official MCP implementation as an adapter to the HTTP application's domain services; authorization must match the selected transport/spec. |
| Tests | Standard testing, httptest, fuzzing, race; file-backed SQLite in CI | Exercise real file-backed database, tenant-scoped SQL and concurrency without adding a Docker dependency. |
| Documentation contract checks | Python jsonschema + PyYAML | Development-only; Go service has no Python runtime requirement. |

Go 1.27 and module versions above are verified selections as of this date, not evergreen claims. Before the first runtime PR, verify the resolved transitive graph under the pinned toolchain and run govulncheck; current stable is not a security guarantee. No database container is required. Record the embedded SQLite version and compile options in runtime diagnostics and verify WAL, FTS5, foreign keys and backup behavior on the selected driver. pgx/v5 is deferred until an actual PostgreSQL migration decision.

## Current implementation

The runtime foundation uses only the Go standard library. `go list -m all` contains this module alone; no SQLite/feed/provider modules are installed yet. `make vuln` uses the separately pinned golang.org/x/vuln tool v1.7.0, verified against the official module proxy. The older GitHub release v1.1.4 crashed while analyzing Go 1.27 standard-library syntax; the newer module tag must pass the same full source scan, not a reduced substitute. The tool is not linked into the application.

## Deliberately deferred

No ORM, DI framework, generic repository generator, Elasticsearch client, Redis queue, vector store, browser automation, OpenTelemetry collector, workflow engine or agent framework in Phase 1. Standard Go metrics/logging plus a lightweight health/freshness surface are sufficient to begin; tracing can be instrumented when diagnostic needs justify the SDK and export path. No second model provider until there is an actual requirement.

Use the standard library for random identifiers and key material; choose/verify the exact representation with the data schema. Do not silently introduce UUID/validation/config/router packages through copied North Cloud code.

## Dependency lifecycle

Pin direct modules and development tools; no @latest in CI. Review release notes, minimum Go version, license, maintainership, transitive additions, reachable vulnerabilities and binary-size impact. Prefer the smallest adapter surface. Regenerate sqlc deterministically; never edit generated output to make a test pass. Run Go's modernizers in a reviewable branch, not as hidden formatting steps.

Sources: [Go releases](https://go.dev/doc/devel/release), [modernc SQLite](https://pkg.go.dev/modernc.org/sqlite), [sqlc](https://docs.sqlc.dev/en/stable/), [Goose](https://github.com/pressly/goose), [gofeed](https://github.com/mmcdole/gofeed), [Anthropic Go SDK](https://github.com/anthropics/anthropic-sdk-go), [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk).
