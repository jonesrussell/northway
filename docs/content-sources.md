# Content sources and the Raspberry Pi pilot

Status: design, not a running collector. The owner approved the [five-source metadata-only PHP pilot](pilot-readiness.md) and future hourly Pi HTTPS polling on 2026-08-31. Read-only device and endpoint checks are complete; exact hardware measurements remain in private infra evidence. The current candidate filesystem is SD-backed, so storage/recovery validation remains a deployment gate. No scheduled polling is enabled. The earlier 1 GiB budget is a hypothesis, not a measured runtime requirement.

## Initial content

Northway starts with a small, operator-approved catalogue of publisher RSS/Atom feeds. It downloads feed documents and stores permitted metadata. This first pilot retains item IDs, titles, links, publisher attribution and publication/observation times only; excerpts and bodies need a separate rights decision. It does not discover or follow article links, crawl sites, render pages, download images or bypass blocks/paywalls. No publisher feed has been enabled by this specification.

For the first PHP workspace, evaluate the [official PHP Atom newsfeed](https://www.php.net/feed.atom), selected framework/library release feeds, and engineering blogs with publisher-provided syndication. PHP lists its Atom feed on its [official sitemap](https://www.php.net/sitemap.php). GitHub also provides a documented [releases API](https://docs.github.com/en/rest/releases/releases#list-releases), which can become a small separate adapter; not every Git tag is a published release. Select exact framework sources from the user's approved interests, not by uploading the repository or automatically subscribing to every dependency.

This pilot has exactly five approved sources; additions or substitutions require a new owner decision. Poll each source at most hourly, staggered, using conditional headers when supported, bounded response sizes and one concurrent fetch. Five hourly feeds mean at most 120 scheduled checks per day with zero automatic retries, not 120 article downloads; requests can still transfer full feed bodies if conditional requests are unsupported. Respect each publisher's tighter rate limits, cache instructions and Retry-After. A later capped retry policy needs separate implementation and review. Requests must identify the service honestly.

Each catalogue entry records URL, publisher, format, topics, visibility/tenant entitlement, poll policy, allowed retained fields, attribution and rights-review status. A working feed URL is not a redistribution license. Rights review is required before commercial reuse; no assumption of permission to reproduce full articles. Keep unpublished/private feeds tenant-scoped.

## What the Pi does

One Go process polls approved feed endpoints, normalizes and deduplicates items, indexes approved title metadata in SQLite FTS5 for this pilot, serves authenticated contextual queries, and stores snapshots/feedback. Candidate retrieval and a deterministic ranking mode work without an LLM. Optional bounded AI calls go to an external provider; no local model server, browser, embedding service or queue is required. Disclose the minimal context and source text sent to that provider.

The owner approved outbound HTTPS polling of the five selected feeds from the Pi; other feeds/API acquisition still require explicit selection. Feed polling still exposes the home IP to those endpoints. If no acquisition may originate from home, disable direct polling and use an external collector or provider: the Pi initiates an authenticated pull of bounded normalized batches. Keep original publisher provenance, rights and observation times, validate imported data, and checkpoint only after a durable transaction. No inbound home port or webhook is required for that import path. This alternative adds an external service and its costs; it is not configured yet.

Use local storage for SQLite, bounded logs and retention, and coherent off-device backups. The read-only check confirmed model/RAM/OS/architecture and current storage privately; recheck headroom and select the actual persistent data path before deployment. Validate builds and recovery on the actual device. Storage durability and power-loss recovery are release concerns, not assumptions.

## Honest limits

Coverage is limited to chosen sources, their feed history and available text. Do not backfill missing articles by scraping. This metadata-only pilot can offer a title/link recommendation but cannot infer release details or version-specific applicability from a headline. Label it "metadata only" or "insufficient detail" instead of inventing a summary. A later rights-approved excerpt mode would need its own retention and display policy; it is not part of this pilot. Source links let the user read the original. Preserve unknown publication times and expose source freshness/failures.

The default 90-day hot-corpus policy is a maximum subject to rights and a configurable storage ceiling, not a promise to retain everything. Stop or evict eligible content predictably before disk exhaustion; do not silently discard saved evidence.

## Later collection

Firecrawl or a separately hosted service scavenged from North Cloud can occupy the same adapter boundary. See the [external collection contract](specs/external-collection.md). Neither requires moving the core service off the Pi; neither is deployed by this plan.

Scheduled external AI search is also an optional pilot acquisition mode, alongside publisher feeds. See [scheduled discovery and custom lists](specs/scheduled-discovery.md) for cited evidence, no-home-crawl enforcement and budgets. It is distinct from ordinary ranking over already stored content; no external search is currently configured.

If real use demonstrates missing coverage, evaluate licensed content APIs or a separately hosted collector. All web crawling, full-page extraction and browser rendering stay outside the home/Pi deployment. Do not reactivate the North Cloud stack just to supply the pilot. Any imported collector functionality needs the migration ledger, fetch isolation, rights review and a measured cost justification.
