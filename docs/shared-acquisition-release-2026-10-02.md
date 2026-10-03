# Shared acquisition production completion

Owner-approved release, 2 October 2026 (Toronto). API commit `4ccd8e425972e5f07fdeeca75f01eed9253fb79d` is deployed on the existing NorthCloud host. [Job specification](specs/NC-SHARED-001.md).

## Result

All 88 reviewed definitions were activated atomically and have recorded initial outcomes. Final verification at `2026-10-03T02:45:48Z` (10:45 p.m. Toronto, 2 October): **83 successful feeds, 1,954 public metadata articles, 88 acquisition attempts, 18,865,135 charged bytes, zero pending fetches and zero active customer workspaces**. Shared acquisition operates without a fake tenant or customer account. Twenty topics remain represented. One denied source is disabled, leaving 87 enabled; four other failed feeds retain bounded scheduled retry. No source failure was concealed or bypassed.

| Publisher | HTTP status | Outcome | Policy |
| --- | --- | --- | --- |
| The Walrus | 403 | http | disabled after denial |
| Windspeaker | 0 | transport | bounded scheduled retry |
| The Narwhal | 200 | parse | bounded scheduled retry |
| ESPN | 202 | http | bounded scheduled retry |
| ESPN | 202 | http | bounded scheduled retry |

Status 0 for Windspeaker means a transport failure, not an HTTP response. The Walrus 403 stops future acquisition and visibility. Narwhal 200 failed feed parsing; ESPN 202 responses were not accepted as feed content. These are recorded external-source outcomes, not a claim that all publishers are currently available. No alternate endpoints or access-control workarounds were used. L'actualite host holds were honored until both remaining initial requests completed.

## Footprint and identity

Only the API image changed. PHP/web retained their images and were rebound to its network namespace. PostgreSQL schema 13, service image, persistent volume, private network and 256 MiB/.25CPU budget were preserved; API retains 128 MiB/.25CPU. No database migration/import, credentials/roles/grants, source cap increase, FETDER change or backup operation occurred.

Qualified local image: `sha256:bc00fe6ce4297e64c18383634b6c69b60201d636a6f168e567f188a6eac3a348`.
Target engine loaded image: `sha256:df8d37dd5cf4e7e43e21a9064d5e32465b919c76f2fb2ad2c04f00f1a78be811`.
The engines report different manifest/config identities; exact image Config and RootFS layer arrays matched before rollout. Both carry source commit 4ccd8e4. Archive SHA256: `2fb8772fcf08829c08b9c753e05592da79a75d1478d5408ee3003ba9e8b37cda`. Immutable broad register SHA256: `5e14f54cf9f0376689aa0b79c4bc41466bbbd25b1a138dfe0018e7a98d80a4c5`.

Global ceilings remain 100 combined source jobs, 180 rolling attempts/day and 64 MiB/day, one active fetch. Bounded sample after release: API 6.5 MiB/128MiB, PostgreSQL 42.3 MiB/256MiB, PHP 61.4 MiB/256MiB and web 4.5 MiB/32MiB. Host had 1064 MiB available memory and 29 GiB free root disk. These are release measurements, not a long-term capacity guarantee.

## Verification and recovery

All four containers running, zero restarts/OOM events. NorthCloud home/API health and neighboring FETDER/Intersnipe/GoFormX routes returned 200. Anonymous feeds and collection observations returned 401. Authenticated two-workspace retrieval/key/snapshot isolation, cache/revocation and deletion behavior were qualified against disposable local PostgreSQL; no new production account/key was created to repeat those fixtures.

Full checks, native PostgreSQL acceptance, populated migration API parity, normal and PostgreSQL catalogue race tests, vulnerability scan, exact-image CLI and non-root container smoke passed. Independent code review approved the immutable commit. Operational review approved helper SHA256 `ff928ffda2acbeeda537cc797682c83b4f2a8cd8770a5912330ee1ec78f3639c` after named ownership, timeout/kill/wait and recovery fencing fixes. The helper timeout paths also passed synthetic tests.

Production authority remains PostgreSQL. Compatible API recovery or disabling public polling preserves existing/post-cutover writes; never restore stale SQLite or interpret an activation client failure as transaction rollback. Existing saved customer preferences are preserved; new onboarding selects the full approved enabled catalogue. Publisher failure investigation is residual source maintenance, not unfinished shared architecture or another staged release.

Public operational receipts, per-source outcomes and qualification logs are retained in the task4 output directory; target receipts are `/opt/northcloud/shared-catalogue-release-receipt.json` and `/opt/northcloud/shared-catalogue-verification.json`. Neither contains publisher article text, credentials or private tenant records.
