# Deployment ownership and cutover

Updated 2026-10-01. The [NorthCloud launch roadmap](launch-roadmap.md) owns current scope and gates. NorthCloud at northcloud.one targets Intersnipe infrastructure as an independent customer product. The existing Pi deployment is a migration source, not the launch target. This document authorizes no operational action.

## Ownership

The Go repository owns application behavior, tenant/resource authorization, migrations, health, portable coherent backup and the canonical API. A proposed separate customer control plane owns human accounts, sessions and workspace memberships; it calls the Go API and never accesses the Go database directly. Claudriel is an optional client.

`jonesrussell/intersnipe-infra` owns environment-specific service definitions, volumes, resource limits, secret custody, ingress/TLS, monitoring and backup scheduling. Keep private infrastructure details there. `jonesrussell/waaseyaa-infra` retains ownership of Pi freeze/retirement. Do not add a competing production workflow to the application repository.

## Release requirements

- Reconcile main `377d00696d6597b646e4e48a80ab948a05f3f361` with recorded Pi `2035fac064b97b49196c5682aa000f675a56bf8f`; verify current state before action. Preserve the existing pilot tenant and evidence pending explicit disposition.
- Select and qualify the actual host, architecture, resource/port budget and stale DNS cutover. Russell confirmed the original product was archived and its server is gone; an active-predecessor migration is not a launch prerequisite. GoFormX's deployment and historical capacity readings do not prove spare resources or authorize NorthCloud changes.
- Prepare exact source/image/schema/profile provenance, dedicated service/network/storage/secrets and a reviewed routing/certificate plan. Build from tracked sources off the shared production host.
- Prove customer authorization, quotas, source permissions and account recovery before public activation. Do not expose the local unauthenticated prototype proxy.
- Prove coherent offsite backups, account-to-tenant mapping recovery and compatible migration/rollback. SQLite backup uses the application-owned interface with exclusive ownership, not a raw live volume copy. Account data and recovery keys need separately protected custody.
- Review a concrete staged deployment package and receive applicable authorization before service, DNS, certificate, credential or paid-provider changes. After new writes, preserve them during rollback or forward repair; reverting an image does not reverse schema changes.

## Pi retirement

Follow the [migration and retirement checklist](launch-roadmap.md#pi-migration-and-retirement): inventory Northway dependencies, restore-test preservation, fence Pi writers/polling, transfer and validate, switch clients, observe the agreed rollback window, then approve scoped removal and data disposition. Keep other Pi services outside this retirement scope. The already archived original NorthCloud and its removed server are not this Pi pilot and require no new retirement work.

Historical actual-device evidence remains in [#20](https://github.com/jonesrussell/northway/issues/20), integration in [#19](https://github.com/jonesrussell/northway/issues/19), source holds in [#46](https://github.com/jonesrussell/northway/issues/46), and the owner-requested pilot pause in [#21](https://github.com/jonesrussell/northway/issues/21). These records do not establish current host health or a successful Intersnipe deployment.