# Local PostgreSQL and shared public acquisition

This candidate is local implementation against runtime base
`9c44b03ce8d48a4de5f5815705f95b764b869116`. It does not include the later
tenant-duplicating curated importer or authorize production provisioning,
credentials, migration, deployment or ingestion. Existing concurrent checkouts
and production releases are outside this checkout's write scope.

## Storage and ownership

SQLite schema 12 remains supported. Explicit `postgres:/absolute/private/file`
selects PostgreSQL schema 13; the file contains a DSN, must be a regular 0600
file in a private directory, and must never be put into flags, logs or source.
The usual explicit `migrate --database postgres:...` precedes serving. Opening
storage neither migrates nor starts ingestion. Native PostgreSQL title search
uses bounded literal queries, the `northway_search` configuration and GIN
indexes. The adapter translates only repository-owned generated SQL; values
remain bound parameters. `make generate-check` still checks the original sqlc
contract. This narrow adapter needs requalification whenever generated queries
change; it is not a generic database abstraction.

One writer pool (four connections, including its ownership session) and two
reader connections are allowed per Store. Short writes share one transaction
advisory lock across independent processes. Migration/import/customer support
require exclusive lifecycle ownership; serving holds shared lifecycle ownership.
Lost ownership connections fail closed on writes. Read transactions use repeatable
read. Claims use row locks with SKIP LOCKED and persistent attempt IDs and leases;
network work starts after the reservation transaction commits. Acquisition is
still **one active fetch globally**, shared between public and private work.
Parallel callers are qualified for exclusive admission, not four active fetches.

Public sources, checkpoints, attempts, metadata articles and versions have
explicit tables without tenant IDs. Workspace subscriptions have tenant/feed
composite ownership and require an active customer workspace, enabled feed and
approved/enabled public metadata policy. Existing private sources/articles and
HTML collection observations remain private. No source/article is copied for a
subscription and no fake tenant is created. Deterministic UUID identities use
separate public source, article, publisher-group and saved-feed namespaces.
Canonical public source URLs normalize host case, a trailing hostname dot,
default HTTPS port and an empty path. Database guards reject public/private ID
collisions rather than reinterpreting private records.

Onboarding commits account creation, per-original-topic private saved feeds,
preferences and public memberships together. Replays preserve saved preferences.
The original twenty reviewed topics/languages remain in source metadata; bounded
feed preferences additionally accept these topics, with five categories maximum
per saved digest. `shared-v1` selects the PostgreSQL customer catalogue. It does
not start CustomerPublisher or a new public poll loop. The explicit local
`RunPublicPollOnce` seam reuses the existing guarded native Fetcher contract and
has no HTTP management route, startup hook or production schedule. Operator
methods must never be called with request/model-supplied policy.

## Register and policy

The immutable reviewed broad and canary JSON inputs retain exact SHA256 values:

* Broad: `5e14f54cf9f0376689aa0b79c4bc41466bbbd25b1a138dfe0018e7a98d80a4c5`.
* Canary: `ea92fb09ec7feba7847b2dbdfe590a10ea354294f143bb3fcf8284bfdf0797ea`.

`InstallReviewedPublicRegister` atomically installs 88 disabled, pending,
not_assessed definitions and marks the ten canary selections for possible future
onboarding. It disregards historical importer activation flags. Installation is
idempotent and does not overwrite separately reviewed policy. Eight explicitly
restricted/unqualified register entries persist as exclusions and cannot be
configured as public sources. Remaining unselected entries are not installed.
Metadata qualification does not establish display rights. Enabling a source
requires an explicit local operator decision with approved review state and
reviewed_metadata rights basis. Synthetic tests supply these decisions solely
for local fixtures. No live canary fetch or activation is performed here.

Public/private acquisition shares 100 configured jobs, 180 rolling attempts/day
and 64 MiB/day. Admission reserves response bytes atomically. Failed/crashed or
policy-fenced work remains charged conservatively; success releases unused bytes
but not its attempt. Host holds span both classes and preserve Retry-After and
cache deadlines. A tenant-local ResetPollSchedule cannot shorten a shared host
hold. Public definition replay does not fence a lease; actual policy/configuration
changes do. Scheduling uses oldest public attempt then stable source identity.
Public metadata retention is bounded by the existing 5,000-item/10,000-version
per-source ceilings. Admission stops at capacity. Disablement retains metadata
and version evidence, stops new acquisition and removes visibility. No new
takedown deletion/retention job is introduced without a reviewed policy.

## Authorization and invalidation

