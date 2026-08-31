# Scheduled AI discovery and custom lists

Status: optional Phase 1 capability, not enabled. No provider calls, subscription, paid resource or real schedule is created by this document. Applies to NW-003 through NW-009, NW-013 and NW-016.

## Two distinct jobs

**Discovery** asks a web-search-capable external provider for recent items matching an approved topic profile. The provider performs retrieval outside the home network. Northway receives source-linked candidates, validates them and imports permitted metadata/excerpts into the same article store used by feeds. Ordinary text generation without retrieval is not a news source.

**Digest generation** selects from stored content for a saved feed/context profile, optionally reranks it, and writes a snapshot on schedule. Claudriel reads the latest available snapshot without triggering a fresh search or model call on each panel render. The existing on-demand query path remains available with its own caps.

For example: discover PHP ecosystem updates every six hours, then produce a daily top-five PHP engineering digest at 08:00 America/Toronto. These are illustrative schedules, not defaults that have been activated. The cheaper initial configuration is one daily discovery and one cached list. Independently disable either job; feed-only retrieval and deterministic ranking remain usable.

## Provider decision

Evaluate Anthropic's [server-side web search](https://platform.claude.com/docs/en/agents-and-tools/tool-use/web-search-tool) first because an Anthropic adapter is already selected. Its search tooling supplies citations and separate search usage; inspect tool errors even when the HTTP response is successful. Keep search separate from the tool-free ranking interface. Use only server-side search, with explicit search-use caps; no local browser, code execution or unrestricted agent loop.

Compare [Perplexity Sonar](https://docs.perplexity.ai/docs/sonar/models/sonar) if results or cost justify a second provider. Its [filters](https://docs.perplexity.ai/docs/sonar/filters) support domain and recency constraints. Do not claim either provider guarantees exhaustive coverage, correct publication dates or valid evidence merely because a citation is present. Select model/tool versions and current pricing at implementation time using a representative evaluation; do not hardcode a provider choice from a marketing comparison. No second SDK is added before that decision.

## Saved profile and safety

A tenant-owned revisioned profile records topics/intent, optional technologies and versions, source/domain preferences, exclusions, language, lookback window, result cap, schedule/timezone, provider/model policy and daily/monthly spending caps. Only operator-approved profiles in the pilot; source/profile authoring by agents is a later scoped write capability. Changes increment a revision and invalidate corresponding personalized caches.

Send only approved topic/context fields, not repository contents, private memory, conversations or credentials. Search-result text and generated lists cannot edit their profile, schedule, provider tools or budget. Results produced from private context remain tenant-private even if their source URLs are public; sharing a public corpus never justifies sharing a personalized discovery query or result set.

Validate bounded structured records: original source URL, source title, available publication time, retrieval time, returned supporting excerpt/citation, and provider/query revision provenance. Validate URLs without dereferencing article pages from home. A provider citation is evidence of attribution, not independent verification of an article or claim. Reject unsupported claims; if only metadata is available, retain a link with metadata-only evidence and unknown applicability. Preserve unknown publication times and do not relabel old news as new because search returned it today. Deduplicate across RSS and search using normalized URL and content/version rules.

Use evidence basis web_search_excerpt for actual provider-supplied cited snippets; never mislabel model prose as an original excerpt or article_excerpt. Keep generated explanations distinct. Content-source rights and provider storage/redistribution terms govern retained fields and commercial use.

## Durable scheduling and budget

Persist next due time, profile revision, scheduled slot, lease/fencing token, attempts, completion and last-success/error in SQLite. A unique (tenant, profile, revision, scheduled slot, job kind) identifies scheduled work. Atomically claim work and reserve budget, then commit before external I/O. Prevent overlap across restarts/processes. Record timestamps in UTC while interpreting wall-clock schedules in an explicit IANA timezone. Define DST behavior: run once at the first occurrence of an ambiguous time and at the first valid time after a skipped time.

When the Pi returns after downtime, coalesce missed slots into one current run; never replay an unbounded backlog of paid searches. An ambiguous provider timeout leaves reconciliation work, not permission to automatically retry and spend twice. Bound lookback overlap for late-indexed items, requests/search invocations/tokens/results/time, and overall daily/monthly cost. A single model request can execute multiple billable searches; count and cap both. Do not claim exact-once provider execution unless the provider supports a verified idempotency guarantee.

Stop paid work at budget exhaustion. Display the last successful snapshot with generated_at and staleness/failure warnings; an upstream error must not overwrite it with an empty "no news" list. Notifications, email and webhooks are outside this pilot: Claudriel pulls the latest snapshot. Add authenticated GET /v1/feeds/{feed_id}/latest-snapshot (feeds:read) for this mode; return the same snapshot contract or 404 before the first result, with no implicit paid work. Apply current ownership/entitlement checks.

## Acceptance

Demonstrate a cited PHP list from a representative provider evaluation, useful results beyond the RSS baseline, provenance preserved across deduplication, and no home article-page requests. Test two-tenant isolation, prompt injection, invalid/uncited results, stale publication dates, provider HTTP-200 tool errors, token/search cap exhaustion, concurrent claims, restart recovery, clock/DST boundaries and no catch-up spending storm. Measure incremental cost per accepted new item and per digest. Keep this optional if it does not improve usefulness enough to justify its cost.
