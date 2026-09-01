# HTTP API

Status: three authenticated product routes are implemented: query, snapshot GET and feedback. Retrieval is deterministic and metadata-only; there are no provider calls or scheduled-list routes. JSON schemas in api/schemas define shapes, with semantics below. Real HTTP/SQLite tests validate input failures, tenant isolation, replay and feedback; actual snapshot/problem responses are checked against the schemas by `make test`. OpenAPI generation and a second-client compatibility gate remain Phase 2 work.

## Transport and identity

Serve JSON over TLS to authenticated clients. Credentials are scoped service bearer keys; tenant identity is never a request field. Browser clients call their own backend rather than receiving service keys. Limit request bodies to 32 KiB and reject unknown fields, duplicate JSON object keys, unsupported media types and malformed/oversized input. Do not log credentials or raw context. Generate a request ID for each HTTP attempt. Expose administrative endpoints only privately.

| Method and path | Scope | Result |
|---|---|---|
| POST /v1/feed-queries | feeds:read | 200 feed snapshot; requires Idempotency-Key header |
| GET /v1/snapshots/{snapshot_id} | feeds:read | 200 stored snapshot, no new model call |
| POST /v1/feedback | feedback:write | 204 accepted event or identical replay |
| GET /v1/feeds/{feed_id}/latest-snapshot | feeds:read | Planned, not implemented; optional scheduled-list mode |

API keys may have a subset of scopes. All endpoints authorize object ownership; guessing another tenant's ID does not confirm existence. Phase 1 source/feed provisioning is operator-only; feed CRUD belongs to Phase 2.

## Query semantics

The request contains an existing feed_id, an explicit context envelope, max_age_hours (1–720) and limit (1–20). Claudriel initially requests at most five items. No repo contents, user identity or tenant override are needed. For personal feeds, context still includes a short intent and technologies is an empty array; focus may name the selected interests. Project context is not required. Saved-feed selection controls eligibility and must not be replaced by topic inference. Technology versions are optional; lack of version evidence prohibits precise compatibility claims.

Restrict retrieval to that feed's selected/entitled sources and current revision. max_age_hours filters known publication times; when publication time is unknown, use observation time for eligibility and keep published_at null. This is not proof that an undated article is recent. Coverage reports source-poll health, not exhaustive web coverage; sources_current cannot exceed sources_selected. Complete means all selected sources meet their configured freshness window, partial means some do, stale means none do. Empty items with complete coverage means no eligible results in this corpus, not that no news exists.

Return at most limit distinct authorized article IDs in a stable stored order. Include original URLs, observed/published timestamps, evidence basis and source attribution. Evidence must match available candidate material, not just pass JSON validation. Feed-only evidence may be insufficient for a summary: say so in summary/why_relevant and warnings. Do not fetch article pages to fill gaps. The article_excerpt basis is reserved for later explicitly permitted external collection; it must not be generated from a feed-only input.

ranking.mode identifies AI or deterministic fallback; version records the effective ranker/prompt/model configuration. Cache keys include tenant, feed/preference revision, entitlements, context digest, corpus revision and ranker version. Cache hits must recheck authorization. Inference is bounded to one attempt per uncached request initially. Timeout, invalid output or budget exhaustion falls back deterministically with a warning where feasible. Server unavailability is an error, not a fabricated empty feed.

## Retry and expiry

Idempotency-Key is a client-generated opaque value of 16–128 printable non-whitespace ASCII characters. Bind it to tenant, route and canonical validated request. Retain records for at least 24 hours; publish this minimum. Identical replay during that window returns the original snapshot without inference; changed payload conflicts; concurrent work returns 409 in_progress with Retry-After. Keep the snapshot available at least through its promised expires_at and any longer active replay guarantee. Clients must not change keys merely to bypass in-progress responses or quota denial.

Snapshot expires_at exceeds generated_at and describes the retention guarantee, not source freshness. GET returns stored ranking, generated_at and evidence; its request_id identifies the current HTTP attempt. After expiry, return 404 if removed. Recheck entitlements and suppress revoked content even during retention; access/rights changes override replay availability. Document that exception to clients. A new query is required to rerank.

Feedback uses event_id for idempotency. It must reference an item in an owned available snapshot. save, dismiss and less_like_this are explicit preference events. undo requires reverses_event_id for a matching unreversed event; other actions forbid that field. Never learn across customers by default. Conflicting event reuse returns 409.

## Errors

Errors use problem.schema.json (custom application/json, not a claim of RFC 9457 conformance). Map invalid_request→400, unauthorized→401, forbidden→403 for missing scope, not_found→404 for unavailable/private objects, conflict/in_progress→409, rate_limited→429, unavailable→503. Send Retry-After for retryable throttling/in-progress/service backoff. retryable guides a bounded client retry; it does not promise the same provider work can be repeated safely. Redact internal SQL, source credentials and raw provider output.

