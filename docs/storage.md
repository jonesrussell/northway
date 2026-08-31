# SQLite storage foundation

Issue #10 implements a tenant-scoped corpus store, not the authenticated feed service. The Linux/ARM64 target uses modernc SQLite, Goose forward migrations, and sqlc bindings. There is no acquisition, API-key provisioning endpoint, provider call, ranking, snapshot, or deployment in this slice.

## Run

```sh
make build
./bin/northway migrate --database ./data/northway.sqlite
./bin/northway serve --database ./data/northway.sqlite
```

NORTHWAY_DATABASE_PATH is the environment alternative; flags override it. Migrate creates a missing private directory/file (0700/0600), applies embedded migrations under a two-minute command deadline, and checks SQLite, foreign-key and FTS integrity. Existing directories/files must already be private; the application does not silently chmod operator-owned paths. Database and immediate parent symlinks are rejected. Keep ancestor directories under operator control on local storage. Network filesystems and untrusted local users are outside this design.

Serve requires an existing, migrated database and never migrates implicitly. The process holds a nonblocking advisory flock on the database inode; other Northway serve/migrate processes fail until it exits. This is an additional safeguard, not protection against an external SQLite client or someone with file access. Stop the service before migration through waaseyaa-infra. Migrations are forward-only; take a coherent backup before upgrade and use expand/migrate/contract for destructive changes. No automatic down/rollback command is provided.

With no database configured, the health-only runtime still starts and readiness is 503. With storage configured, startup checks the file, schema, WAL/FTS5 capability and pragmas; readiness is 200 only while storage checks succeed. This means **storage readiness**, not a working contextual feed. Dependency errors remain redacted from HTTP responses. Schema versions newer than the binary fail closed. SQLite version and compile options are logged at startup without content or private query data.

## Connection and transaction contract

- One writer connection, two query-only read connections. All connections, including replacements, get foreign_keys=ON, busy_timeout=50ms, synchronous=FULL and a 2 MiB page-cache target through driver DSN options. WAL persists in the database.
- A context-aware write gate serializes writers. Transactions use BEGIN IMMEDIATE before reads or changes. There is no application lock-retry loop: lock errors propagate after SQLite's bounded busy wait. Cancellation during a busy wait may take up to that short wait plus scheduling overhead.
- Store methods expose feature types, not raw handles, sqlc rows or caller-defined transaction callbacks. Close follows HTTP request draining. SQL errors roll back article, version and FTS changes together.
- Read requests can cancel while waiting for the bounded pool. Long readers, WAL growth, checkpoint monitoring, retention and coherent off-device backups still require the operations work package; do not copy a live database without its WAL.

## Initial schema and scope

Migration 1 creates STRICT tenants, sources, articles, article_versions, feeds and feed_sources tables. IDs are canonical lowercase UUID TEXT, timestamps are UTC integer microseconds, and unknown publication time stays NULL. The storage adapter accepts article timestamps and search lower bounds only from the Unix epoch through the end of UTC year 9999, checking the UTC instant before integer conversion to prevent overflow and preserve RFC3339 compatibility. Sub-microsecond precision is discarded. Stable source item identity is unique within (tenant, source, origin ID). Updates cannot silently move an article to another source/origin. Title/body hashes identify immutable content versions; the current row and version insertion commit together. Version retention policy is future work.

