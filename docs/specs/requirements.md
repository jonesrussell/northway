# Product and engineering requirements

Status: requirements are delivered incrementally; runtime and storage tests establish only their scoped portions, not the complete feed-service/release requirements below. Requirement IDs are stable references for issues, tests and review; changing an invariant requires a spec/ADR update in the same PR.

| ID | Requirement | Verification before release |
|---|---|---|
| NW-001 | A generic authenticated agent can query a saved feed with a bounded context envelope. | HTTP contract test; second independent client in Phase 2. |
| NW-002 | Tenant identity comes only from a validated credential. Every private object and cache entry is scoped. | Two-tenant negative tests at HTTP and real tenant-scoped SQL paths; missing-tenant fail-closed test. |
| NW-003 | Source ingestion is conditional, bounded, restart-safe and idempotent while preserving changed items. | 200/304/update/retry/crash fixtures; real persisted state. |
| NW-004 | A response distinguishes publication time, observation time, source freshness and incomplete coverage. | Partial-source failure, unknown publication date and stale-snapshot cases. |
| NW-005 | AI may select/explain only permitted candidates; no fabricated source, item ID, citation or applicability claim. | Structured-output checks, labelled review, adversarial fixtures. |
| NW-006 | AI failure or budget exhaustion has a labelled deterministic fallback; no unbounded work. | Timeouts, invalid output, cap/race tests and cancellation. |
| NW-007 | Query retries and snapshot reads do not duplicate inference or usage; feedback is idempotent and reversible. | Idempotency-key replay/conflict/concurrent request tests. |
| NW-008 | Raw private project/context data is minimized and never placed in ordinary logs or cross-tenant caches. | Logging/caching/deletion tests; provider data-handling disclosure. |
| NW-009 | Quotas reserve worst-case allowed work atomically before provider calls, then reconcile actual usage. | Concurrent budget exhaustion, abandoned reservation, retry/provider failure tests. |
| NW-010 | Migration, backup, retention and restore preserve authorized data and required provenance. | Fresh-install/upgrade/restore/cleanup tests with real file-backed SQLite. |
| NW-011 | Go package boundaries, formatting, tests, race checks, generated SQL and dependency checks are reproducible. | CI and local harness use the same commands; no ignored failing gate. |
| NW-012 | Claudriel integrates through its PHP-backed tool boundary; news does not reuse the leads contract. | Client contract tests and end-to-end panel test; no shared app imports. |
| NW-013 | Public/private source entitlements and fetch destination policy cannot be bypassed by agents or article text. | Redirect/DNS/private-address/permission tests before private/custom sources. |
| NW-014 | Commercial launch requires measured unit costs, source-rights review, tenant lifecycle and reconciled billing. | Paid-beta release checklist; no automatic public launch. |
| NW-016 | The Pi performs no website crawling, article-page fetches, browser rendering or local model inference. Only approved bounded feed/API polling is allowed; external collection is optional. | Network fixture asserts no article-link traversal; actual-device resource/recovery test; outbound-only external import mode if needed. |
| NW-015 | North Cloud imports have provenance and selected behavior tests; retirement is separately gated. | Migration ledger plus consumer/data cutover and explicit archive approval. |

## Non-goals

Northway is not a repository executor, general autonomous web researcher, social-publishing platform or emergency-warning authority. Enterprise readiness means demonstrable boundaries, recovery and delivery controls; this specification is not a SOC 2/ISO certification or a claim that the not-yet-built product is enterprise-ready.
