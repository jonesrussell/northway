# Collection and discovery sprint roadmap

Updated 2026-10-01. Owner-directed planning; no production crawling, seeding, database provisioning or auto-publication is authorized by this document. This supplements the launch roadmap with current implementation evidence and supersedes its developer-only initial coverage proposal. NorthCloud is topic-agnostic; useful source coverage precedes speculative ranking services.

## Current truth

Inspected source and deployed API: 9c44b03, schema 12. Production health/readiness passed; anonymous observations returned 401. No new sources, credentials or HTML acquisition were activated by that release.

| Capability | Actual state |
| --- | --- |
| RSS/Atom metadata acquisition | Implemented. Existing feed-only publisher workers use approved sources, conditional HTTP, global attempt/byte budgets and retry/cache holds. No article-page or enclosure download. |
| Source catalogue | Fixed developer-v1 catalogue contains Go and Kubernetes feeds per customer workspace. This is code configuration, not verified live row counts. The attempted read-only database inventory failed to open the database; that route was stopped. |
| Broad feed import | Planned. Pilot CLI admits only its exact reviewed manifest hash; customer provisioning selects a fixed server catalogue. Neither is an arbitrary bulk importer. Underlying transactional provisioning can be reused, with reviewed format and policy changes. |
| HTML collection | Implemented, deployed but inactive: exact admitted HTTPS URLs only, depth zero; source admission defaults disabled/unapproved. DNS-pinned public-only fetch, 15-second deadline, 2 MiB body, bounded OG/JSON-LD metadata and external references. |
| Refresh/frontier | Implemented due-time scheduling, fair source cursor, exclusive claim, expiry/fencing, attempt reservations and cache/backoff holds. Refresh eligibility already exists. No derived-link enqueue or site traversal. |
| HTML worker activation | Planned: no live HTML scheduler/worker CLI or operator approval interface is exposed. collection-api enables management/delivery, not crawling. Robots approval is operator-confirmed with expiry, not automatically retrieved/refreshed. |
| Observations and agents | Implemented tenant-filtered cursor delivery, stable identity/fingerprints, tombstones, scoped expiring grants, revocation and management inventory. No autonomous catalogue expansion or publication authority. |
| FETDER consumer | Locally qualified d6f5b8b; not deployed. Private observations/review/gallery are downstream product work. Production installation qualification remains blocked on prescribed PowerShell gate. |
| Creator publication | No automatic public page creation. Existing registered-member New members ordering stays based on published member media; discovered external creators are a separate unclaimed/reviewed population. |

## Storage decision

Keep the current SQLite deployment for serial feed polling and the bounded single-worker pilot. Owner approved PostgreSQL on 2026-10-01 as the next sprint foundation before continuous discovery and independent crawler workers. Migrate before enabling that concurrency, rather than waiting for a feed-count threshold. Local implementation is authorized; production placement, new database credentials/security settings and cutover require separate specific approval.

The current adapter deliberately owns one database inode with an exclusive advisory lock. It uses WAL, one writer connection, two query-only readers, a context-aware write gate, BEGIN IMMEDIATE, 50 ms busy timeout, a 256 MiB database ceiling and 16 MiB maintenance reserve. Corpus, source policy, attempt accounting, grants and observation state are tenant-owned in that store. This is coherent for the present workload; multiple independent writers would violate the deployment model even if SQLite can support other architectures. PostgreSQL does not implement missing robots, admission, rights or discovery policy.

Use one operational PostgreSQL database for sources, checkpoints, frontier claims, host reservations, retries, grants and observations. No Elasticsearch, broker, separate crawler database or second collection API. SKIP LOCKED can claim competing queue work, but does not provide durable leases, FIFO fairness or stale-completion protection. Preserve explicit lease tokens, short claim transactions, atomic host/byte reservations, idempotent observations and tenant authorization. Commit before network fetch.

Migration costs: PostgreSQL operations and connection memory, WAL/autovacuum/disk budgets, driver and transaction changes, migrations, replacement of SQLite pragmas/FTS5 search semantics, and a expanded populated-schema/concurrency test matrix. Retain storage/business seams rather than building a generic dual-backend framework. No provision or shared GoFormX database reuse is approved. The previous host observation suggested about 1 GiB available memory; it is historical headroom, not placement qualification. A fresh host/whole-service baseline and measured PostgreSQL footprint remain required.

Triggers: selected independent workers or multiple API instances; measured sustained write-gate contention that violates agreed latency; storage reserve pressure despite justified retention; or a concrete availability requirement. Source count alone is insufficient. For a serial first batch, PostgreSQL need not delay catalogue qualification.

Minimal migration gate: preserve IDs, tenants, source URLs, cursor ordering, revision fingerprints, claim/accounting semantics and grants; rehearse export/import on disposable fixtures; compare row counts and representative reads; pause writers and document exact cutover; verify concurrency, crash/restart, stale completion, tenant isolation and backwards-compatible consumer behavior. Define rollback before switching writes; never restart an old dataset and discard new writes. Owner has rejected backup work: this roadmap does not authorize backup jobs, copies or retention changes.

