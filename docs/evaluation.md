# Evaluation and release gates

Status: planned checks. The foundation's JSON contract checks are not runtime evidence.

## Relevance

Use authored metadata-only fixtures and examples labelled by the user; publisher excerpts require a separate rights decision. Include all five personal interest areas plus PHP releases, irrelevant tutorials, repeated stories, missing dates and weakly supported applicability. Evaluate personal queries with empty technologies, two project contexts, and an explicitly selected mixed feed. See [synthetic personal inputs](../testdata/personal/README.md).

Compare deterministic retrieval with AI reranking. Target at least 80% relevant items in top-five results within each interest area and mixed-digest manually reviewed sample, while separately reviewing excluded relevant examples. Record source diversity and duplicate rate; do not optimize only for clicks. Require evidence for why a version-specific item applies. Unknown applicability must remain unknown.

## Failure and replay

- Conditional fetch 200/304, malformed feed, timeout, oversized body, redirect and backoff.
- Repeated ingestion yields one canonical item; changed content creates a new version/observation as intended.
- Process interruption does not lose poll checkpoints or mark incomplete work complete. Backlogs over 100 items and identical timestamps drain fully.
- Model timeout, invalid JSON, invented candidate IDs, injected instructions and exceeded budget produce a labelled deterministic fallback or explicit error, never an unbounded retry.
- Empty relevant results, partially unavailable sources, stale cache and provider failure are distinguishable.
- Snapshot reads do not rerun AI. Feedback retry is idempotent; contradictory reuse of an idempotency key is rejected.

## Isolation

Create two tenants from Phase 1 tests. Probe feed IDs, snapshots, item membership, feedback, API scopes, logs and cache reuse across them. Include job execution with no tenant, connection pragma enforcement, public/private source entitlements and concurrent budget reservation. API-layer tests alone are insufficient; exercise every tenant-scoped SQL path with a real file-backed SQLite database, including FTS joins and concurrent writes. SQLite has no RLS safety net.

## Performance and economics

Run the actually approved source set under the shared attempt/byte budget (see [personal feeds](personal-feeds.md)) on the user's Raspberry Pi after recording model, RAM, OS architecture and storage. Establish its budget from that device rather than assuming the earlier 1 GiB hypothesis. Measure idle and peak RSS, CPU time, disk growth, query latency (cached/uncached), provider tokens/cost, source last-success age and operator time. Leave at least 30% memory headroom under ingestion plus query and backup load. Target cached responses under 500 ms and uncached under 10 s for the pilot; these are evaluation targets, not advertised SLAs.

One bounded ranking call per cache miss initially. Record failed/retried provider work and cache hits separately. Measure cost per useful result, not merely per request. Test retention and restore before deleting any legacy data. Do not quote a savings percentage against North Cloud's memory-limit sum as if it were measured usage.

## Product proof

Phase 1: two weeks of user review across the five personal interests and mixed digest in Claudriel, including use without a coding project. Unavailable First Nations coverage cannot be substituted silently with generic Canadian news; publisher concentration and empty categories remain visible. PHP-only success is insufficient. Phase 2: a second agent consumes the same API and gets useful contextual results without relying on Claudriel's prompt, private memory, or UI. Phase 3: billing and export/deletion/restore drills plus source-rights and security reviews before paid beta.