Every corpus source is tenant-owned in this first schema, including sources whose publisher is public. There is no cross-tenant shared corpus or entitlement bypass. Composite foreign keys enforce tenant membership for article/source, feed/source and article/version relations. All reads, updates, deletes and FTS candidate queries require explicit tenant scope. Corpus methods require a Principal and derive tenant scope from it; authenticated service reads require feeds:read and mutations require a tenant-bound local operator. See [identity](identity.md). A raw TenantID is not authentication. CreateTenant is an internal operator provisioning seam, not an HTTP endpoint. Source metadata is storage only; neither its presence nor HTTPS syntax approves fetching or redistribution (#9/#12).

Migration 2 adds external-content FTS5 and insert/update/delete triggers, then rebuilds the index from any existing rows. Tests upgrade populated migration-1 files, rerun migration idempotently, verify deletion/integrity and reopen the file. Migration 3 adds scoped key storage. Migration 4 adds query claims/snapshots, spending holds and revision/access controls; see [query transactions](query-transactions.md). Populated schema-1, schema-2 and schema-3 upgrades are tested. Future migrations must extend this upgrade matrix.

The storage Search method joins both tenant and feed-source membership, applies an observation-time lower bound, orders by observation time and article ID, and caps results at 50. It accepts 1–8 literal alphanumeric terms (256 bytes total, 64 per term); punctuation is discarded and quoted terms are combined with AND. SQL parameterization and MATCH grammar bounds are separate. Contextual relevance, exclusions, rights policies, pagination and the public query contract belong to #13 and later work; this method is a bounded corpus primitive only.

## Verification and tooling

```sh
make generate          # reviewed regeneration with sqlc 1.31.1
make generate-check    # regenerate in a temporary directory, compare all Go bindings
make integration       # real file-backed SQLite behavior/failure tests
make check race vuln
make licenses          # notices for actually linked application modules
```

The sqlc harness downloads the official Linux amd64/arm64 release with a pinned SHA-256, caches it under ignored bin/tools, and verifies its version. SQL stays in db/queries; generated bindings stay inside internal/sqlite/sqlc. FTS5 MATCH SQL is explicitly handwritten in the SQLite adapter and tested against the real engine; sqlc's relational schema inputs are migrations 1, 3 and 4; migration 2 contains the handwritten FTS objects. New relational migrations must be added to that input with the schema change. db is an embedded migration-assets package and cannot import SQL or expose storage handles.

Tests demonstrate missing/wrong tenant rejection, composite constraints, FTS and version atomicity, timestamp round trips, read-only/replaced connections, serial writers, external lock deadlines, migration rollback, newer-schema rejection and process restart. Disk-full coverage uses SQLite's real max_page_count limit to force SQLITE_FULL and checks rollback/recovery; it does not fill the host disk or establish filesystem/power-loss behavior on the Pi. Those actual-device gates remain in #20.

## Container contract

The image includes /data/northway owned by UID/GID 65532 with mode 0700; it declares no implicit VOLUME. Mount durable storage at /data and use --database=/data/northway/northway.sqlite. The private child directory avoids relying on Docker preserving a volume root's permissions. For a host bind mount, infra must prepare that child with the same owner/mode. Retain a read-only root filesystem and run migrate as the same UID against the volume before serve. No production compose/routing/secrets are introduced here.

make container-smoke first verifies health-only behavior, then creates a uniquely named disposable Docker volume, migrates it non-root, runs storage-backed readiness and restarts the same container. It removes only its own test resources. The 128 MiB smoke limit is an empty-corpus test condition, not a measured production capacity budget. Native ARM/Pi runtime testing and coherent backup/restore remain required before deployment.

## Dependency refinement

Use Goose v3.27.3's provider library inside the migration command rather than importing its multi-driver CLI. Only modernc SQLite is registered; no dotenv loader or other database driver is linked. This refines the earlier tool-only selection to preserve one executable and embedded immutable migrations. Inspect the actual linked package graph and THIRD_PARTY_NOTICES.txt; a module's go.mod includes tool/test dependencies that are not necessarily linked into Northway. sqlc remains development-only. Go's license and generated third-party notices are included in the container.

Sources: [modernc connector/DSN contract](https://pkg.go.dev/modernc.org/sqlite), [Goose provider](https://pressly.github.io/goose/documentation/provider/), [SQLite FTS5](https://www.sqlite.org/fts5.html), [SQLite WAL](https://www.sqlite.org/wal.html).
