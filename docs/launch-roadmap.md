# NorthCloud product launch roadmap

Updated 2026-10-01. Status: owner-directed product plan, not implementation or deployment approval.

## Decisions and scope

The product is **NorthCloud**, at **northcloud.one**: contextual, source-backed news feeds for people and their agent clients. The existing `jonesrussell/northway` repository supplies its Go foundation. Repository/module/binary renaming is separate work; existing identifiers remain valid until a reviewed compatibility change.

Deploy the product on **Intersnipe infrastructure**, following GoFormX's separation of a customer control plane and Go API. Claudriel is an optional client, not the required host, identity system or UI. Retire the Northway Pi deployment only after migration, target acceptance and a rollback window. This retirement decision covers Northway alone, not the Pi or other workloads.

Owner clarification, 2026-10-01: the original NorthCloud was a different product, was archived, and its old server is gone. Russell describes Northway as the forked, pruned and tuned pivot that now forms the new NorthCloud at northcloud.one. Treat remaining DNS pointing at the old server as stale cutover records, not evidence of an active predecessor to migrate. No further legacy-product dependency investigation is a launch prerequisite. Preserve historical code/import provenance without importing the old deployment topology. The current Northway Pi pilot is separate and must still be preserved until replacement acceptance.

This roadmap supersedes the Pi-first delivery order in the [historical roadmap](roadmap-pi-history.md) and historical issue milestones. Existing security, content-rights and data-preservation requirements remain applicable. Issue checklists are evidence and implementation inputs, not proof that this new launch has passed.

## Evidence baseline, not current production verification

| Component | Inspected baseline | Meaning |
| --- | --- | --- |
| Go foundation | `377d00696d6597b646e4e48a80ab948a05f3f361`, September 1 | Desktop WSL checkout matched GitHub main and was clean during reconnaissance |
| Pi application | `2035fac064b97b49196c5682aa000f675a56bf8f` | Recorded successful private deployment; current running state must be verified before migration |
| Claudriel | `31840e0fdd2330d6dc8a81d78e4105dfeedc9734` | Owner-allowlisted backend adapter, not customer onboarding |
| GoFormX API reference | `b5308f5c2a6df7d86506a4adca21f5b2bfab4e1d` | Recorded deployed revision; still in open PR #204 when inspected, not equivalent to main `fd9cc3f6db9b49e5597b355bfffce09f4c5fb0ab` |
| GoFormX control-plane reference | `bb384f90cf995e1165cc57e121ebc4347a8913b2` | Recorded deployed revision; later PR heads are not deployment evidence |
| Intersnipe infra | `7afd21fb7268b813882d4e7730f2331bd9543f52`, September 27 | Existing shared-host infrastructure; no NorthCloud deployment definition established by inspection |

