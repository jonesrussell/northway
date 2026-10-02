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
