# Delivery index

[Project](https://github.com/users/jonesrussell/projects/11) · [Milestones](https://github.com/jonesrussell/northway/milestones)

Issues own scope and acceptance criteria; native sub-issues and blocked-by relationships record dependencies. This index is navigation, not a second status tracker. Phases have no invented delivery dates.

## Views

- [All work](https://github.com/users/jonesrussell/projects/11/views/1)
- [Delivery](https://github.com/users/jonesrussell/projects/11/views/2)
- [Phase gates](https://github.com/users/jonesrussell/projects/11/views/3)
- [Personal pilot](https://github.com/users/jonesrussell/projects/11/views/4)
- [Ready](https://github.com/users/jonesrussell/projects/11/views/5)
- [Blocked](https://github.com/users/jonesrussell/projects/11/views/6)
- [Security](https://github.com/users/jonesrussell/projects/11/views/7)
- [Migration](https://github.com/users/jonesrussell/projects/11/views/8)

The milestone/phase views are tables rather than date-based roadmaps, because there are no agreed dates. Native view filters/columns are configured; custom grouping is not claimed.

## Work by phase

### [P0: Foundation](https://github.com/jonesrussell/northway/issues/1)

Reviewable architecture, SQLite/Pi constraints, source policy, API examples, executable contract checks and linked delivery plan. No runtime capability claimed.

- [#7 Publish architecture, contracts and delivery harness](https://github.com/jonesrussell/northway/issues/7) — NW-001 NW-011 NW-016; prerequisites: none.
### [P1: v0.1 - Personal Pi pilot](https://github.com/jonesrussell/northway/issues/2)

A contextual PHP feed in Claudriel; restart/restore and two-tenant isolation evidence on the actual Pi; two weeks of user review. Optional scheduled discovery has bounded cost. No home crawling.

- [#8 Bootstrap the Go application and enforce package boundaries](https://github.com/jonesrussell/northway/issues/8) — NW-011 NW-016; prerequisites: #7.
- [#9 Approve pilot sources and confirm Pi deployment constraints](https://github.com/jonesrussell/northway/issues/9) — NW-003 NW-013 NW-016; prerequisites: #7.
- [#10 Implement SQLite schema, FTS5 and safe connection lifecycle](https://github.com/jonesrussell/northway/issues/10) — NW-002 NW-010 NW-011; prerequisites: #8.
- [#11 Implement tenant-scoped API keys and object authorization](https://github.com/jonesrussell/northway/issues/11) — NW-002 NW-008 NW-013; prerequisites: #10.
- [#12 Port bounded RSS/Atom ingestion without home crawling](https://github.com/jonesrussell/northway/issues/12) — NW-003 NW-004 NW-013 NW-015 NW-016; prerequisites: #10, #9.
- [#13 Build deterministic contextual retrieval and grounded snapshots](https://github.com/jonesrussell/northway/issues/13) — NW-001 NW-004 NW-005 NW-006; prerequisites: #11, #12.
- [#14 Make query idempotency, caching and spend reservations atomic](https://github.com/jonesrussell/northway/issues/14) — NW-007 NW-008 NW-009; prerequisites: #11.
- [#15 Add bounded AI reranking with evidence checks and fallback](https://github.com/jonesrussell/northway/issues/15) — NW-005 NW-006 NW-008 NW-009; prerequisites: #13, #14.
- [#16 Implement query, snapshot and reversible feedback HTTP contracts](https://github.com/jonesrussell/northway/issues/16) — NW-001 NW-002 NW-004 NW-007; prerequisites: #13, #14.
- [#17 Evaluate and implement optional scheduled external AI discovery](https://github.com/jonesrussell/northway/issues/17) — NW-003 NW-005 NW-008 NW-009 NW-016; prerequisites: #9, #14, #12.
- [#18 Persist custom-list schedules and serve cached latest snapshots](https://github.com/jonesrussell/northway/issues/18) — NW-003 NW-007 NW-009 NW-016; prerequisites: #16, #17.
- [#19 Integrate a contextual news panel through Claudriel PHP](https://github.com/jonesrussell/northway/issues/19) — NW-001 NW-008 NW-012; prerequisites: #16, #15.
- [#20 Prove Pi resource limits, retention and off-device recovery](https://github.com/jonesrussell/northway/issues/20) — NW-010 NW-011 NW-016; prerequisites: #10, #12, #16, #9.
- [#21 Run the two-week personal pilot acceptance review](https://github.com/jonesrussell/northway/issues/21) — NW-001 NW-002 NW-005 NW-010 NW-012 NW-016; prerequisites: #19, #20.
### [P2: v0.2 - Agent developer preview](https://github.com/jonesrussell/northway/issues/3)

Two independent agent clients use documented HTTP/MCP contracts; feed management, revocation and tenant-negative tests pass. Manual onboarding only.

- [#22 Expose scoped feed and preference management](https://github.com/jonesrussell/northway/issues/22) — NW-001 NW-002 NW-007 NW-013; prerequisites: #21.
- [#23 Add the official MCP SDK as a thin application adapter](https://github.com/jonesrussell/northway/issues/23) — NW-001 NW-002 NW-009; prerequisites: #22.
- [#24 Publish OpenAPI and prove a second independent agent client](https://github.com/jonesrussell/northway/issues/24) — NW-001 NW-011 NW-012; prerequisites: #22.
- [#25 Review multi-tenant agent preview safety and usability](https://github.com/jonesrussell/northway/issues/25) — NW-002 NW-005 NW-008 NW-009 NW-013; prerequisites: #23, #24.
### [P3: v0.3 - Private paid beta](https://github.com/jonesrussell/northway/issues/4)

Measured unit costs, source rights, tenant lifecycle, billing reconciliation and recovery/security review pass before inviting paying partners. Public launch remains separate.

- [#26 Implement tenant lifecycle, data export and deletion](https://github.com/jonesrussell/northway/issues/26) — NW-002 NW-008 NW-010 NW-014; prerequisites: #25.
- [#27 Implement bounded plans and reconcile billing with actual usage](https://github.com/jonesrussell/northway/issues/27) — NW-007 NW-009 NW-014; prerequisites: #25.
- [#28 Review publisher/provider rights and customer data terms](https://github.com/jonesrussell/northway/issues/28) — NW-008 NW-013 NW-014 NW-015; prerequisites: #9.
- [#29 Hold the private paid-beta release gate](https://github.com/jonesrussell/northway/issues/29) — NW-010 NW-014; prerequisites: #26, #27, #28.
### [P4: Selective capability expansion](https://github.com/jonesrussell/northway/issues/5)

Import only customer-justified North Cloud behavior with provenance, rights, quality and resource evidence. PostgreSQL only when measured constraints justify migration.

- [#30 Decide whether measured load warrants PostgreSQL migration](https://github.com/jonesrussell/northway/issues/30) — NW-002 NW-010 NW-014; prerequisites: #29.
- [#31 Evaluate Firecrawl versus a North Cloud-derived external collector](https://github.com/jonesrussell/northway/issues/31) — NW-003 NW-005 NW-009 NW-013 NW-015 NW-016; prerequisites: #21.
- [#32 Select one North Cloud capability and port it with provenance](https://github.com/jonesrussell/northway/issues/32) — NW-005 NW-013 NW-015 NW-016; prerequisites: #29.
### [P5: North Cloud retirement](https://github.com/jonesrussell/northway/issues/6)

Consumers accounted for, retained data restored, cutover/rollback reviewed and obsolete services/credentials retired. Archive requires explicit approval.

- [#33 Inventory legacy consumers, deployments and retained data](https://github.com/jonesrussell/northway/issues/33) — NW-010 NW-015; prerequisites: #21.
- [#34 Prepare and rehearse North Cloud retirement cutover](https://github.com/jonesrussell/northway/issues/34) — NW-010 NW-015; prerequisites: #33, #32.
- [#35 Archive North Cloud only after approved retirement](https://github.com/jonesrussell/northway/issues/35) — NW-010 NW-015; prerequisites: #34.

## Verification and enforcement

The initial foundation checker verified synthetic schema/examples and local links. Runtime, SQLite and identity slices add lifecycle, real-file tenant isolation, migration/FTS/lock/failure tests, generated SQL checks, operator key lifecycle, HTTP authorization-wrapper tests and local container smoke tests. Current command evidence belongs to the linked issues/PRs and CI, not this navigation index. Source polling, paid provider search, product HTTP routes, Pi deployment and backup/restore remain future gates; the implemented foundation does not establish them.

Initial private-repository workflow runs did not start because GitHub reported an account billing/spending restriction. After the owner made Northway public, [a new hosted run](https://github.com/jonesrussell/northway/actions/runs/33348835800) started and revealed a pip-cache dependency-path error. The workflow now explicitly uses requirements-dev.txt. Check the current PR/run for hosted verification; neither Dependabot success nor the old billing failure establishes its current state.

The earlier private-repository branch-protection inspection returned HTTP 403. This setup has not configured branch protection or required checks. Establish and verify review/required-check enforcement in the runtime harness work; public visibility is not evidence that those controls are enabled. Do not weaken tests or install a privileged Pi runner to bypass failures.

Deployment belongs to the existing private infrastructure repository: see [the handoff](../deployment.md) and [#36](https://github.com/jonesrussell/northway/issues/36). This task is part of the personal-pilot milestone and a prerequisite for the actual-device operations gate.

No automatic merge, deployment, live billing, external messaging or North Cloud archival is configured.
