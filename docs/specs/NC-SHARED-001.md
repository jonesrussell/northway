# NC-SHARED-001: complete shared public acquisition
Revision 1, 2026-10-02. Owner-directed implementation: one job, one completion gate.

## Outcome and current baseline
Acquire public publisher metadata once globally, independent of customer count. All 88 reviewed definitions are in scope; the ten-source stage is historical, not a new delivery phase. Main 72b87fb already supplies PostgreSQL schema 13, explicit global corpus/jobs, private subscriptions, serial production polling, shared accounting and isolation. Reuse it; preserve current FETDER and PostgreSQL data.

## Required behavior
An offline activate-catalogue operation validates the immutable 88-source register, installed identity/policy and combined capacity before any write. Require a bounded approval record. Pending definitions become approved for linked-title, publisher, URL and date metadata only. Set all 88 eligible for onboarding. Preserve existing exact-register activation provenance and schedule; replay is idempotent. Reject excluded/disabled/conflicting sources atomically, including 401/403 policy denials. Operational errors on approved sources are not policy denials.

Daily acquisition, one active fetch globally, 100 combined configured sources, 180 rolling attempts/day and 64 MiB/day remain fixed. Preserve guarded HTTPS fetching, DNS pinning, deadlines, response limits, no redirects, host holds and conservative crash/failure accounting. No full text, summaries, original media, browser crawl, new accounts/grants/credentials, services or backups.

New shared-v1 workspaces receive private per-topic saved feeds referencing all approved enabled global sources. No duplicate source/job/article ownership. Repeated onboarding preserves chosen preferences; existing customers' saved selections are not silently expanded. Shared retrieval requires active workspace, enabled feed, subscription and current source policy. Context, private corpus, keys, snapshots and feedback remain tenant-private. Shared revisions invalidate stale completion/cache; tenant deletion preserves shared corpus and other subscriptions. Publisher disablement stops acquisition/visibility while preserving evidence.

## Acceptance
- Install/activate 88 definitions with 20 original topics and zero workspaces; retain provenance and exclusions.
- Upgrade previously activated ten to all 88; preserve old approval provenance, verify replay leaves revisions unchanged.
- Conflicts, denial or insufficient combined capacity produce no partial activation or source revival.
- All 88 complete an initial acquisition attempt with an honest recorded outcome, attribution and bounded retry/denial behavior.
- Two workspaces use the same acquisition/corpus; onboarding covers all sources and retries preserve preferences.
- Existing populated migration, HTTP/key isolation, shared cache/revocation, deletion, lease/crash, budget and host-hold acceptance tests pass.
- Full local checks and reviewed exact image/register identities precede specifically approved deployment; measure host effects and reconcile all source outcomes.

## Completion and release
Complete only after approved production release, all-88 initial-outcome reconciliation and reader/isolation checks. A local candidate is not completion. Prepare the concrete qualified release before requesting deployment approval. Retain PostgreSQL authority; additive forward repair preserves new writes, never restore stale SQLite. Natural bounded scheduling replaces simultaneous bulk fetching. Source failures do not justify bypasses or silently omitted definitions.

Residual work: changed customer preferences, broad crawling/discovery, media/full-text rights and FETDER creator publication. Publisher availability is external and must be reported truthfully.