Primary references: [SQLite WAL](https://www.sqlite.org/wal.html), [SQLite suitability](https://www.sqlite.org/whentouse.html), [PostgreSQL queue locking](https://www.postgresql.org/docs/current/sql-select.html), [PostgreSQL resource settings](https://www.postgresql.org/docs/current/runtime-config-resource.html).

## Small, testable sprints

| Sprint | Deliverable | Acceptance and boundary |
| --- | --- | --- |
| 1. Catalogue inventory and broad feed admission | Supported read-only source/policy/budget inventory; reviewed topic-agnostic RSS/Atom register; typed operator import using existing transactional business services. Await research worker feed list. | Exact URL/UUID identity, publisher/category, format, attribution/rights basis, interval, byte cap, review record and enabled state; duplicate import is idempotent, conflicting policy fails visibly. Disabled inventory first. No hash bypass or SQL import. Two-tenant isolation and invalid/duplicate inputs tested. |
| 2. Bounded feed launch | Qualify 2-3 candidates, then at most 10 approved feeds in one selected tenant at daily cadence. | Supported live inventory and remaining budgets obtained first; no automatic replication into every workspace. At 2 MiB maximum, 10 daily checks reserve at most 20 MiB/day before existing work. Stay within 180 attempts/64 MiB global daily caps, one active acquisition, host holds and response limits. Rights/robots where applicable, parse/failure/304 tests and useful-item review pass. Do not enable until register and scope settled. |
| 3. PostgreSQL qualification, conditional on worker decision | Port existing store/frontier transactions and search behavior to one PostgreSQL deployment candidate. | Disposable migration/count/identity checks and concurrent claims/fencing/host reservations pass; measure full host memory, connections, WAL, autovacuum, disk and neighboring-service latency. Explicit placement/cutover decision; no automatic provisioning. Retain SQLite pilot if independent workers are deferred. |
| 4. One HTML source activation | Operator policy approval and one bounded RunOnce/worker path; current robots fetch/parse/expiry and source rights register. | Exact approved URL, budget reservation, DNS/SSRF, redirects, malformed/compressed/oversize content, timeout, retry/cache and stale robots fail closed. One relevant FETDER existing source candidate is link.me/aaronhugese, pending qualification; do not duplicate or silently merge its existing public source record. Private observations only. |
| 5. Continuous refresh and limited discovery | Durable bounded refresh plus candidate suggestions from specifically approved feeds/pages; separate discovery admission from fetch and publication. | Owner selects domains, derivation depth, candidates/day, concurrency and stop controls. Links remain inert until canonicalized, deduplicated and admitted. No login/CAPTCHA/paywall evasion or private collection. Fairness, crashes, retries, stale leases, per-host backoff and budget exhaustion tested. No broad crawl switch. |
| 6. Agent and operator management | Shared business APIs for inventory, seed proposals, status, approval/removal and grant lifecycle; one documented independent client. | Separate read, proposal and operator approval scopes; durable audit without credentials; revoke/expiry every request. Agents cannot promote their own candidate policy or public content. Explicit owner reveal only for newly approved credentials. |
| 7. FETDER acceptance and publication workflow | Qualify/release existing private consumer; source-backed unclaimed creator review, claim/removal and approved media references. | Prescribed installation gate passes in an authorized environment; no backup creation/pruning under current owner instruction. Existing-source collision reviewed, provenance visible, moderation/publication explicit; claim evidence and takedown stop refresh. Member New members ordering unaffected. No broad publication or original media rehosting implied. |

## Selective reuse, not restoration of the old stack

Use docs/migration.md and its audited north-cloud reference 51b877de7dab311c981dcdb4d38dfdca9965aeb1. Feed parsing/conditional-fetch behaviors have already been selectively imported. Review source-manager registry concepts and narrow crawler checkpoint/dedup/robots helpers only when a sprint needs them. Record exact origin paths/commit, ownership/license notices, dependency removal, changes, failure tests and measured resource impact for each selected import. Do not claim uninspected helpers are reusable.

Do not restore per-source indices, Elasticsearch, Redis routing, ML sidecars, legacy Compose topology, unbounded browser rendering or source-manager as another authoritative service. The old product is archived and its server gone; no active legacy deployment migration is required.

## Decisions remaining

PostgreSQL sequencing is approved. Remaining decisions: placement/resource limits; research register and initial tenant; source rights and attribution; HTML domains/discovery bounds; original-media versus links/embed/thumbnail policy; moderation/publication owner; claim/removal evidence; observation retention and operational latency goals. Thumbnail caching and original downloads each need their own source-specific display/license and storage decision. Live video/stream state needs provider-specific official API/embed/feed qualification, quota/cost and freshness semantics, not HTML guesses.

The approved FETDER observations-read grant (one hour, exact tenant) remains unissued until the backend is ready to use it. This plan grants no new credentials, acquisition scope, billing, broad crawling, live feeds or publication.
