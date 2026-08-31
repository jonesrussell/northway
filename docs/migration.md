# North Cloud migration ledger

Reference repository: jonesrussell/north-cloud. Audited reference commit: `51b877de7dab311c981dcdb4d38dfdca9965aeb1`.

No North Cloud implementation code has been imported in the initial Northway commit. North Cloud has not been archived or stopped.

## Import protocol

For each capability, record origin repo/commit/paths, why it is needed, third-party dependencies/notices and ownership/license review, changes made, behavior tests, resource impact and rollback implications. Prefer porting a bounded package or function into Northway's architecture. Use `git cherry-pick -x` only when a commit is actually self-contained and appropriate; otherwise include origin information in the commit body and ledger.

Do not import .env files, production data, personal documents, generated model artifacts or source corpora merely because they sit in the old checkout. No root LICENSE was found in the audited North Cloud tree; first-party ownership and third-party/source-content rights need explicit review before redistribution. Keep all applicable notices.

## Candidates

| Capability / reference paths | Target phase | Status | Required adaptation / proof |
|---|---|---|---|
| Conditional feed fetch: crawler/internal/feed/http_fetcher.go; feed/poller.go | 1 | Candidate, not imported | Extract from crawler bootstrap; persist poll state in Northway; 304, timeout, redirect and oversize-response fixtures. |
| Feed parsing and normalization under crawler/internal/feed/ | 1 | Candidate, not imported | Stable identity, URL canonicalization, duplicate syndication and changed-item behavior; no Elasticsearch writes. |
| Naming/dedup helpers under infrastructure/naming/ and crawler storage paths | 1 | Review before selection | Replace source-name-index coupling with stable source IDs; do not port create-if-absent semantics that suppress real updates. |
| Selected quality/topic rules under classifier/internal/classifier/ | 1–2 | Candidate, not imported | Keep only PHP/news retrieval value; create labelled evaluations; remove database-per-service and sidecar dependencies. |
| Source registry concepts under source-manager/ | 1–2 | Design reference | Northway owns sources added for its pilot; later registry cutover needs an explicit mapping and single writer for each migrated source. No dual authoritative registries for the same migration scope. |
| HTTP/checkpoint patterns in signal-producer/ | 2 | Reference only | Fix dense-lookback / 100-hit pagination and timestamp ties before reuse; never advance beyond undelivered accepted records. |
| Structured procurement parsers under rfp-ingestor/internal/parser/ | 4 | Parked | Add only for a client use case; closing-date changes/cancellations and rights; preserve existing Claudriel lead API until explicitly migrated. |
| Community/Indigenous taxonomy and alert-crawler/ | 4 | Parked | Preserve upstream taxonomy ownership and lifecycle semantics; do not promise emergency completeness; remove ES/Redis requirement if ported. |
| HTML extraction/browser rendering | 4 | Parked | Source-specific value, resource cap, safe fetch path and source policy review before enabling. |
| Publisher Redis routing and publish_history | None by default | Do not port unchanged | Existing Pub/Sub can lose offline-consumer messages; batch cursor advances despite individual failures. Prefer pull snapshots; add acknowledged outbox if push becomes necessary. |
| setup-ilm.sh, per-source raw/classified indices, Compose stack, five ML sidecars, monitoring stack | None | Retire or redesign | No bulk import. The audit identified ineffective retention setup and broad fixed dependencies. |

## Import record template

```text
Capability:
Northway issue/PR:
Origin repository / commit / paths:
Ownership and license/notice review:
Dependencies intentionally retained:
Old dependencies removed:
Behavior changed:
Tests and resource evidence:
Consumer/data cutover:
Rollback:
Status: candidate | imported | validated | cut over | retired
```

## Archive gate

The [roadmap](roadmap.md) defines retirement prerequisites. A new repository or successful personal pilot is not sufficient to archive North Cloud. Inventory remaining users, preserve data, verify replacement output, disable legacy writers/timers, retain a rollback window, and obtain explicit approval. Do not mechanically port every experiment just to empty this ledger.
