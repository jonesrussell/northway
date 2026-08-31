# Optional external collection boundary

Status: design option, not a dependency of the first useful feed. No collector is deployed, Firecrawl account enabled, paid request issued or extra repository created.

## Replaceable collection

An external collector may be a managed API such as Firecrawl, or a small separately hosted service built from selected North Cloud behavior. Both deliver the same normalized document contract through Northway adapters. The Pi never dereferences article URLs, crawls sites or renders pages; calling an external HTTPS collection API is allowed only under its configured policy and budget.

Firecrawl documents [single-page scraping](https://docs.firecrawl.dev/api-reference/v2-introduction) and [site crawling](https://docs.firecrawl.dev/api-reference/endpoint/crawl-post). For this product, start with extraction of specifically approved URLs when feed/search evidence is inadequate, not whole-site crawling. Managed Firecrawl and self-hosted Firecrawl are different operational choices; self-hosting on the Pi would undermine the chosen boundary. Review current service/retention terms before use. The upstream [repository](https://github.com/firecrawl/firecrawl) and [self-hosting guide](https://github.com/firecrawl/firecrawl/blob/main/SELF_HOST.md) must be reviewed before adopting its code; a hosted API integration is not the same as importing its implementation.

If a custom collector becomes worthwhile, selectively extract North Cloud's bounded fetching, parsing, URL normalization, source policy and retry behavior. Read exact pinned paths and record provenance before importing anything. Do not port Elasticsearch, multiple databases, Redis publication or the entire classification/browser stack by default. Hosting, patches, destination security, rate controls and recovery become our responsibility. A small source-specific collector may suffice; parity with a managed crawling platform is not promised.

## Responsibilities

Northway owns tenant/source authorization, schedules, approved URL requests, extraction budget, deduplication, evidence validation, relevance and snapshots. Collector adapters own transport/provider translation. The external service owns isolated remote fetching/rendering, bounded execution and extraction results. Article content cannot expand a job's destination list, source access or spending policy.

An acquisition service receives only the required approved URL and extraction policy, not personal project context. Store provider credentials in operator configuration, never browser code, source URLs or Git. Tenant-private source credentials require separate isolation and explicit policy before support. Neither Firecrawl nor a custom collector guarantees access rights or makes blocked/paywalled content permissible.

## Normalized result contract

Finalize a versioned schema before implementing the adapter. Minimum fields: schema version, stable job/item IDs, approved source identity, requested and final URL, original publisher, known publication timestamp (nullable), observed/fetched timestamp, permitted extracted text/excerpt, content hash, extraction/provider version, evidence basis, rights/retention metadata and explicit success/partial/failure status. A collector response cannot assign its own tenant or grant source entitlements; Northway binds those from the authenticated job record.

Do not label generated summaries as extracted text. Preserve canonical/original URLs and failed extraction state. Validate byte/item limits, types, encodings and provenance before persistence. Reject unexpected destinations or unapproved redirects in returned metadata. Collector-side destination/dial checks are still required; Pi-side URL validation alone cannot prevent remote SSRF. Model-supplied URLs require explicit policy validation before becoming collection jobs.

For managed extraction, the Pi calls HTTPS and receives results or polls a provider job. For a custom worker, the Pi pulls authenticated result batches with a durable cursor. Advance the cursor only after a transaction commits; repeat delivery must be idempotent. No required inbound webhook or open home port. Retry with bounded backoff; cancellation/timeouts may not cancel remote work or remove charges, so reconcile job status rather than blindly resubmit.

## Decision gate

First measure missing evidence/coverage with feeds and scheduled AI search. Trial a managed adapter on a small rights-approved URL set only if the gap matters. Compare extracted evidence quality, failure rate, latency, per-useful-item cost, data handling and operator effort. Implement a North Cloud-derived service only if those measurements or source-control requirements justify owning it. Phase 4 is the default placement; bring forward a bounded extraction task only when pilot evidence establishes the need. Full extraction remains optional, and stale/partial results must not disable the rest of Northway.
