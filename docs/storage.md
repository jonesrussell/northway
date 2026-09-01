# SQLite storage foundation

Issue #10 implements a tenant-scoped corpus store, not the authenticated feed service. The Linux/ARM64 target uses modernc SQLite, Goose forward migrations, and sqlc bindings. There is no acquisition, API-key provisioning endpoint, provider call, ranking, snapshot, or deployment in this slice.

## Run

```sh
make build
./bin/northway migrate --database ./data/northway.sqlite
./bin/northway serve --database ./data/northway.sqlite
# Stop serve first; prepare a private directory and use a new destination.
mkdir -m 0700 -p ./backups
./bin/northway backup --database ./data/northway.sqlite --output ./backups/northway.sqlite
```

NORTHWAY_DATABASE_PATH is the environment alternative; flags override it. Migrate creates a missing private directory/file (0700/0600), applies embedded migrations under a two-minute command deadline, and checks SQLite, foreign-key and FTS integrity. Existing directories/files must already be private; the application does not silently chmod operator-owned paths. Database and immediate parent symlinks are rejected. Keep ancestor directories under operator control on local storage. Network filesystems and untrusted local users are outside this design.

Serve requires an existing, migrated database and never migrates implicitly. The process holds a nonblocking advisory flock on the database inode; other Northway serve/migrate processes fail until it exits. This is an additional safeguard, not protection against an external SQLite client or someone with file access. Stop the service before migration through waaseyaa-infra. Migrations are forward-only; take a coherent backup before upgrade and use expand/migrate/contract for destructive changes. No automatic down/rollback command is provided. The backup command takes the same exclusive ownership lock, so it refuses while serve or migrate owns the source. It validates the source, uses SQLite `VACUUM INTO` to include committed WAL state, validates the copy, fsyncs it and atomically publishes a mode-0600 file without replacing an existing destination. The output directory must already be private (0700). The CLI applies a fixed two-minute deadline to the complete validation and copy operation; expiry returns `context deadline exceeded` and publishes nothing. Supported older schemas are accepted so the new binary can take a pre-upgrade snapshot; a schema newer than the binary is refused explicitly. Failed or canceled work publishes no snapshot. Because the source is opened read-write, closing the offline backup connection can checkpoint and remove its WAL after the durable snapshot is published; the source directory must therefore remain writable. A SIGKILL or power loss can leave an unpublished `.northway-backup-*` staging file for the operator to inspect and remove.

With no database configured, the health-only runtime still starts and readiness is 503. With storage configured, startup checks the file, schema, WAL/FTS5 capability and pragmas; readiness is 200 only while storage checks succeed. Every writer connection, including a replacement after cancellation, applies a 256 MiB `max_page_count` ceiling and a 256-page WAL autocheckpoint target through the connection DSN. Ordinary writes stop with 16 MiB of reusable/extendable page headroom so maintenance can still delete and checkpoint; maintenance alone may consume that reserve. These are runtime admission controls, not a filesystem quota: WAL, snapshots and other services still need host headroom.

To verify a restore, copy a snapshot rather than consuming the retained backup, keep the destination directory/file private, then run migration before serve. `VACUUM INTO` emits a coherent database that may use delete journal mode; migration deliberately restores the required WAL mode and verifies the current schema:

```sh
mkdir -m 0700 -p ./restore
cp --no-clobber ./backups/northway.sqlite ./restore/northway.sqlite
chmod 0600 ./restore/northway.sqlite
./bin/northway migrate --database ./restore/northway.sqlite
./bin/northway serve --database ./restore/northway.sqlite
```

## Connection and transaction contract

- One writer connection, two query-only read connections. All connections, including replacements, get foreign_keys=ON, busy_timeout=50ms, synchronous=FULL and a 2 MiB page-cache target through driver DSN options. The writer gets the 256 MiB page ceiling and a 256-page autocheckpoint target on every open. Readiness uses only the reader pool and does not fail merely because maintenance holds the writer. WAL persists in the database.
- A context-aware write gate serializes writers. Transactions use BEGIN IMMEDIATE before reads or changes. There is no application lock-retry loop: lock errors propagate after SQLite's bounded busy wait. Cancellation during a busy wait may take up to that short wait plus scheduling overhead.
- Store methods expose feature types, not raw handles, sqlc rows or caller-defined transaction callbacks. Close follows HTTP request draining. SQL errors roll back article, version and FTS changes together.
- Read requests can cancel while waiting for the bounded pool. Hourly pilot maintenance prunes eligible tenant rows in 1,000-row batches, then requests a truncate checkpoint; a busy checkpoint emits an operator warning and a later hourly run retries it without halting publisher collection. Long readers can still delay truncation. Do not copy a live database without its WAL. The portable offline backup primitive is implemented, but recoverability requires copying a coherent snapshot to a separate path/device, migrating it and exercising real tenant queries.

## Initial schema and scope

Migration 1 creates STRICT tenants, sources, articles, article_versions, feeds and feed_sources tables. IDs are canonical lowercase UUID TEXT, timestamps are UTC integer microseconds, and unknown publication time stays NULL. The storage adapter accepts article timestamps and search lower bounds only from the Unix epoch through the end of UTC year 9999. Stable source item identity is unique within (tenant, source, origin ID). Updates cannot silently move an article to another source/origin. Title/body hashes identify immutable content versions; the current row and version insertion commit together. Pilot maintenance removes articles older than 90 days and superseded versions past that cutoff while preserving each current version.