Read-only membership projections combine authorized shared metadata with the
requesting tenant's private corpus. They do not expose private observations,
context, credentials, feedback or another tenant's snapshots. Public corpus and
policy revisions are recorded in public_query_scopes for work/snapshots. Shared
changes atomically advance subscribed feed revisions, fencing pending completion
and cache reuse alongside private corpus and entitlement revisions. Changed
shared corpus/policy makes an old shared snapshot unavailable; unsubscribe
suppresses its formerly accessible items. Private legacy snapshots retain their
existing replay behavior. Feedback remains tenant-private and snapshot-bound.

API keys are rechecked in the same transaction as admission, retrieval,
completion, snapshot reads and feedback. Revocation advances entitlement
revision; expiry/revocation cannot use an old principal to write a cache result.
Workspace suspension removes its public memberships and blocks access.
Deletion removes private state/memberships while preserving public ownership and
other workspaces. Operator-owned publisher policy changes fence stale workers.
No collection grant receives additional scopes.

## Paused-writer cutover and forward repair plan

These are future release gates, not instructions to execute on production.

1. Review the concrete schema-13 image, register hashes, PostgreSQL placement,
   connection/TLS custody, capacity and neighbor-service measurements. Obtain
   specific provisioning/cutover/ingestion approval separately. Preserve the
   current homepage and source-rights exclusions. Keep ingestion disabled.
2. Drain and pause all source and target writers, polls, API writes and grant
   issuers under the established deployment ownership procedure. Confirm no
   competing application owner. Do not bypass an ownership or access refusal.
3. Explicitly migrate an empty target. ImportPostgres exclusively opens the
   paused SQLite source and refuses any nonempty private/public target. It
   copies trusted schema-12 tables transactionally with foreign keys enforced,
   preserves revisions and IDs, resets identity sequences, and verifies row
   counts. It does not promote legacy tenant feed data into public ownership.
   Install shared configuration only after the private import is accepted.
4. Compare populated counts, article/version identities, private observation
   IDs/cursor/tombstones, grants, representative API projections and isolation.
   Requalify search, budgets, restart/lease fencing, revision races and bounded
   resource use on the actual proposed host. Keep the original SQLite data
   intact and quiescent. No backup job, snapshot, offhost copy, pruning or deletion
   of existing backup artifacts is part of this plan.
5. Before any target write, routing can return to the intact paused SQLite
   source with its original binary after explicit rollback approval. Once target
   writes occur, PostgreSQL is authoritative: pause writers, preserve those
   records and use reviewed additive forward repair. Never overwrite the target
   from stale SQLite or discard post-cutover queries, feedback, grants,
   observations, checkpoints or charges. A lossless reverse exporter is not
   implemented; returning to SQLite after target writes requires separate
   implementation, reconciliation and approval.
6. Verify readiness and exact release identity before routing traffic. Public
   canary activation remains a separate approved action, daily or slower; expand
   to 88 only after bounded real-feed and policy acceptance. Local synthetic
   qualification is not a production canary receipt.

## Local review and limits

The implementing agent reviewed ownership joins, binding boundaries, lease and
reservation transactions, namespace/import behavior, revocation races and
startup composition. This is a recorded local review, not independent production
release sign-off. Existing repository checks, PostgreSQL acceptance tests, race
checks, vulnerability and notice checks must be recorded at the committed head.

The application schema cap checks database size before bounded admission, with
16 MiB reserved from the 256 MiB envelope. WAL/server disk limits remain server
configuration and were constrained only in the disposable local container. No
full-disk production stress or neighboring production-service measurement is
claimed. Shared revision fan-out intentionally invalidates conservatively across
subscribed feeds. A dedicated generic storage framework, Elasticsearch, broker,
new microservice, backup jobs and production rollout are absent.

## Complete-catalogue job, 2026-10-02

[NC-SHARED-001](specs/NC-SHARED-001.md) supersedes the staged ten-source delivery plan. Existing production PostgreSQL/shared polling is retained. The full job uses `postgres activate-catalogue --database postgres:/private/connection --approval-record <record>` to validate and activate all 88 immutable reviewed definitions atomically. Prior exact-register approval provenance is preserved; denied/conflicting sources refuse the whole action. All become onboarding-eligible, while existing saved preferences remain unchanged.

Read-only `postgres catalogue-status --database postgres:/private/connection` reports every public source's URL, topic, policy state, last attempt/success/status/error and article count, without article text or tenant data. Use its complete reconciliation for release acceptance. Daily scheduling and combined 100-source/180-attempt/64-MiB limits remain fixed. This implementation record does not authorize production activation; complete qualification and obtain approval for the exact release package first.
