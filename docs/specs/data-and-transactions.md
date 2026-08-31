# Data, transactions and background work

Applies to NW-002, NW-003, NW-007, NW-009 and NW-010. Status: SQLite corpus schema, migrations and connection lifecycle are implemented in #10; ingestion/query/feedback transactions and backup/retention below remain specifications for later slices. See [storage implementation](../storage.md).

## SQLite invariants

Use one file-backed SQLite database on local storage, WAL journal mode, foreign keys enabled on every connection and a bounded busy timeout. Verify these settings and FTS5 support at startup and in integration tests, including newly opened/replaced connections. Start with a single write connection and a small bounded read pool. Use short write transactions with a deliberate immediate-transaction strategy for read/modify/write reservations; do not rely on deferred upgrades that can fail after reading. Cap lock retries and honor cancellation. No network calls inside transactions.

SQLite provides no server roles or row-level security. Restrict file/directory access to the service/operator, but recognize that anyone with database-file access can read every tenant. Every private query requires explicit authenticated tenant scope, including FTS joins, writes, deletions, batch jobs and caches. Omitted tenant scope must fail closed. Composite tenant/object uniqueness and foreign keys prevent accidental cross-tenant attachments. Authorization lookup is a dedicated narrow path; feature code cannot access a raw database handle.

Use opaque UUID strings consistently in JSON, Go and SQL. Store IDs as validated TEXT and timestamps as UTC integer microseconds, converting explicitly to RFC3339 at the API boundary. Prefer STRICT tables and CHECK constraints for enums, booleans and nonnegative counters. Never use floating point for quota/cost accounting. Define unique source/item identity and version hashes independently of presentation titles.

Store API keys as a nonsecret lookup prefix and a cryptographic digest of high-entropy secret material; constant-time comparison, revocation and last-used metadata. Never record raw keys. Exact generation/storage design is reviewed in the identity issue.

## Transaction boundaries

**Ingestion:** claim an eligible poll with an expiring lease and fencing version; fetch/parse outside the transaction; atomically upsert item/version/observation and successful poll state only if lease ownership still matches. A crash must not mark unprocessed content successful. A 304 updates successful-check timestamps without inventing an article version. Conditional headers advance only after processing commits. One concurrent fetch initially; capped backoff and persisted failure state prevent restart storms.

**Query:** authenticate and authorize feed; validate bounded input; atomically claim (tenant, idempotency key), canonical request hash and worst-case budget reservation. Commit before retrieval/provider I/O. Concurrent repeats return the existing snapshot or a retryable in-progress response. Same key with a different payload conflicts. Persist the validated snapshot and reconcile usage in a short final transaction. Expired reservations require reconciliation: a provider timeout does not prove no charge occurred. Ambiguous provider completion must not trigger blind duplicate inference.

**Feedback:** validate tenant ownership and snapshot membership; append an idempotent event keyed by (tenant, client event ID). A different payload with the same key conflicts. Reversal is a new event referencing an unreversed event belonging to that tenant and item. Update preference revision atomically, invalidating personalized cache keys. An undo cannot reverse another undo.

**Retention:** delete eligible expired rows in bounded batches; honor pending reservations, saved evidence and source rights. Keep minimal tombstones for defined replay/suppression windows. Bound disk usage independently of age. Backup expiry and customer deletion policy must agree; do not promise immediate erasure from retained backups.

## Search and portability

Use sqlc with SQLite/database/sql bindings; storage adapters map generated rows to feature types. FTS5 retrieves candidates with explicit source/feed membership, entitlement, age and exclusion predicates. Parameterize SQL and separately normalize/escape FTS syntax; SQL parameters alone do not constrain MATCH operators. Cap candidates and query complexity. Maintain FTS content atomically with article changes and test deletion/rebuild consistency.

Use a stable tie-breaker such as (observed_at, article_id) for keyset pagination, never just a timestamp. Immutable snapshots retain their recorded ordering instead of reranking on read.

Keep SQLite SQL behind the storage adapter without a speculative generic database abstraction. PostgreSQL migration is a future project: rewrite dialect-specific queries/FTS/migrations and transaction behavior, backfill data, verify counts and tenant constraints, compare retrieval results, rehearse cutover and rollback. It is not a driver swap. Consider it when measured write contention, multi-host operation, DB-enforced tenant policies or recovery/availability needs exceed this design.

## Recovery and schema changes

Goose SQL migrations run exclusively before serving requests; deployment prevents concurrent app/migration processes. Use monotonically ordered IDs and test fresh install and upgrades from every supported release. Generated sqlc output is committed and regeneration must produce no diff. Destructive changes use expand/migrate/contract and a tested backup, not an assumed lossless down migration.

Monitor WAL growth and checkpoint progress; long readers can delay checkpoints. Begin with durable synchronous settings and document any later durability/performance tradeoff. Do not copy only the live .db file while ignoring its WAL. Use the SQLite online backup API or a documented coherent snapshot procedure, verify integrity and foreign keys, then restore onto a separate path/device and test queries. Keep backups off the Pi as well as local recovery material. Validate disk-full, process interruption and power-loss recovery expectations on the actual storage.

References: [SQLite WAL](https://www.sqlite.org/wal.html), [foreign keys](https://www.sqlite.org/foreignkeys.html), [online backup](https://www.sqlite.org/backup.html), [FTS5](https://www.sqlite.org/fts5.html).
