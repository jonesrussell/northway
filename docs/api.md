# Draft HTTP API

Status: specified, not implemented. JSON schemas in api/schemas are authoritative shapes; examples are synthetic. These semantics supplement schema validation and require server integration tests in Phase 1. OpenAPI generation and a second-client compatibility gate precede the Phase 2 preview.

## Transport and identity

Serve JSON over TLS to authenticated clients. Credentials are scoped service bearer keys; tenant identity is never a request field. Browser clients call their own backend rather than receiving service keys. Limit request bodies to 32 KiB and reject unknown fields, duplicate JSON object keys, unsupported media types and malformed/oversized input. Do not log credentials or raw context. Generate a request ID for each HTTP attempt. Expose administrative endpoints only privately.

| Method and path | Scope | Result |
|---|---|---|
| POST /v1/feed-queries | feeds:read | 200 feed snapshot; requires Idempotency-Key header |
| GET /v1/snapshots/{snapshot_id} | feeds:read | 200 stored snapshot, no new model call |
| POST /v1/feedback | feedback:write | 204 accepted event or identical replay |
| GET /v1/feeds/{feed_id}/latest-snapshot | feeds:read | Optional scheduled-list mode: 200 stored snapshot or 404 before first result; no paid work |

API keys may have a subset of scopes. All endpoints authorize object ownership; guessing another tenant's ID does not confirm existence. Phase 1 source/feed provisioning is operator-only; feed CRUD belongs to Phase 2.

## Query semantics

The request contains an existing feed_id, explicit technical context, max_age_hours (1–720) and limit (1–20). Claudriel initially requests at most five items. No repo contents, user identity or tenant override are needed. Technology versions are optional; lack of version evidence prohibits precise compatibility claims.

Restrict retrieval to that feed's selected/entitled sources and current revision. max_age_hours filters known publication times; when publication time is unknown, use observation time for eligibility and keep published_at null. This is not proof that an undated article is recent. Coverage reports source-poll health, not exhaustive web coverage; sources_current cannot exceed sources_selected. Complete means all selected sources meet their configured freshness window, partial means some do, stale means none do. Empty items with complete coverage means no eligible results in this corpus, not that no news exists.

Return at most limit distinct authorized article IDs in a stable stored order. Include original URLs, observed/published timestamps, evidence basis and source attribution. Evidence must match available candidate material, not just pass JSON validation. Feed-only evidence may be insufficient for a summary: say so in summary/why_relevant and warnings. Do not fetch article pages to fill gaps. The article_excerpt basis is reserved for later explicitly permitted external collection; it must not be generated from a feed-only input.

ranking.mode identifies AI or deterministic fallback; version records the effective ranker/prompt/model configuration. Cache keys include tenant, feed/preference revision, entitlements, context digest, corpus revision and ranker version. Cache hits must recheck authorization. Inference is bounded to one attempt per uncached request initially. Timeout, invalid output or budget exhaustion falls back deterministically with a warning where feasible. Server unavailability is an error, not a fabricated empty feed.

## Retry and expiry

Idempotency-Key is a client-generated opaque value of 16–128 printable non-whitespace ASCII characters. Bind it to tenant, route and canonical validated request. Retain records for at least 24 hours; publish this minimum. Identical replay during that window returns the original snapshot without inference; changed payload conflicts; concurrent work returns 409 in_progress with Retry-After. Keep the snapshot available at least through its promised expires_at and any longer active replay guarantee. Clients must not change keys merely to bypass in-progress responses or quota denial.

Snapshot expires_at exceeds generated_at and describes the retention guarantee, not source freshness. GET returns stored ranking, generated_at and evidence; its request_id identifies the current HTTP attempt. After expiry, return 404 if removed. Recheck entitlements and suppress revoked content even during retention; access/rights changes override replay availability. Document that exception to clients. A new query is required to rerank.

Feedback uses event_id for idempotency. It must reference an item in an owned available snapshot. save, dismiss and less_like_this are explicit preference events. undo requires reverses_event_id for a matching unreversed event; other actions forbid that field. Never learn across customers by default. Conflicting event reuse returns 409.

## Errors

Errors use problem.schema.json (custom application/json, not a claim of RFC 9457 conformance). Map invalid_request→400, unauthorized→401, forbidden→403 for missing scope, not_found→404 for unavailable/private objects, conflict/in_progress→409, rate_limited→429, unavailable→503. Send Retry-After for retryable throttling/in-progress/service backoff. retryable guides a bounded client retry; it does not promise the same provider work can be repeated safely. Redact internal SQL, source credentials and raw provider output.
