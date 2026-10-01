# Public feed qualification, 2026-10-01

## Owner-authorized scope

The owner requested heavy topic-agnostic RSS/Atom seeding, and authorized source selection and pre-release public fetch/parser qualification. That authorizes selecting a bounded healthy catalogue, not publisher rights, credential expansion, broad HTML crawling, originals/media republishing, billing, source-cap expansion or backups. No additional approval is required merely to choose feeds under this scope. Production admission waits for exact release qualification and verified remaining capacity.

Native bounded fetch/parser at importer source 8657889 performed 120 single attempts, about 10 MiB total response bytes, serially with per-host spacing, pinned public DNS, TLS hostname verification, no proxy/redirect-following/compression/retries and 15-second/2 MiB bounds. Redirect destinations were tested only when supplied by the audited research and the original returned 3xx; no alternative route was attempted after Arab News 403. Three explicit HTTPS URLs in the typed import were tested separately because the raw audit seed used HTTP. No original XML, item titles, article text or media corpus was retained.

95 of 113 candidates passed native transport and parsing. 88 are recommended for linked-headline metadata ingestion across 20 topics and 73 publishers. This is current observed compatibility, not an uptime or licensing guarantee. The full qualification report/register live in the release handoff; preserve their hashes and explicit restriction notes.

## Publisher exclusions

- Indian Express and Guardian retain the research's personal/noncommercial restrictions; exclude from the customer catalogue.
- CNA is query-bearing and explicitly restricted; exclude.
- ScienceDaily's [official RSS terms](https://www.sciencedaily.com/terms.htm) prohibit database/organizational storage without written consent, in addition to display constraints. The existing corpus persists metadata; exclude both feeds. The initial research note understated this storage restriction and the qualified register corrects it.
- Le Monde's [official RSS notice](https://www.lemonde.fr/en/about-us/article/2026/03/27/le-monde-rss-feeds_6751860_115.html) limits usage to personal/nonprofessional/noncollective use, with authorization/fees for other use; exclude.
- BBC cricket passes native parsing, but the official feed-terms URL linked from [BBC Developer](https://support.bbc.co.uk/platform/feeds/NewsFeeds.htm) returned a restricted-URL error during terms retrieval. That route was stopped; hold this entry pending terms qualification, without guessing permission.
- Transport/parser/no-store failures remain excluded; do not weaken SSRF, cache or parsing rules to fit the catalogue.

Other selected entries retain rights status not_assessed. Their basis is owner-authorized public linked-headline metadata aggregation with source attribution/original article links; no license or commercial-redistribution right is asserted. Retain only title, link, source and dates, as implemented; no summaries/full text/enclosures/media. Follow takedowns and update known publisher restrictions.

## Admission and expansion

Use a mixed first canary of ten: world, Canada, Indigenous, science, health, business, arts, sports, nature and food, each a distinct publisher. Then expand from the topic/publisher round-robin priority queue to min(88, 100 minus verified global poll rows), with further duplicate/immutable-profile reconciliation as required. Existing disabled rows count against 100. Do not automatically replicate across every workspace.

Use daily cadence and deterministic initial staggering. Supported inventory now reports global source slots, active sources, normal scheduled checks/day, customer workspace UUIDs and the named tenant's source policies. Require prospective normal checks/day plus existing work to fit 180; fixed 64 MiB/day byte reservation/charging remains the final runtime stop. One qualification sample's small actual bytes cannot justify lifting that limit.

## Current inventory route

Deployed 9c44b03 /northway management inventory succeeds and lists capabilities, not global RSS source count. Its collection status API counts only tenant HTML seeds and requires a distinct existing status scope; do not expand the separately approved FETDER read grant. The local importer catalogue inventory uses supported exclusive store ownership and must run during a coordinated paused-API window, with the exact qualified API image and existing volume. No live database copy, privilege override or permission workaround is needed. The previous standalone read-only PHP helper could not open SQLite; exact root cause is unverified.

Fresh host snapshot showed 1,058 MiB available RAM, 31 GiB free disk; API RSS about 3.4 MiB in its 128 MiB container. These observations do not qualify PostgreSQL placement and do not authorize provisioning it.
