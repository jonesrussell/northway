# Personal source selection: 2026-08-31

The owner explicitly approved the ten feeds presented in the conversation. This records exact source selection for a personal, metadata-only pilot. It does not grant publisher permission or activate collection. Catalogue schema version 2 pins this decision; replacing or adding endpoints requires a new reviewed owner decision. The five-source PHP bootstrap is unchanged.

| Interest | Selected source | Exact endpoint |
|---|---|---|
| development | InfoQ Programming | https://feed.infoq.com/programming |
| development | GitHub Blog | https://github.blog/feed/ |
| entertainment | Variety | https://variety.com/feed/ |
| entertainment | Global Entertainment | https://globalnews.ca/entertainment/feed/ |
| canada | CBC Canada | https://www.cbc.ca/webfeed/rss/rss-canada |
| canada | Global Canada | https://globalnews.ca/canada/feed/ |
| first_nations | Anishinabek News | https://anishinabeknews.ca/feed/ |
| first_nations | APTN News | https://www.aptnnews.ca/feed/ |
| world | BBC World | https://feeds.bbci.co.uk/news/world/rss.xml |
| world | Global World | https://globalnews.ca/world/feed/ |

## Boundaries

All sources remain disabled and planning-only. The selection covers proposed retention of source item ID, title, original URL, publisher, publication time and observation time only. No descriptions, article text, images or enclosures are approved. The shared cadence, request/byte ceilings and storage allocation remain proposals, not enabled schedules or measured Pi capacity.

No publisher outreach, paid service, article traversal, deployment, Pi SSH session, AI-provider export, commercial redistribution or source substitution is authorized by this record. Source selection, publisher-use clearance and operational activation are separate decisions. Existing activation prerequisites remain unchanged.

## Publisher guidance follow-up

Checked on 2026-08-31 using public publisher guidance, without requesting feed documents or article pages. No publisher content was stored in the repository or sent to an AI provider for ranking. This is an engineering gate record, not legal advice.

- **InfoQ:** [Terms](https://www.infoq.com/terms-and-conditions/) allow summaries with original links and disallow whole-work republication. Automated acquisition/storage applicability still needs resolution; this is not SaaS or AI clearance.
- **GitHub Blog:** [Platform terms](https://docs.github.com/en/site-policy/github-terms/github-terms-of-service) were accessible, but applicability and blog-specific content rights remain unresolved.
- **Variety:** [PMC terms](https://www.pmc.com/terms-of-use), dated August 21, 2026, were retrieved. Section 19 conditions feed use on attribution, direct links and preserving supplied content. Section 9 restricts automated acquisition and AI/algorithmic uses. The catalogue changes from `terms_unavailable` to a conservative `permission_required` hold: reconcile applicable feed permissions with this service's acquisition, normalization and ranking before activation. Do not infer clearance from RSS availability.
- **Global Canada, Entertainment and World:** [Corus terms](https://www.corusent.com/terms-of-use/) could not be retrieved. Publisher-use review remains unresolved for all three sections.
- **CBC Canada:** [CBC terms](https://www.cbc.ca/aboutus/terms-and-conditions.html) could not be retrieved. Rights and the historical feed timeout remain unresolved.
- **Anishinabek News:** Applicable syndication permission remains unconfirmed; no new permission evidence resolved the historical feed refusal. Its specific Nation/community perspective must remain distinct from broader Indigenous coverage.
- **APTN News:** Official [terms](https://www.aptnnews.ca/terms/) were available as indexed search text restricting automated collection without prior consent; the full page could not be retrieved. Keep the permission hold and unprobed endpoint status. APTN covers First Nations, Metis and Inuit peoples.
- **BBC World:** [Current terms](https://www.bbc.co.uk/usingthebbc/terms/) could not be retrieved. The earlier 2022 reference remains historical evidence, not current clearance; retain the permission hold.

## Remaining release work

[Issue #46](https://github.com/jonesrussell/northway/issues/46) remains open for publisher-use clearance, source availability, honest coverage and operational budgets. Existing workstation observations remain unchanged: CBC timed out, Anishinabek returned 403, GitHub Blog failed the conservative probe screen, and APTN was not probed. These are not Pi measurements. Do not bypass access restrictions or weaken XML/SSRF checks to get a successful response.

No source was activated or re-probed by this revision. First Nations acquisition remains unresolved and three Global sections share one publisher group. Selection approval alone does not demonstrate representative coverage or a working news service. Optional AI ranking (#15) is not technically necessary to retrieve real news; all recorded activation prerequisites remain in place.
