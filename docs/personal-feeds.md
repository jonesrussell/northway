# Personal feeds beyond PHP

Status: source curation and design for #44, not implemented ingestion, ranking or UI. The owner requested development (including PHP), entertainment, Canadian news, First Nations news and world news. PHP remains a useful technical bootstrap; it does not define the product or the full personal-pilot acceptance gate.

## Source candidates and evidence

[The candidate manifest](../catalogue/personal-pilot.json) records two candidates per interest area, exact endpoints, publisher attribution, rights-review status and dated probe outcomes. Every candidate is disabled and awaiting individual source approval. The [five-source PHP bootstrap](pilot-readiness.md) and its exact approval checks are unchanged. Approval of an interest area is not permission to fetch arbitrary URLs or export content to an AI provider.

Workstation WSL checks on 2026-08-31 (not Pi evidence):

| Interest | Candidate | Initial result | Conditional result |
|---|---|---|---|
| Development | InfoQ Programming | RSS parsed; 13,272 bytes, 8 entries | 200 again |
| Development | GitHub Blog | 200 RSS media type; 173,457 bytes; conservative XML screen rejected | Not attempted |
| Entertainment | Variety | RSS parsed; 18,301 bytes, 10 entries | 304 |
| Entertainment | Global Entertainment | RSS parsed; 22,381 bytes, 10 entries | 304 |
| Canada | CBC Canada | Timeout; no response recorded | Not attempted |
| Canada | Global Canada | RSS parsed; 21,082 bytes, 10 entries | 304 |
| First Nations | Anishinabek News | 403; body not parsed | Not attempted |
| First Nations | APTN News | Not probed; permission-dependent | Not attempted |
| World | BBC World | RSS parsed; 20,294 bytes, 26 entries | No validator supplied |
| World | Global World | RSS parsed; 20,830 bytes, 10 entries | 304 |

The one-off probe made exact feed-document requests with verified TLS, a fresh publicly routable DNS result pinned for each connection, no redirects, identity encoding, a 15-second deadline and 2 MiB byte ceiling. DTD/entity markers were rejected before parsing. It kept response metadata/counts only; no publisher bodies, titles, images, enclosures or article pages were retained or sent to a model. It made at most one conditional follow-up, no automatic retries, and wrote nothing to the Pi. These checks are not the production transport/parser or its SSRF tests. The byte screen is deliberately conservative: GitHub's rejection could reflect a declaration-like string inside content; it is not evidence that the feed is malicious or malformed. Do not weaken the production parser based on that observation.

CBC World also timed out during discovery; it is not in the proposed ten-source budget. A web-tool timeout for Two Row Times and publisher reference for Turtle Island News were considered as further research leads, not extra approved feeds. No access-block evasion or alternate-agent impersonation was attempted. None of these observations establishes freshness, continuous availability, completeness or Pi reachability. Publication dates and editorial relevance were not assessed by the metadata-only probe.

## Publisher and rights review