The Pi [capacity/recovery gate #20](https://github.com/jonesrussell/northway/issues/20) records native execution, bounded load, coherent backup and off-device restore. It also records SD-backed storage and an automated offsite-backup gap. The [pilot #21](https://github.com/jonesrussell/northway/issues/21) was paused by the owner on September 1 after one evidence day and zero effective owner judgments. Calendar time does not complete usefulness acceptance. [Source gate #46](https://github.com/jonesrussell/northway/issues/46) remains open; versioned-profile support did not approve or activate CBC Indigenous.

Reconnaissance read source, repository state and recorded evidence. It did not establish current live health, publisher terms, customer rights, available host capacity or backup freshness. Recheck exact commits before implementation. Preserve pilot data; no greenfield assumption or destructive bootstrap is authorized.

## Ownership and proposed topology

| Owner | Responsibility |
| --- | --- |
| This Go repository | Canonical OpenAPI, tenant-owned feeds, approved-source entitlements, ingestion, ranking, snapshots, feedback, API credentials, quotas and data lifecycle |
| Proposed separate NorthCloud control-plane repository | Human accounts, sessions, workspace membership, onboarding, reader, key management, usage, recovery, public site/docs and operator UI |
| `jonesrussell/intersnipe-infra` | Environment-specific deployment, routing/TLS, secret custody, resource enforcement, backups and monitoring |
| `jonesrussell/waaseyaa-infra` | Pi writer freeze, migration evidence and eventual Northway-only retirement |
| Shared Go authentication module, proposed | Narrow assertion/JWKS verification mechanics; no product ownership or database access |

Recommend Waaseyaa/PHP for the control plane, using GoFormX's reviewed account and API patterns. This technology and the new repository name remain proposals. Go owns the business contract; PHP resolves membership and calls that API, never mounts its SQLite database or stores a second authoritative feed corpus. External clients use the same business services through scoped credentials.

Use a dedicated Compose project, network, volumes, secrets and loopback ports behind Intersnipe's host ingress. The existing GoFormX host is a candidate, not a selected or capacity-qualified target. Intersnipe infrastructure documents server ownership by Paul and administration by Russell; confirm the hosting arrangement and change scope. Keep exact hosts, addresses, port reservations and credential locations in the infrastructure repository.

Retain one Go process and SQLite initially, subject to measured concurrency, storage, recovery and availability requirements. A control-plane account store is separately owned. Do not add PostgreSQL merely to copy GoFormX; do not split database writers across hosts without a qualified storage design. `northcloud.one` is the chosen product domain; API subdomain, canonical browser origin, DNS routing and certificates still need an exact reviewed cutover plan. The owner has resolved the old-product dependency concern; existing old-server DNS is stale, not a migration source.

## Smallest customer product

A customer signs in, obtains one workspace through retry-safe provisioning, selects approved feeds, reads bounded source-linked results with freshness/coverage warnings, explicitly refreshes and records feedback, creates a least-privilege API key once, uses an independent API client, and revokes that key. Provide usage/limits, supported account recovery and an export/deletion path. The journey must work without Claudriel.

Recommend an invitation-based customer beta with a public explanatory site. Invite versus open signup, verification/recovery policy, free versus paid launch and team scope require owner decisions. Do not inherit GoFormX's unverified-login, mail-deferral, greenfield, host or credential choices.

Defer arbitrary/private source URLs, article extraction, general crawling, paid AI discovery/ranking, broad MCP compatibility, team administration and billing unless explicitly selected. Keep deterministic ranking. Recorded feedback does not currently train or alter deterministic ranking, so do not advertise learned personalization. Commercial or multi-user source permission must be established independently of personal-pilot approval.

## Required product gaps

- Current HTTP supports query, snapshot and feedback, not online tenant/key lifecycle or feed management. Offline provisioning takes exclusive database ownership; signup must not shell out to it or stop all customers' service.
- Implement idempotent account-to-workspace provisioning with explicit reconciliation of partial cross-service failure. Browser tenant selectors are never authority.
- Add online scoped key issuance, metadata inventory, rotation/revocation and an expiry policy. Keep first-party assertions and external keys separate. Keys appear only in an explicit one-time owner reveal, never logs or routine reads.
- Replace single-configured-tenant polling with bounded fair scheduling and entitlement checks. Enforce global publisher-fetch caps as well as tenant limits; customer growth must not multiply acquisition unboundedly.
- Implement HTTP rate/concurrency controls, durable usage/quota enforcement and safe retry accounting. Reserved 429 documentation is not an implemented limiter.
- Add customer export/deletion, suspension, audit and support workflows; define retention across active data and backups. No cross-customer learning by default.
- Publish OpenAPI and prove an independent client. MCP is a later adapter unless selected, not a prerequisite for the first documented HTTP client.

## Sequence and objective gates

| Stage | Deliverable | Acceptance gate |
| --- | --- | --- |
| 1. Scope and baseline | Decisions, exact revision manifest, current infra/DNS baseline and Pi pilot preservation record | Named owners; current Pi data and clients accounted for; coherent pilot restore proven; stale DNS cutover scoped; unresolved choices recorded |
| 2. Shared-auth contract | [Extraction roadmap](shared-go-auth.md), fixed conformance vectors, product-specific profiles | GoFormX unchanged behavior first; both consumers reject cross-product credentials; real persistent replay/isolation tests pass |
| 3. Customer boundary | Control plane, workspace provisioning, scoped online management and OpenAPI | Retry creates one owned workspace; partial failure reconciles; two tenants cannot cross feeds, snapshots, keys, jobs, caches or feedback; session/CSRF and revocation work |
| 4. Sources and limits | Customer-permitted catalogue, fair polling, quotas, usage and retention | Exact source rights/attribution recorded; global and tenant budgets enforced under concurrency/replay; suspension stops work; source failures visible |
| 5. Local product acceptance | Browser and independent API-client workflow using isolated data | Customer reaches useful feed without Claudriel; replay, refresh, stale/error, one-time key reveal and revocation pass; account recovery/export/deletion exercised |
| 6. Release qualification | Exact-source images, infra/configuration diff, migration and recovery package | Tracked-source builds, immutable hashes/revision labels, dependency/secret-layer checks; fresh and populated lifecycle tests; no runtime build on shared production host |
| 7. Target qualification | Current capacity baseline, bounded co-load and offsite recovery | Agreed headroom/latency/disk thresholds pass with ingestion/query/backup; adjacent services unaffected; encrypted offsite restore recovers accounts, tenant mappings and Go data |
| 8. Staged cutover | Restricted target, final migration, routing/TLS and client switch | Separately authorized exact package; one writer per migrated dataset; target readback and TLS/renewal pass; rollback remains viable and current writes preserved |
| 9. Customer launch acceptance | Real customer journey, public docs/site and observation | Russell plus a distinct tenant pass deployed journey/isolation; manually reviewed useful-item samples, freshness, failures and operator effort support claims; no mock-only acceptance |
| 10. Pi retirement | Scoped cleanup and preserved recovery record | Migration accepted; agreed rollback window complete; clients moved; explicit removal/data-disposition approval; other Pi workloads healthy |

Before implementing a stage, create a bounded local plan and map relevant existing issues (#22/#24/#25 for management/client safety; #26/#28 for lifecycle/rights; #27/#29 if charging). Historical prerequisite chains must be reconciled to this sequence, not silently treated as completed. No issue is closed by this documentation update.

## Operations and customer evidence

Record exact source SHAs, image/archive digests, target architecture, schema/profile revisions, key identifiers (not values), test results and configuration hashes. Separate source readiness, disposable rehearsal and actual deployed evidence. GoFormX's deployment authorization is not NorthCloud authorization.

Back up the Go store and control-plane account/membership mapping coherently, with separately protected key-recovery material. Require encrypted offsite retention, failure alerts and actual independent restore before customer acceptance. Choose RPO/RTO and acceptable backup interruption explicitly; daily backup and 24-hour RPO/4-hour RTO are discussion proposals, not approved promises. Never use a raw live SQLite volume copy as the supported backup.

Observe source last-success age, failures/backlog, request latency/errors, quota denials, SQLite contention/WAL, disk reserve, backup age, restart/OOM and certificate expiry. Redact credentials, contexts and private feedback. Define operator ownership, suspension/revocation, incident response, source takedown and account recovery. Shared analytics is optional and must not capture private feed/context data.

Public documentation must include a tested quickstart, credential scope/revocation, idempotency/retry rules, freshness limitations, quotas, privacy/retention, support and source attribution. Publish pricing only after its decision and enforcement. Do not claim learned personalization, source completeness, tested multi-harness support or an SLA from passing health checks.

## Pi migration and retirement

1. Inventory only Northway's service/volume, image/schema/profile, polling, clients, keys, monitoring, backup hooks, tunnels and local evidence tooling. Do not inspect or mutate unrelated workloads as part of cleanup.
2. Capture and independently restore a coherent snapshot; retain pilot feedback/evidence and map the existing tenant to Russell's new workspace through an explicit migration.
3. Freeze Pi Northway writes and polling for final transfer. Keep target polling disabled until migrated entitlements, counts and behavior are verified. Prevent dual writers and duplicate polling.
4. Switch clients and verify target source freshness, ownership, replay and feedback. Retain a stopped Pi service plus compatible image/data through an agreed rollback window; 14 days is a proposal, not an approved deadline.
5. After target writes, rollback must preserve those writes through a tested compatible reverse path or forward repair. Restarting an old Pi snapshot alone is not valid rollback.
6. After acceptance/window, authorize Northway-only service/artifact removal, obsolete monitoring/backup/tunnel entries and credential revocation. Review shared dependencies before removal.
7. Delete the Pi volume or retained snapshots only under the approved data-retention/disposition decision. Record remaining recovery artifacts and verify unrelated services remain healthy.

This plan is not authorization to stop the Pi service now, delete data, change DNS, spend money or deploy.

## Remaining owner decisions

1. Specific Intersnipe host/hosting agreement, capacity budget, canonical origin/API routing and the exact replacement of stale `northcloud.one` DNS records.
2. Control-plane stack/repository (Waaseyaa/PHP remains unanswered), invitation versus open signup, account verification and recovery policy. Initial public-site versus customer-beta sequencing also remains open; lineage clarification does not approve an implementation choice.
3. Customer source catalogue and rights, including whether Indigenous coverage is a launch requirement and its accurate labeling.
4. Free versus paid launch, quotas, retention, RPO/RTO, support and observation criteria.
5. Pilot tenant migration details, rollback window and final data disposition.
6. Shared-module name/owner/license and product assertion profiles; no repository rename or module creation is implied.

## Delivery workflow

For this owner-authorized roadmap work, use focused local changes, local checks and recorded review, then direct commit/merge and push. Do not create PRs or rely on GitHub Actions as the acceptance gate. Do not disable workflows or bypass repository protection. A blocked push is a blocker to report, not permission to weaken controls. Record actual checks, exact commits and remaining decisions; unrelated GoFormX application PRs remain untouched.