The HTTP adapter uses [durable query coordination](query-transactions.md), [deterministic retrieval](retrieval.md) and [atomic reversible feedback](feedback.md). In-progress maps to 409 with a one-second Retry-After. Authentication/transient storage failures return 503 with the same backoff. There is no per-tenant rate limiter yet (#20), so 429 remains reserved; do not expose the pilot publicly before operational abuse/resource gates are complete.

A failed/expired durable query key is terminal: map its unavailable error to `retryable=false` without Retry-After. Transient service failures remain retryable where applicable. Do not automatically replace a terminal key. A cached `deterministic_fallback` remains usable until its TTL even when funding/provider availability improves; its mode remains explicit.


## Implemented transport details and limits

- Every matched product attempt has a fresh server-generated X-Request-ID, also returned in snapshot/problem JSON, including authentication failures. A caller-supplied request ID is not trusted. Responses are no-store and nosniff. Auth is checked on every attempt; no credentials, raw context or internal SQL/errors are logged by the adapter.
- POST requires exactly one application/json Content-Type (optional charset=utf-8), no Content-Encoding and no query parameters. Invalid media type, body over 32 KiB, unsupported method on a known endpoint, invalid UTF-8/unpaired surrogate, unknown/case-mismatched/duplicate decoded object keys, nulls, trailing documents and invalid schema fields return 400 invalid_request. Protocol-level rejection before a request reaches the Go handler is outside the JSON application contract.
- Fields are case-sensitive, including nested context and technology objects. Optional fields may be omitted but not null; an explicit empty technology version is invalid. Text additionally follows the existing typed query policy: nonblank, no NUL, bounded Unicode lengths; IDs are canonical lowercase UUIDs. Exact integer JSON encodings such as 5, 5.0 and 5e0 normalize identically. Numeric length/exponent and JSON depth are bounded before allocation.
- POST query requires exactly one Idempotency-Key. GET snapshot accepts no body, query parameters or HEAD alias (HEAD is rejected with 400 and, per HTTP semantics, has no response body); it never calls Query. Unsupported/unclean paths do not redirect to another product operation. The optional latest-snapshot endpoint remains unavailable.
- The minimum query idempotency retention is 24 hours. A snapshot remains readable past expires_at while its longer replay retention is active; after that it is unavailable even before physical cleanup. A new key after cache expiry can create a new snapshot; an original key preserves its original snapshot. Failed/expired keys are terminal and never automatically recycled. Failure after a claim is created is reported as nonretryable unavailable in the first response too; failed cleanup may leave it in-progress until its bounded lease expires. Do not replace keys automatically.
- Feedback undo names the same snapshot and article as its target. Identical event replay rechecks current availability; revoked/expired evidence overrides replay, returning 404. Feedback changes a feed revision, but recorded events do not yet affect deterministic selection. See [feedback semantics](feedback.md).
- TLS terminates at the trusted private reverse proxy; the Go listener defaults to loopback. Migration/provisioning, approved source activation, rate limits/retention, Claudriel integration and Pi deployment remain separate operator/release work. Publisher scheduling is available only through trusted startup configuration for one provisioned tenant; product routes cannot start, enable or configure collection. This implementation neither activates a source nor grants source redistribution rights.


### Query/feedback overlap and recovery

A feedback event increments the feed revision atomically, as required by the data transaction contract. If it arrives after a query claim and before candidate reading/finalization, that query fails its revision check and becomes terminal (`503 unavailable`, retryable=false). This deliberately preserves the same revision/access fence used for other preference changes; a query cannot complete under a newer revision than it claimed. Even though this deterministic ranker does not yet consume event preferences, the cache invalidation contract remains in force.

Clients should avoid starting a refresh while their own feedback submission is pending. If another client or concurrent event changes the revision anyway, retain and display the last available snapshot (subject to normal access checks), show that the refresh did not complete, and offer an explicit Refresh action. That user action creates a new logical query with a fresh key against current revisions. It is not an automatic retry, periodic panel render or silent key replacement. Do not repeatedly retry the failed key; it stays terminal. If deliberately refreshed work also fails, show service unavailability rather than looping. Prior snapshot GET remains read-only and does not rerank.

This recovery path is tested through real HTTP and SQLite by pausing retrieval, committing feedback, releasing the query, observing terminal failure/replay, reading the old snapshot and explicitly requesting new work under the updated revision. Same-context coalescing and automatically restarting safe deterministic work are separate coordination changes, not implied here.