Every corpus source is tenant-owned in this first schema, including sources whose publisher is public. There is no cross-tenant shared corpus or entitlement bypass. Composite foreign keys enforce tenant membership for article/source, feed/source and article/version relations. All reads, updates, deletes and FTS candidate queries require explicit tenant scope. Corpus methods require a Principal and derive tenant scope from it; authenticated service reads require feeds:read and mutations require a tenant-bound local operator. See [identity](identity.md). A raw TenantID is not authentication. CreateTenant is an internal operator provisioning seam, not an HTTP endpoint. Repeating the same tenant creation succeeds without changing the existing row, preserving its creation time and corpus/entitlement revisions. Source metadata is storage only; neither its presence nor HTTPS syntax approves fetching or redistribution (#9/#12).

Migration 2 adds external-content FTS5 and insert/update/delete triggers, then rebuilds the index from any existing rows. Migration 3 adds scoped key storage. Migration 4 adds query claims/snapshots, spending holds and revision/access controls; migration 5 adds disabled-by-default collection policies, fenced poll attempts and cursors; migration 6 adds saved retrieval policy and rich snapshots; migration 7 adds feedback events; migration 8 adds retention-path indexes. Populated upgrades from every earlier schema are tested. Future migrations must extend this matrix.

The storage Search method joins both tenant and feed-source membership, applies an observation-time lower bound, orders by observation time and article ID, and caps results at 50. It accepts 1–8 literal alphanumeric terms (256 bytes total, 64 per term); punctuation is discarded and quoted terms are combined with AND. SQL parameterization and MATCH grammar bounds are separate. Contextual relevance, exclusions, rights policies, pagination and the public query contract belong to #13 and later work; this method is a bounded corpus primitive only.

## Verification and tooling

```sh
make generate          # reviewed regeneration with sqlc 1.31.1
make generate-check    # regenerate in a temporary directory, compare all Go bindings
make integration       # real file-backed SQLite behavior/failure tests
make check race vuln
make licenses          # notices for actually linked application modules
```

The sqlc harness downloads the official Linux amd64/arm64 release with a pinned SHA-256, caches it under ignored bin/tools, and verifies its version. SQL stays in db/queries; generated bindings stay inside internal/sqlite/sqlc. FTS5 MATCH SQL is explicitly handwritten in the SQLite adapter and tested against the real engine; sqlc's relational schema inputs are migrations 1 and 3 through 7; migration 2 contains the handwritten FTS objects. New relational migrations must be added to that input with the schema change. db is an embedded migration-assets package and cannot import SQL or expose storage handles.

Tests demonstrate missing/wrong tenant rejection, composite constraints, FTS and version atomicity, timestamp round trips, read-only/replaced connections, serial writers, external lock deadlines, migration rollback, newer-schema rejection, process restart, tenant-scoped retention and persisted freshness/lease health. Disk-full coverage uses SQLite's real max_page_count limit to force SQLITE_FULL and checks rollback/recovery; actual ARM64 constrained execution and separate-device restore evidence are recorded through #20 rather than inferred from unit tests.

Migration and serve reject a database already above 256 MiB rather than applying a false ceiling. For such a file, keep the old known-good binary stopped, make a coherent backup, remove or export eligible data with a reviewed operator procedure, compact the copy and rehearse the upgrade there before replacing the live file. The personal pilot database was measured far below this threshold before the ceiling was introduced.

## Container contract

The image includes /data/northway owned by UID/GID 65532 with mode 0700; it declares no implicit VOLUME. Mount durable storage at /data and use --database=/data/northway/northway.sqlite. The private child directory avoids relying on Docker preserving a volume root's permissions. For a host bind mount, infra must prepare that child with the same owner/mode. Retain a read-only root filesystem and run migrate as the same UID against the volume before serve. No production compose/routing/secrets are introduced here.

make container-smoke first verifies health-only behavior, then creates a uniquely named disposable Docker volume, migrates it non-root, runs the executable readiness probe, restarts the same container, creates an offline coherent snapshot and runs the migration command against that snapshot. It removes only its own test resources. The 128 MiB smoke limit is an empty-corpus test condition, not a measured production capacity budget. Native ARM/Pi runtime testing and an off-device backup/restore drill remain required before deployment.

## Dependency refinement

Use Goose v3.27.3's provider library inside the migration command rather than importing its multi-driver CLI. Only modernc SQLite is registered; no dotenv loader or other database driver is linked. This refines the earlier tool-only selection to preserve one executable and embedded immutable migrations. Inspect the actual linked package graph and THIRD_PARTY_NOTICES.txt; a module's go.mod includes tool/test dependencies that are not necessarily linked into Northway. sqlc remains development-only. Go's license and generated third-party notices are included in the container.

Sources: [modernc connector/DSN contract](https://pkg.go.dev/modernc.org/sqlite), [Goose provider](https://pressly.github.io/goose/documentation/provider/), [SQLite FTS5](https://www.sqlite.org/fts5.html), [SQLite WAL](https://www.sqlite.org/wal.html).

Migration 6 adds saved selection preferences and rich snapshot details. See [deterministic retrieval](retrieval.md) for the implemented service, bounds, response projection and legacy snapshot handling.
