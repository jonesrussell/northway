# Phased roadmap

Phases are acceptance gates, not delivery-date promises. Ship one useful workflow before expanding the product. All phases after Phase 0 are unimplemented.

## Phase 0 — Foundation (this repository)

Outcome: agree on a product boundary and a small implementation target.

- Independent private repository, architecture decision, migration ledger, draft feed-query contract, synthetic examples, and contract checks.
- First client: Claudriel. First scenario: contextual PHP news in one workspace.
- Initial deployment: Raspberry Pi, one Go process, embedded SQLite/WAL/FTS5. Confirm model, RAM, OS architecture and storage before setting performance budgets. The earlier 1 GiB hypothesis is not a measured requirement; remote model-provider cost is separate.

Exit: roadmap and contracts are reviewable without starting or migrating the old stack. No running service is claimed.

## Phase 1 — Useful personal pilot (v0.1)

Outcome: a working contextual feed inside Claudriel, backed by Northway.

1. Create the Go application, configuration, health/readiness endpoints, migrations, and local startup with embedded SQLite, enforced connection pragmas, serialized writes and a verified FTS5 build.
2. Manually provision one tenant, a revocable hashed API key, and a feed containing 5–10 explicitly selected RSS/Atom sources. Do not ship signup or arbitrary URL ingestion yet.
3. Port feed fetching with conditional requests, parsing, stable item identity, deduplication, bounded retries, and persisted poll state. Use available feed text only; prohibit home crawling and article-page fetching. Start with hourly staggered polling and one concurrent fetch, subject to publisher limits. See [content sourcing](content-sources.md).
4. Store articles, source observations, feed definitions, query snapshots, feedback, and usage records. Every tenant-owned row and lookup is scoped from authenticated identity.
5. Implement POST /v1/feed-queries: deterministic retrieval, bounded AI reranking and explanations, validated output, and deterministic fallback. Return evidence, publication/observation times, and coverage/staleness signals.
6. Add snapshot retrieval and idempotent feedback. Feedback is explicit, scoped, reversible, and must not become cross-customer training data by default.
7. Integrate through Claudriel's PHP backend and agent-tool contract. Add one panel with at most five items, source links, explanations, and save/dismiss controls. News is separate from the existing North Cloud leads API.
8. Add request budgets, rate limits, bounded logs, failure reporting, retention, and restore-tested backups. Limit AI to one ranking call per cache miss initially; repeated panel renders must not trigger repeated calls.
9. Optionally enable one scheduled custom list using an external web-search provider, durable schedule state and hard spending caps. Separate acquisition from digest ranking, serve the last snapshot without refreshing on panel render, and preserve feed-only operation. See [scheduled discovery](specs/scheduled-discovery.md). No crawling runs at home.

**Exit gates:** user finds the feed useful over two weeks; at least 80% of a manually labelled top-five sample is relevant; review excluded examples for misses; no invented sources or unsupported applicability claims; source failures are visible; ingestion and query retries are safe; two test tenants cannot see one another's feeds, snapshots, feedback, credentials or caches. Demonstrate restart recovery and resource headroom under the actual pilot workload. See [evaluation](evaluation.md).

**Explicitly excluded:** self-service onboarding, billing, generalized web search, multi-region availability, custom source plugins, a public dashboard, all old North Cloud classification domains, and bulk historical migration.

## Phase 2 — Agent developer preview (v0.2)

Outcome: a second independent agent client can use Northway without Claudriel.

- Stabilize versioned HTTP/OpenAPI contracts, error semantics, pagination, and a tiny client example.
- Add a thin MCP adapter for query_feed, get_snapshot, and submit_feedback over the same application services. A read tool may consume bounded compute; feedback is a separately scoped write tool.
- Implement the authorization required by the selected MCP transport and current specification; do not assume an API key alone provides delegated OAuth access.
- Introduce authenticated feed CRUD, source selection from an approved catalogue, preference revisions and explicit source muting. Scope immutable feed revisions in cache keys.
- Test genuinely distinct tenants and clients, key rotation, quota races, expired snapshots, prompt injection, and usage accounting.
- Measure first-result latency, cache hit rate, freshness, useful items per request, provider failures, cost per uncached query, and operator burden.

**Exit gates:** two clients work from published documentation; contract tests run against the server; an external design partner can be onboarded manually; cross-tenant negative tests pass through HTTP, MCP and background jobs. No self-service public signup yet.

## Phase 3 — Private paid SaaS beta (v0.3)

Outcome: operate safely for paying design partners.

- Tenant and member lifecycle, workspace-scoped permissions, signup/invites as appropriate, key management and usage visibility.
- Meter successful queries, provider tokens/cost, ingestion work and storage separately. Establish a pricing model from measured costs; avoid unlimited AI or crawl plans.
- Billing integration with idempotent events, spend ceilings and enforceable quotas. Provider timeout/retry/cache-hit accounting must be explicit before charging.
- Source licensing and redistribution review, attribution controls, customer data export/deletion, declared model-provider data handling and retention.
- Operational runbooks, capacity tests, backup recovery exercises, abuse handling and support process. Define realistic service and source-freshness objectives without promising upstream availability.
- An SSRF-resistant fetch path, tenant-private credentials and source entitlement checks must exist before customer-supplied URLs/private feeds are enabled.

**Exit gates:** restore/export/deletion drills and a billing reconciliation pass; bounded unit costs; security review of tenant access, fetch isolation and agent interfaces; agreed service terms and content rights. Public launch is a separate explicit decision.

## Phase 4 — Selective capability expansion

Outcome: move proven North Cloud capabilities into a product customers use.

Candidates: structured release/advisory feeds; domain-specific extraction; procurement signals; community taxonomy; externally hosted browser rendering for individually justified sources (never on the home Pi); acknowledged webhooks and delivery outbox; embeddings only if relevance evaluations demonstrate a useful improvement.

Every import requires a customer use case, an owner, contract/quality tests, a cost budget, a rights/provenance record, and removal or isolation of old infrastructure assumptions. Before splitting worker deployment from API across hosts, evaluate PostgreSQL and rehearse a migration. SQLite remains valid for a paid single-instance service while its measured write/availability limits are acceptable.

**Exit gates per capability:** useful to a real client, operable within its budget, and replaceable without forcing every customer to run or fund it.

## Phase 5 — Retire North Cloud

Outcome: North Cloud becomes a read-only historical archive.

- Inventory actual consumers, deployments, timers, data volumes, credentials, sources and contracts. Include Minoo, Claudriel and any other remaining consumers; repo docs alone do not prove they are inactive.
- Mark every retained capability migrated or intentionally retired in the ledger. Do not require porting unused experiments.
- Export required source configuration/content with provenance; restore and verify it. Compare old/new outputs where compatibility is promised.
- Disable legacy writers and scheduled jobs through a reviewed cutover; avoid concurrent source-registry ownership. Preserve a rollback window and verify downstream reads.
- Update downstream configuration/docs, revoke obsolete credentials, stop unneeded paid resources, and preserve required backups.
- Only after explicit approval, archive the GitHub repository and add a pointer to Northway. Archiving GitHub does not shut down infrastructure or preserve database volumes.

## First implementation work package

Build one vertical slice: selected source → persisted article → authenticated contextual query → explained result → Claudriel panel. Start with the Phase 1 foundations, not a mass cherry-pick. No Phase 2–5 work is required to demonstrate this slice.
