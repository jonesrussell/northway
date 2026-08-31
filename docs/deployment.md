# Deployment ownership and next implementation

Status: integration plan, not a deployed service. The owner identified waaseyaa-infra as the existing Pi deployment repository. That repository is private; host inventory, routing, credentials and operational details stay there. Making Northway public does not publish its deployment configuration or authorize public API ingress.

## Repository responsibilities

Northway owns the Go application, SQL migrations, application/container contract, health/readiness behavior, tests and portable backup interfaces. waaseyaa-infra owns the environment-specific service definition, persistent storage, resource enforcement, secret custody, proxy/network configuration, monitoring, backup integration and reviewed deployment procedure. Do not add a competing production deploy workflow to this repository.

Target one non-root linux/arm64 service with a persistent SQLite directory. Cross-compilation proves artifact construction; actual-device checks must prove execution, capacity and recovery. Confirm available shared-host headroom before choosing hard limits. No new domain, ingress, live provider credentials or separate database server is required to start local development.

The database backup path must support a Go container. Use a Northway backup command or a deliberately isolated compatible helper; do not install another application runtime solely to reuse a helper. Preserve WAL coherence, exclusive migration behavior and restore verification. App-image rollback cannot automatically undo a destructive database migration.

## Next implementation order

1. [#8 Go foundation](https://github.com/jonesrussell/northway/issues/8): one module, configuration, graceful shutdown, health/readiness, meaningful checks and reproducible ARM build. Dependency additions must be verified when installed.
2. [#9 sources and device constraints](https://github.com/jonesrussell/northway/issues/9): select the initial corpus and confirm documented versus live hardware capacity through the deployment owner.
3. [#10 SQLite](https://github.com/jonesrussell/northway/issues/10), then the linked identity/ingestion/retrieval work: persist a source item and return an authenticated, evidence-backed deterministic result. AI discovery is a separate bounded extension.
4. [#36 infrastructure integration](https://github.com/jonesrussell/northway/issues/36): prepare and validate a focused infra PR once runtime/storage/HTTP contracts exist. [#20](https://github.com/jonesrussell/northway/issues/20) proves actual-device resource and recovery behavior before the pilot.

The first useful vertical slice is a selected feed item persisted in SQLite and returned through an authenticated contextual query. Keep acquisition fixtures/local tests independent of live provider spending and production access. No service promotion, host inspection, secret read or deployment is part of this planning update.
