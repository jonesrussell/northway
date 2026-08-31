# PHP pilot source and device review

Status: preparation for #9, not source approval or deployment. The five candidates in [the proposal manifest](../catalogue/php-pilot.json) are disabled. The runtime does not load this file. Issue #9 stays open until source selection, acquisition location and device facts are confirmed; merging this preparation must not unblock live ingestion automatically.

## Proposed first five sources

Start with PHP itself and general development tools; do not infer Laravel, Symfony or another framework from a private project or subscribe to every dependency. These are release-oriented signals, not comprehensive PHP news or a security-alert service.

| Candidate | Exact feed | Publisher evidence | Intended signal |
|---|---|---|---|
| PHP news | [Atom](https://www.php.net/feed.atom) | [PHP sitemap](https://www.php.net/sitemap.php) explicitly links its newsfeed | Language releases and announcements |
| Composer | [Release Atom](https://github.com/composer/composer/releases.atom) | [Official site](https://getcomposer.org/) links its repository | Dependency-management changes |
| PHPUnit | [Release Atom](https://github.com/sebastianbergmann/phpunit/releases.atom) | [Official site](https://phpunit.de/) links its code repository | Testing-tool releases |
| PHPStan | [Release Atom](https://github.com/phpstan/phpstan/releases.atom) | [Project releases](https://github.com/phpstan/phpstan/releases) | Static-analysis changes |
| Psalm | [Release Atom](https://github.com/vimeo/psalm/releases.atom) | [Official site](https://psalm.dev/) links its repository | Static-analysis changes |

Review date: 2026-08-31. Public web inspection established the publisher/repository links. The web tool reported `application/atom+xml` for each endpoint but could not parse that media type. **No endpoint XML, conditional 304 behavior, response size, rate limit or Pi reachability has been verified.** A media-type observation is not a successful ingestion test. No publisher feed bodies, articles or private project content are committed here. These dated observations are not a guarantee that endpoints will remain available.

The manifest is the machine-readable proposal, not a tenant registry or fetch allowlist. Proposed polling is hourly, staggered, one concurrent request, 2 MiB maximum decoded response, 15-second timeout and no automatic article-link traversal. Five hourly sources imply at most 120 scheduled attempts/day before separately capped retries. These are proposed Northway ceilings, not publisher permission or measured Pi budgets. Respect tighter publisher limits, conditional validators and Retry-After. #12 must enforce destination/DNS/redirect, decompression and parser bounds; validating a URL here does not make it safe to fetch.

## Content rights and AI boundary

All five rights decisions remain pending; no commercial redistribution or provider export is approved. The proposed first retained fields are source item ID, title, canonical link, publisher attribution, publication time when supplied and Northway observation time. **Descriptions, release-note bodies, full articles, images and enclosures are excluded from this proposal.** Titles/metadata may be insufficient for a meaningful explanation: label that limitation and link to the publisher. Do not fabricate a summary or imply version applicability from a headline.

The owner must review the intended personal-pilot use and the publisher/platform terms before activation. Useful references include [PHP copyright](https://www.php.net/copyright.php), the [Composer site license statement](https://getcomposer.org/) and [GitHub terms](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service). A repository's code license, public visibility or syndication endpoint is not treated here as clearance to republish all release-note content or send it to an AI provider. Commercial use and excerpts require a separate explicit rights decision. This is a conservative product policy, not a legal clearance opinion.

No source is fetched by CI. [Synthetic feed fixtures](../testdata/feeds/README.md) are authored for this repository and use reserved `.invalid` domains. They establish test inputs, not production parser, network, safety or ingestion behavior.

## Device evidence and privacy

The existing [private deployment repository](https://github.com/jonesrussell/waaseyaa-infra) was reviewed read-only on 2026-08-31, using its fetched default branch while preserving the operator's current checkout. Its inventory/hardware documentation describes an asset and target configuration, not a fresh measurement of installed RAM, OS, mounted storage or available headroom. No host was contacted, no environment/credential files were read, and no infrastructure files were changed. Host addresses, inventory details and routing remain private.

Northway's public portable target remains Linux ARM64. Cross-build and 128 MiB container smoke evidence already exist; neither establishes native Pi execution, available memory or storage durability. Keep these distinct:

| Fact needed | Current evidence | Remaining action |
|---|---|---|
| Installed model and RAM | Private documentation only; no live verification | Bounded read-only device check |
| OS and architecture | Portable ARM64 target; no live verification | Read OS metadata and architecture |
| SQLite storage medium/filesystem/free space | No live verification | Inspect only the deployment-owned data filesystem |
| Shared-host headroom and enforced limits | No live verification | Snapshot available memory, load, filesystem capacity and aggregate container resource usage |
| Outbound acquisition location | Home crawling is prohibited; feed/API polling is still undecided | Owner chooses Pi HTTPS polling or external collection |
| Native runtime and recovery | Not established by this slice | #20/#36 device execution and restore gates |

For an authorized device check, use the deployment owner's existing access procedure. Limit reads to model, memory totals/availability, architecture, OS version, target filesystem type/capacity and aggregate resource use. Do not dump environment variables, full container inspect/config, process arguments, credentials, database contents, serial numbers or network inventory. Store any exact host evidence only in the private infra repository through its reviewed workflow. Do not install tools, change cgroups, restart containers, migrate data or deploy as part of a read-only check.

## Decisions required to finish #9

1. Accept or adjust the five exact sources and metadata-only retention proposal; record the rights-review decision before activation.
2. Decide whether hourly outbound feed HTTPS requests may originate from the Pi. This is feed polling, not crawling, but still exposes its public IP to publishers. Otherwise keep acquisition external and build the authenticated batch-import path.
3. Authorize the bounded read-only device check through waaseyaa-infra, or supply equivalent current facts privately. [#36](https://github.com/jonesrussell/northway/issues/36) explicitly excludes production SSH/host inspection without separate authorization; this source-preparation PR does not supply it.

Then complete #9's evidence and proceed to #12's bounded collector. Do not equate source selection with deployment approval. No sources, providers, domains, credentials, live polling or paid services are activated by this change.
