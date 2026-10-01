# Developer beta source register

Reviewed 2026-10-01. Russell delegated source selection. This records the selected
small catalogue and publisher evidence, not ownership of publisher rights.
Production activation remains part of the exact release approval.

| Source | Feed | Attribution and permission evidence |
| --- | --- | --- |
| Official Go blog | https://go.dev/blog/feed.atom | The Go Authors; [site copyright](https://go.dev/copyright) specifies CC BY 4.0 except separately noted material |
| Official Kubernetes blog | https://kubernetes.io/feed.xml | Kubernetes contributors; [website repository license](https://github.com/kubernetes/website/blob/main/LICENSE) is CC BY 4.0 |

Retain titles, canonical publisher links, publication/observation times and source
identity only. Do not retain feed bodies, images, enclosures or scrape article pages.
Titles may be stripped of markup/normalized; disclose that. Display attribution,
the original link (including author/notice information there), and a link to
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). No endorsement claim;
respect exclusions and takedown requests. These permissions do not extend to
arbitrary linked third-party content or other candidate feeds.

Fixed beta ceiling: five personal workspaces, two selected sources each, four-hour
poll interval, one serial acquisition globally. At most 60 normally scheduled
checks per day across five workspaces; conditional requests where supported,
no automatic retry. Existing durable global ceilings of 180 attempts and 64 MiB
per day remain hard stops, including failures and restarts. Each response is at
most 2 MiB. Full-feed responses can exhaust the byte cap earlier: show degraded
coverage rather than bypassing it. No paid services, general crawling or AI calls.

`NORTHCLOUD_CATALOGUE=developer-v1` explicitly enables this deployment-selected
catalogue and its serial scheduler. It is off by default and is incompatible with
the old single-pilot polling option. Workspace provisioning retries reconcile the
same immutable profile; changed existing profiles fail rather than being adopted.
No Pi profile is changed or silently migrated.
