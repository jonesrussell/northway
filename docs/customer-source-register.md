# Customer beta source recommendation

Reviewed 2026-10-01. Russell delegated initial source selection. This is a
documentation recommendation for the invitation beta, not activation evidence.
The [launch roadmap](launch-roadmap.md) owns scope and release acceptance.

| Recommended source | Feed | Publisher permission evidence |
| --- | --- | --- |
| Official Go blog | https://go.dev/blog/feed.atom | [Go site copyright](https://go.dev/copyright): site contents under CC BY 4.0 except separately noted material; code has a separate license |
| Official Kubernetes blog | https://kubernetes.io/feed.xml | [Official website repository license](https://github.com/kubernetes/website/blob/main/LICENSE): CC BY 4.0 |

Recommend a developer-news catalogue because these sources have explicit reuse
terms and a small, understandable scope. This is not general-news or Indigenous
coverage. The Pi's five-feed personal-use approval does not apply to customers.

Retain only item identifiers, titles, canonical links, publisher/author attribution
and publication/observation times. Do not retain feed bodies, images or enclosures,
or fetch article pages. Preserve supplied notices, identify title normalization,
link the original and [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/),
and make no endorsement claim. Respect separately excluded and third-party
material; the site licenses are not blanket permission for everything linked.

Before activation, verify the exact feed's available author/notice fields and
exclusions, document any attribution mapping, test the visible attribution and
API output, and record the reviewed source/profile revision. Honor takedowns,
publisher cache guidance and tighter publisher limits. Disable a source when
its necessary rights or attribution cannot be established.

Propose two sources per workspace, at most five initial workspaces, four-hour
polling and one global acquisition at a time, with conditional requests and no
automatic retries. These numeric limits remain proposals pending runtime and
owner acceptance. Enforce global request/byte caps independently of customer
count; exhausted budgets must produce visible stale/degraded coverage rather
than unbounded polling. Test failures, restarts and simultaneous tenants.

Synthetic fixtures are suitable for deterministic tests and must be labeled.
They do not demonstrate a live news service. Launch requires real-source
freshness, useful-item review and source-linked results under enforced limits.
