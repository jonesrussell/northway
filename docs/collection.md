# Creator collection: local first slice

Owner-authorized on 2026-10-01: selective reuse of the archived North Cloud
collection concepts in the new NorthCloud (Northway) Go service, with a FETDER
consumer. This is local implementation and fixture qualification. No live source,
service tenant, credential, schedule, publication or deployment is activated.

## Implemented boundary

One operator-admitted exact HTTPS source URL is a depth-zero SQLite frontier
entry. AddCollectionSeed is tenant-authorized, retry-safe and disabled/unapproved.
ConfigurePoll has an explicit html mode; existing news workers select feed mode
only. HTML claims share the existing serial admission, rolling 180-attempt/64-MiB
budget, conservative failed/crashed transfer charges, 2-MiB response cap,
15-second transport deadline and two-minute fenced lease. A ten-second global
host hold protects consecutive HTML requests. Source/item/version caps remain
bounded. Reconfiguration fences existing claims. No transaction spans network I/O.

PreviewAllowed is a separate operator display-rights decision, not permission to
cache, download or rehost. HTML acquisition requires operator-confirmed current
robots permission with an expiry no later than 24 hours; expired evidence blocks
claiming. This first slice does not automatically retrieve/parse robots.txt.
Real-source qualification and automated robots refresh are prerequisites to
activation; the fixture uses controlled bytes and makes no source HTTP requests.
No CLI exposes live collection or policy approval yet.

The transport reuses pinned-public-IP HTTPS, rejects mixed private DNS answers,
proxies, redirects, compression and automatic retries, and bounds headers/body.
The HTML extractor uses the maintained x/net tokenizer already in the module:
bounded tokens/attributes/JSON-LD work, sanitized bounded text, OG profile/image
metadata, ProfilePage/Person fields, VideoObject references, sameAs/rel=me links.
References never expand the frontier. No browser rendering or original media
download. Only narrow YouTube privacy-mode/Vimeo playback references are admitted;
unknown embeds become outbound links.

## Durable delivery contract

Collection items use stable source + kind + original-URL UUID identities, not
body-text hashes or usernames. Metadata changes append monotonically revisioned
observations; unchanged 200s and 304s do not invent revisions. 404/410 create
removal revisions for known items. Missing HTML metadata alone does not prove
item deletion. A source-wide removal may produce multiple bounded export pages.

CollectionBatch returns version, after, next and at most 100 ordered events.
The tenant comes from the operator principal. Sequence numbers may have gaps
because other tenants have events. Consumers commit events and cursor together,
replay from the last committed cursor, and must never jump to a later cursor
before processing its events. Export includes removal revisions, no raw HTML,
credentials, media bytes or ownership assertions. Per-source 5,000-item and
10,000-revision limits stop collection pending retention review; events are not
silently pruned underneath a consumer. Customer deletion explicitly clears these
tables and preserves acquisition budget accounting.

## Agent operations

- northway collection seed --database PATH --tenant UUID --source UUID --url URL --title TITLE
  admits one disabled seed. No scraped/model output may invoke it.
- northway collection status --database PATH --tenant UUID returns bounded counts.
- northway collection export --database PATH --tenant UUID --after CURSOR exports
  one replayable batch.
- northway collection fixture --database DIR/northway-collection.fixture.sqlite
  --input testdata/creator.html creates a fresh synthetic tenant/source, runs the
  real store/collector/parser flow on controlled bytes, disables the fixture
  source, and exports the first batch. It refuses an existing database.

These CLI operations call typed business services. The local operator already
owns the private database file; Open's exclusive lock prevents concurrent serve.
No browser management dependency. HTTP/MCP transport and a separate service-tenant
provisioning contract remain follow-up work, not an implicit sixth personal
workspace or permission expansion of feeds:read.

## Qualification and residual scope

Tests cover fixture extraction, preview permission, malicious references,
token/cancellation bounds, durable replay/restart, unchanged metadata, removal,
atomic rollback, wrong-tenant settlement, lease expiry, concurrent claims,
expired robots evidence and disabled/idempotent seed admission. Existing news,
transport and budget tests remain applicable to shared mechanisms.

The first consumer is FETDER's private observation review/media presentation.
Autonomous link following, arbitrary provider APIs, live state, automatic
publication, cache/rehosting, paid services and scheduling remain disabled or
unimplemented. Enablement needs an accepted source register, seed list, rights,
robots policy, budgets and separately authorized release.
