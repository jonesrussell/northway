# Shared acquisition completion candidate

Owner request: complete all 88 reviewed sources as one job, without another ten-source delivery phase. [Specification](specs/NC-SHARED-001.md).

Reuses production shared PostgreSQL schema13 and polling from main72b87fb. Adds offline full-register activation, immutable identity/metadata/schedule validation, prior approval preservation, all-source onboarding eligibility and public operational outcome reporting. No new migration, database import, credentials, source definitions or capacity increase.

## Qualification

- make check: passed (contracts, formatting, vet, all normal tests, boundaries, generated SQL, license checks, both architectures and runtime/storage/identity smoke).
- PostgreSQL storage acceptance: passed, including all88 acquisition at zero workspaces, ten-to88 upgrade, idempotency,176 memberships/40 feeds for two workspaces and atomic combined-capacity rejection.
- PostgreSQL application acceptance: passed; populated import through actual CLI retains API snapshot parity, plus shared HTTP/key/snapshot isolation.
- Vulnerability scan: no vulnerabilities found.
- Independent focused review: approved after correcting capacity fixture to four valid hourly private sources.
- Final race results and immutable image identity are recorded with the release package.

The first database run used an incorrect broad test selector, exercising SQLite-only PRAGMA/trigger fault tests against PostgreSQL. Its temporary filesystem also exhausted WAL. It is not accepted evidence. Qualification reran the actual PostgreSQL-specific suites with a reset disposable loopback fixture; ordinary SQLite tests are verified by make check. No production database was used.

## Concrete release scope

Replace only the API image with this exact qualified candidate under the established deployment ownership/maintenance procedure. Retain schema13, PostgreSQL authority, private data, PHP/web images and their existing network composition. Run activate-catalogue with the owner's exact approval record, then resume existing public polling. Reconcile all88 via catalogue-status: identity,policy,last attempt/success/status/error and article count. Permit bounded native scheduling, never bypass host holds or provider denials.

If current policy conflicts or capacity refuses activation, stop without partial changes and report the exact conflicting public state. Do not revive denied sources. After all initial outcomes are recorded, verify public reader routes, private isolation, exact image and neighboring resource impact. Record failures truthfully; external source availability is not fabricated.

Production deployment requires specific approval for the resulting exact image and full-catalogue activation. Code recovery disables public polling or restores the compatible API image; retain post-cutover PostgreSQL writes and all existing data. No backups, new secrets, accounts/grants, whole-site redeploy or FETDER changes.