The [InfoQ programming page](https://www.infoq.com/programming/) links its RSS offering. [InfoQ terms](https://www.infoq.com/terms-and-conditions/) permit a summary/link approach and reject whole-work republication; this pilot stays narrower, retaining selected metadata only. That statement is not blanket SaaS or AI-provider permission. [GitHub Blog](https://github.blog/) and [GitHub platform terms](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service) are references for further blog-specific review, not a claimed content licence.

[Global's directory](https://globalnews.ca/pages/feeds/) lists Canada, World and Entertainment feeds. [Corus terms](https://www.corusent.com/terms-of-use/) returned 403 here, so terms remain unverified. [Variety](https://variety.com/) has a reachable feed, but [Penske terms](https://pmc.com/terms-of-use/) were not retrieved. [CBC's directory](https://www.cbc.ca/rss/) and [terms](https://www.cbc.ca/aboutus/terms-and-conditions.html) were also unavailable to this review. None is commercially cleared by this catalogue.

The [Anishinabek Nation communications page](https://anishinabek.ca/departments/communications/) links Anishinabek News. This is a specific Nation/community perspective, not a proxy for all First Nations. Its feed was inaccessible from the workstation and syndication rights remain unconfirmed. [APTN terms](https://www.aptnnews.ca/terms/) restrict automated collection without prior consent; hold it for permission review rather than probing or enabling it. Its proposed `/feed/` path is unverified. APTN covers First Nations, Metis and Inuit peoples: keep those distinctions, and label broader Indigenous coverage honestly. Do not infer the owner's Nation, location, identity or political preferences.

[BBC's feed reference](https://support.bbc.co.uk/platform/feeds/NewsFeeds.htm) is old. The accessible [2022 BBC terms PDF](https://downloads.bbc.co.uk/usingthebbc/bbc_terms_of_use_31March2022english.pdf), sections 8 and 15, distinguishes RSS display from metadata/computer-analysis uses. Current applicable terms and permission for this service need confirmation. A working RSS response is not clearance for AI processing. This is a conservative product hold, not legal advice or a statement that the archived terms are current.

Rights review must resolve personal storage/display and automated acquisition before source activation. Excerpts, full text, provider export and commercial redistribution require separate decisions. Do not contact publishers or send permission requests as part of this change. Keep dated evidence and explicit owner decisions in a reviewed catalogue revision; then provision approved sources through the operator boundary. The offline schema plus duplicate checks pin these ten candidate identities/URLs and prevent claiming activation or source approval; the schema is not a generic runtime registry or a ten-source product limit.

## Feed selection and context

Provision tenant-owned saved feeds for Development, Entertainment, Canada, First Nations and World, plus an optional explicitly selected Mixed feed. Initial provisioning is operator-only; authenticated CRUD remains #22. Display a small selector in Claudriel, not a new dashboard. Each choice submits its saved `feed_id`; a label or topic never grants source entitlement.

The existing query schema already supports personal requests: keep `context` and a short explicit `intent`, use `technologies: []`, and optionally set focus topics. Do not omit required fields or invent a new query parameter. See [synthetic query shapes](../testdata/personal/queries.json). These shapes are not implemented HTTP endpoints. Private project context is optional input to a selected Development query, not a required ingredient of a personal news query. Switching to Canada must not forward PHP dependency lists. Selected-feed eligibility comes before contextual ranking; personal feeds cannot be suppressed merely because a coding project is active.

In #13, a broad personal intent with no technologies must retrieve recent eligible items from the saved feed, even if literal words such as "show recent news" do not match FTS. Use FTS when meaningful search/context terms exist; do not make PHP/development vocabulary a hidden precondition. Produce metadata-grounded recommendations with original links and publication/observation times. A headline is insufficient for detailed summaries, verified event claims or version applicability. The default remains deterministic while provider export is unapproved.

## Mixed digest and honest coverage

For an initial maximum-five digest, use explicit equal category preference across the five selected interests. Prefer one eligible item per category, stable ordering within each category, and at most two items from one publisher group. Global's three sections share the `corus` publisher group. No category is filled from an unselected source, and neither source count nor publisher sections imply editorial independence.

Emit each authorized item/canonical URL at most once across categories. Assign a multi-category item to one category deterministically. With missing/duplicate/category-cap-limited candidates, return fewer than five and explain the gap; do not silently fill First Nations coverage with generic national news. Only an explicit future preference revision may change balance or allow redistribution of empty slots. Content-based clustering across different URLs is future work; do not merge stories merely because titles look alike. Category balance, source selection and publisher limits belong to the saved feed/preference revision already represented in cache identity, never an untracked UI-only setting.

The current draft response has source-level coverage, not per-category coverage fields. #13/#16 must use existing warnings for missing categories initially or introduce any new structured coverage field through a separate schema change. Do not claim today's transport or query coordination already performs category balancing. At present First Nations acquisition is unresolved, Canadian coverage depends heavily on one reachable publisher, and world coverage is limited to English-language editorial perspectives. Report and resolve those gaps before calling the personal pilot representative or complete.

## One shared resource budget

Proposed ceilings, pending implementation and measured device validation:

- Preserve the bootstrap's existing five hourly schedules: 120 attempts/day at most. Ten additional candidates would each poll no more than every four hours, staggered: another 60. Total ceiling is 180 attempts per rolling 24 hours, not 180 per feed or tenant. All remain disabled. Source-specific stricter rates and cache/Retry-After instructions take precedence; this is not a breaking-news SLA.
- One concurrent fetch, 15-second deadline and 2 MiB maximum decoded response. Reserve remaining capacity before starting; enforce both wire and decoded/decompression bounds during streaming. Stop before exceeding a separate 64 MiB aggregate decoded-byte budget per rolling 24 hours, including failed/partial responses. The attempt cap alone could transfer 360 MiB; the byte cap deliberately prevents that worst case. Defer remaining work with a budget reason and stale coverage, never catch up in a burst after restart or raise caps automatically.
- Fetch once per authorized source/sharing boundary, then reference the data from several personal feeds. For this pilot reuse is within one provisioned tenant; cross-tenant sharing still needs explicit rights/entitlements. Cache lookups, panel changes and mixed-digest requests must not enqueue a fresh source fetch or multiply schedules. No automatic fetch retries, article traversal, provider calls or mandatory additional services.
- Propose a 256 MiB corpus-metadata/index allocation within the eventual whole-database/storage policy, not an implemented SQLite size limit. Account for FTS/index growth, WAL, snapshots, backups and free-space reserve separately; shared SD-backed storage is not dedicated Northway capacity. Bound per-poll item admission/history, avoid unchanged-item writes and stop admission before disk pressure. The 90-day corpus policy is an upper age limit, not a promise to fill the allocation or permission to delete saved evidence. #20/#36 must choose and test the final budget/recovery plan; these numbers are design ceilings, not measured requirements.

## Delivery and acceptance

#12 implements the generic collector using approved/synthetic fixtures; the five-source bootstrap is not a hard-coded service limit. #13 implements personal retrieval and mixed selection with tenant/cache boundaries. #19 adds the feed selector and minimal context handling. #20/#36 prove native workload/storage/recovery. Source-specific approval, permissions and failed endpoint checks must be resolved before those candidates are activated; #44 completes curation/design only, not permission acquisition or a working feed service.

[Synthetic non-PHP RSS and query fixtures](../testdata/personal/README.md) cover all interest areas without publisher corpora. Offline checks prove catalogue boundaries and input shape only. Runtime tests must prove denied entitlements, duplicate handling, absent publication dates, missing categories, stable publisher-balanced output, and no collection on panel switching. Evaluate a manually labelled sample separately in each category and the mixed digest; PHP-only relevance cannot pass the personal-pilot release gate.
