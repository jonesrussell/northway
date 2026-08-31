# PHP pilot source and device review

Status: the owner approved the five-source personal, metadata-only pilot and future hourly HTTPS feed polling from the Pi on 2026-08-31. The separately authorized read-only device check and bounded endpoint probes are complete. The [selection manifest](../catalogue/php-pilot.json) remains disabled and the runtime does not load it. This completes source/device discovery for #9, not ingestion implementation, source activation or deployment.

## Approved first five sources

Start with PHP itself and general development tools; do not infer Laravel, Symfony or another framework from a private project or subscribe to every dependency. These are release-oriented signals, not comprehensive PHP news or a security-alert service.

| Candidate | Exact feed | Publisher evidence | Intended signal |
|---|---|---|---|
| PHP news | [Atom](https://www.php.net/feed.atom) | [PHP sitemap](https://www.php.net/sitemap.php) explicitly links its newsfeed | Language releases and announcements |
| Composer | [Release Atom](https://github.com/composer/composer/releases.atom) | [Official site](https://getcomposer.org/) links its repository | Dependency-management changes |
| PHPUnit | [Release Atom](https://github.com/sebastianbergmann/phpunit/releases.atom) | [Official site](https://phpunit.de/) links its code repository | Testing-tool releases |
| PHPStan | [Release Atom](https://github.com/phpstan/phpstan/releases.atom) | [Official installation guide](https://phpstan.org/user-guide/getting-started) links a GitHub release download | Static-analysis changes |
| Psalm | [Release Atom](https://github.com/vimeo/psalm/releases.atom) | [Official site](https://psalm.dev/) links its repository | Static-analysis changes |

Evidence date: 2026-08-31. Public publisher links were inspected, then each exact feed was requested from the Pi once, with one conditional follow-up using its ETag. All five returned HTTP 200, Atom media type and parseable Atom XML within the 2 MiB ceiling. These are dated observations, not an availability or publisher-rate guarantee.

| Source | Initial decoded bytes | Atom entries | Conditional response |
|---|---:|---:|---|
| PHP | 1,461,199 | 870 | 304 |
| Composer | 47,517 | 10 | 304 |
| PHPUnit | 32,547 | 10 | 304 |
| PHPStan | 274,862 | 10 | 200, full response again |
| Psalm | 73,864 | 10 | 304 |

The one-off probe used existing Python, verified TLS, pinned a publicly routable resolved destination, refused redirects and compression, limited each request to 15 seconds/2 MiB, rejected DTD/entity declarations and parsed transiently in memory. It fetched no article links or enclosures, installed nothing, and retained only response metadata/counts. No publisher feed bodies, articles or private project content are committed or sent to an AI provider. Four observed 304s do not guarantee future conditional responses; PHPStan's `conditional_get_verified` is false. The probe is not the production collector or its security test suite. PHP's large feed makes streaming parsing, bounded entries/work and an explicit oversized-feed failure policy important in #12.

The manifest is the machine-readable selection record, not a tenant registry or fetch allowlist. Approved polling policy is hourly, staggered, one concurrent request, 2 MiB maximum decoded response, 15-second timeout and no automatic article-link traversal. Five hourly sources imply at most 120 scheduled attempts/day; the proposal permits zero automatic retries and spreads sources across the interval. Any later capped retry policy needs its own #12 implementation/review. These are Northway ceilings, not publisher permission or measured Pi budgets. Respect tighter publisher limits, conditional validators and Retry-After. #12 must enforce destination/DNS/redirect, decompression and parser bounds; validating a URL here does not make it safe to fetch.

## Content rights and AI boundary

The owner accepted the metadata-only personal-pilot policy. All five publisher-rights reviews remain pending; owner approval is not publisher permission or legal clearance. No commercial redistribution or provider export is approved. The approved first retained fields are source item ID, title, canonical link, publisher attribution, publication time when supplied and Northway observation time. **Descriptions, release-note bodies, full articles, images and enclosures are excluded from this pilot.** Titles/metadata may be insufficient for a meaningful explanation: label that limitation and link to the publisher. Do not fabricate a summary or imply version applicability from a headline.

Publisher/platform terms must still be reviewed before activation; source-selection approval does not waive that gate. Useful references include [PHP copyright](https://www.php.net/copyright.php), the [Composer site license statement](https://getcomposer.org/) and [GitHub terms](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service). A repository's code license, public visibility or syndication endpoint is not treated here as clearance to republish all release-note content or send it to an AI provider. Commercial use and excerpts require a separate explicit rights decision. This is a conservative product policy, not a legal clearance opinion.

No source is fetched by CI. [Synthetic feed fixtures](../testdata/feeds/README.md) are authored for this repository and use reserved `.invalid` domains. They establish test inputs, not production parser, network, safety or ingestion behavior.

## Device evidence and privacy

The authorized read-only live check used the existing [private deployment repository](https://github.com/jonesrussell/waaseyaa-infra)'s access procedure with normal SSH host-key verification. Exact model, RAM, OS, storage and aggregate resource measurements are recorded in its private `docs/northway-pilot-capacity-2026-08-31.md` evidence record. No credentials/environment, production data, process arguments, full container configuration or network inventory were read. No host configuration changed; no tools were installed and no service was deployed or restarted. The infra checkout's pre-existing working branch was preserved.

Northway's portable target remains Linux ARM64. The live architecture matches it and the point-in-time check found memory/free-space headroom for further pilot evaluation. **The current candidate data filesystem is SD-backed, not SSD-backed.** This is a durability concern, not proof the pilot cannot work. #36 must explicitly select the persistent data path/storage strategy; #20 must verify native execution, bounded workload, coherent off-device backup/restore and recovery before activation. Avoid write-heavy backfill and unnecessary rewrites; preserve SQLite durability settings. A single idle-host snapshot cannot establish safe production limits or power-loss safety.

| Fact needed | Current evidence | Remaining action |
|---|---|---|
| Installed model and RAM | Live verified; exact facts private | Recheck near deployment |
| OS and architecture | Live verified; ARM64 target matches | Native application execution in #20/#36 |
| SQLite storage medium/filesystem/free space | Current candidate filesystem live verified; SD-backed | #36 chooses actual Northway volume/path and durability plan |
| Shared-host headroom and limits | Live memory/load/capacity and aggregate container usage/limit snapshot recorded privately | Measure Northway workload; prove its own enforced limits |
| Outbound acquisition location | Owner approved Pi HTTPS feed polling; five endpoints reachable | Implement bounded collector; no schedule enabled |
| Native runtime and recovery | Not established by this check | #20/#36 execution and restore gates |

For an authorized device check, use the deployment owner's existing access procedure. Limit reads to model, memory totals/availability, architecture, OS version, target filesystem type/capacity and aggregate resource use. Do not dump environment variables, full container inspect/config, process arguments, credentials, database contents, serial numbers or network inventory. Store any exact host evidence only in the private infra repository through its reviewed workflow. Do not install tools, change cgroups, restart containers, migrate data or deploy as part of a read-only check.

## Approval and next gate

On 2026-08-31 the owner explicitly accepted these five metadata-only sources, hourly Pi HTTPS feed polling and the separate read-only capacity check. This allows #9's source/device discovery to finish and #12's collector implementation to proceed. It does not authorize deployment or starting a timer. `proposal_only: true` means this remains a non-runtime planning record even though `selection_status` records owner approval; `enabled: false` remains mandatory globally and per source.

#12 must enforce destination/DNS/redirect safety, decoded byte/parser/work bounds, persistent conditional validators, transactional item identity/deduplication and failure/poll state. A 200 response to a conditional request is normal and must not duplicate items or trigger uncontrolled writes. Retain only the selected metadata fields; do not import descriptions into the corpus. Finish publisher-rights review before activation. Storage selection, actual-device workload and recovery remain #20/#36 deployment gates. No providers, ingress, credentials, live polling, paid services or North Cloud imports are activated by this change.
