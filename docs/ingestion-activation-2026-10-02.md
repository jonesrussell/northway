# Shared ingestion activation

Russell explicitly authorized enabling production ingestion on 2 October 2026.
This supersedes the previous disabled-release boundary. The reviewed shared
storage seam was not wired to a production loop. This slice connects it to the
existing serial scheduler with explicit `serve --public-polling`, valid only
for PostgreSQL shared-v1. No tenant identity, extra broker, worker service,
Elasticsearch, capacity increase, backup job or observations grant is introduced.

The `postgres activate-canary --database postgres:/private/connection
--approval-record owner-chat-2026-10-02` operator action requires paused serve,
validates the complete immutable ten-source set and remaining combined capacity,
then activates atomically. It records authorization in provenance and preserves
source-rights exclusions. Conflicting policy or a denied source refuses replay.
The reviewed_metadata decision concerns only the already documented linked-title,
publisher, original URL and date aggregation. Original publication rights remain
unassessed; no commercial license, full text, summary or media right is asserted.

Daily or slower cadence, guarded native fetching, global100 sources/180 attempts/
64MiB daily, host holds and conservative failed-attempt charging remain enforced.
HTTP401/403 settles accounting and disables that source transactionally, revoking
visibility. It is never retried through another route or revived by replay.
Public status reflects stale/failed metadata independently of customer count.

Before rollout: local checks, actual PostgreSQL activation/denial/atomicity tests,
independent immutable review, production package identity and bounded host checks.
Replace only the API image/command under the established maintenance lock. Keep
PostgreSQL, PHP/web, secrets and private data intact. Qualify live canary outcomes
and resource impact before any broader88-source activation. Disable public polling
through its explicit flag for code recovery; do not restore old SQLite data.

## Production receipt

Application718972102703368365ea404ad6802ccb6406d931 and reviewed infrastructure
1595a6b090f8605278acd0f93f43e22cdb59a649 landed directly on main. The API image
loaded on target is sha256:097fac3291ad5fcf75066b5967732cc78ff633e1bb6357a2892df9e3c5129ea3;
archive SHA2566f3b2693aa86b6fcf5aeae4c60e3a3c78bbf60a6934fd3f93457bfd4ba9d718c.
PostgreSQL remained unchanged; PHP/web retained their images and were rebound to
the replacement API namespace. No data import, new account, key or grant.

`make check`, vulnerability check, container smoke and PostgreSQL tests passed.
The parallel race suite had one transient existing TestHTTPContracts503 under
load; that test and the complete app race package passed in isolated reruns.
All other race packages passed the original run. Independent app and rollout
reviews approved exact executable candidates. No checks were silently skipped.

Initial post-restart public check returned502 before PHP/web were ready. The
read-only verifier subsequently accepted all five service routes200 and exact
container identities at2026-10-02T20:46:24Z. Ten enabled canary sources each
returned200:212 shared metadata articles,10 total attempts,0 pending,468941
charged response bytes. Daily cadence and78 disabled broad entries retained.
All four containers had0 restarts/OOMs. Samples: API6.801MiB/128MiB,
PostgreSQL49.92MiB/256MiB, PHP15.2MiB/256MiB and web4.938MiB/32MiB.
These are bounded release observations, not a long-term capacity guarantee.
