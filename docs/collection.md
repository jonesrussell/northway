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
No browser management dependency. The opt-in HTTP adapter is implemented below; MCP/common framework integration
and production service-tenant registration remain follow-up work, not an implicit sixth personal
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

## Agent management integration boundary

The supported nonbrowser test path is the local operator CLI: collection seed,
status and export call Store.AddCollectionSeed, Store.CollectionStatus and
Store.CollectionBatch with a tenant-bound operator principal. Seed admission
never enables or approves acquisition. The fixture command exercises extraction,
fenced collection settlement, durable events and cursor replay without networking.


The opt-in HTTP adapter is implemented with --collection-api. It uses separate
nwa1 bearer grants, not nw1 feed keys, first-party browser assertions, or the
retired shared-secret MCP service. Existing RequireManagement, RequireOperator
and external feed scopes are unchanged. No remote issuance endpoint, operational
grant, or deployment enablement is included. The durable grant store contains
only SHA-256 digests of randomly generated 256-bit secrets; authentication checks
expiry/revocation on every request, during last-used settlement, and inside the
serialized seed mutation transaction. Grants are
tenant-bound, at most 24 hours, individually revocable, and capped at 100 records
per tenant (including historical records). Tenant suspension/deletion and the
shared 60 authenticated requests/minute budget apply to the adapter. Deletion
also erases grant records.

Product operation contract for the platform manifest:
- collection.seed: POST /v1/collection/seeds; scope collection:seed;
  input {id: UUID, url: exact HTTPS URL, title: string}; result
  {id, acquisition_changed:false}. New seeds are disabled/unapproved;
  retrying an existing seed preserves its current operator policy. Retry same
  ID/URL is safe; different
  tenant selection, enablement fields and changed source URLs are rejected.
- collection.status: GET /v1/collection/status; scope collection:status;
  no input; result {seeds,enabled,items,revisions}, tenant scoped.
- collection.observations: GET /v1/collection/observations?after=N; scope
  collection:observations:read; result the existing version-1 batch
  {version,tenant_id,after,next,events}, at most 100 events. Replays are deterministic.

Authority comes exclusively from the grant; no tenant input is accepted.
CollectionScopes are a distinct Go type, never added to existing Scopes.Valid.
The three Store methods authorize their exact capability themselves, allowing
CLI and HTTP/MCP transports to share policy. Collection HTTP auth cannot grant
feed reads, key management, source policy approval, collector execution,
scheduling, discovery expansion, or FETDER publication.

Local tests generate only ephemeral in-memory/test-database credentials.
Production setup requires explicit owner approval for the migration, adapter
enablement and trusted TLS ingress, and each tenant-specific grant's recipient,
exact scopes, label, lifetime and secure one-time handoff. An operator must use
GenerateAgentGrant/CreateAgentGrant and RevokeAgentGrant through the supported operator CLI described below. Existing production grants/scopes remain untouched.
The framework task owns common manifest packaging and MCP mapping; this product
supplies the domain contract and working HTTP adapter above.

## Completed operator grant lifecycle

northway agent-grant create --database PATH --tenant UUID
  --scopes collection:observations:read --label RECIPIENT --ttl 1h
  --output NEW_PRIVATE_FILE [--dry-run]
northway agent-grant revoke --database PATH --tenant UUID
  --grant-id NONSECRET_ID [--dry-run]

Both commands require exclusive offline database ownership, a provisioned tenant
and canonical scope/identifier input. Creation requires explicit TTL1m..24h and
a named recipient/purpose label. Dry-run validates without generating credentials,
writing output, inserting or revoking. JSON results contain status, operation,
tenant, nonsecret grant ID and approved metadata only. Creation uses exclusive
no-follow0600 output in an existing0700 directory, syncs file/directory before
digest insert and removes its new file on failed persistence. An interrupted
handoff can leave an inactive file; reconcile rather than overwrite or retry
blindly. Output failures after success retain the valid private credential.
Renew manually by creating a replacement in a new file, switching the recipient,
then revoking the prior ID. Test fixtures prove replacement remains active.

The HTTP batch now adds tenant_id derived from the authenticated principal.
Local operator exports omit this optional field. FETDER remote pulling requires
this field and persists its stream/tenant binding with observations and cursor.

## Framework manifest alignment

northway management inventory [--collection-api] projects the actual selected
local operation catalogue without opening storage or a listener. API route
bindings and required scopes use the same embedded owned contracts that the
HTTP registration consumes. Operator CLI results use their owned operation IDs.
The disabled API projection marks its operations planned. Staged companion:
docs/management/northway-api.management.json (enabled local composition).

This is product binding/schema evidence, not a framework conformance receipt.
Structural validation against the provided framework v1 management schema
passes. The Go API has no canonical framework site.yaml, and the control-plane
site manifest does not activate creator_collection. Canonical application/capability
adoption and source/site digest-bound ManagementInventoryInterface receipts await
the qualified framework cohort and owning audit task. No unqualified package was
installed and no authored companion is used as runtime proof. MCP execution,
remote issuance, scheduling and live acquisition remain absent.
