# NorthCloud alpha.302 release qualification

2026-10-01. Local candidate on `northcloud-beta`, based on diagnostics commit
`2f170d6` and reviewed implementation `814432f`. No push, PR, framework patch,
framework publication, production deployment, credentials, DNS/TLS, or Pi change.

## Resolved production lifecycle failure

Reproduced the alpha.299 packaged failure in `install:init` after production
`db:init --no-sync-schema`. Checked published tags and the alpha.302 source
(`3f845363093a9c20588c8645e67ee61bef9e2e83`). Its kernel passes
`validateCapabilities: !$this->restrictedDiscoveryOnly` only for explicit
restricted discovery; normal production capability enforcement remains intact.
Consumed the exact published alpha.302 cohort: all 64 Waaseyaa runtime packages,
without local framework/vendor modifications or development-mode production boot.

The application now supplies the framework's required authentication eligibility
and internal-field services to its existing maintained controllers. Preflight
scans as `www-data`, which owns private SQLite storage; the root supervisor
publishes successful JSON atomically into root-owned `.waaseyaa`. Request workers
cannot modify that artifact directory. Failed scanning stops startup. The API
requires an 8 MiB `/tmp` tmpfs within its existing 128 MiB cap for SQLite
temporary maintenance work on populated/restored data; its root remains read-only.

## Executed local evidence

- `go test ./...`: passed on WSL/Linux. A first run overlapped Composer extraction
  and failed on a disappearing vendor directory; the completed-install rerun passed.
- `python3 scripts/qualify_control_plane.py`: candidate-local offline Composer
  install on a Linux filesystem, strict site diagnostics, architecture/golden-path
  checks and seven PHP tests / 37 assertions passed after the final auth wiring.
- `BETA_CHROMIUM=/home/fsd42/.cache/ms-playwright/chromium-1234/chrome-linux64/chrome
  python3 scripts/smoke_beta_browser.py`: passed two invited accounts, simultaneous
  single-use invitation, operator approval, sessions/CSRF, tenant separation,
  key reveal/revocation, live Go/Kubernetes metadata, feedback, password recovery,
  old-session/token-reuse denial, unsupported routes, account/workspace exports,
  offline suspension/deletion and preservation of the other workspace.
- `python3 scripts/smoke_beta_containers.py`: production pre-genesis refusal,
  fresh/idempotent installation, two-account registration/login, external key
  acceptance/revocation passed with API/PHP/web hard caps 128/256/32 MiB.
  Combined SQLite restore passed into independent volumes, including both
  logins/workspaces, live/revoked keys and discarded old sessions. Final
  immutable images repeat this test; the release bundle records exact IDs.
- Independent read-only review approved the application/packaging delta at patch
  SHA256 `4cc043a331a92ab17dbb8d16e11d5f9eda110ca1f42927a8074b8b9fc4076570`.
  Restore harness follow-ups preserve the same production source.

Windows-mounted site regeneration refused its publication identity check. The
supported generator was rerun in an isolated Linux copy with its own offline
dependencies, and generated artifacts copied back. No generator guard was weakened.

## Combined restore procedure

Stop only NorthCloud web, PHP and API writers. Take the Go supported offline
backup and a SQLite backup of the account database at this one quiesced point.
The account connection is read-only; its source volume permits SQLite's required
shared-memory sidecar. Restore into independent volumes with original private
ownership/modes. Run Go `migrate` before `serve` to restore required WAL/settings.
Preserve the separately protected original application/signing secrets; restore
no browser session files or old preflight artifact. Startup regenerates preflight.
Prove both logins and personal workspaces, retained live/revoked external keys,
and refusal of old browser sessions. The test uses only disposable fixture data.

This local rehearsal does not qualify off-host encrypted custody, deletion-ledger
application, RPO/RTO promises, or target capacity. Those remain activation gates.

## Target and approval boundary

Read-only recheck: inventory `fetder-droplet`, actual OS hostname `fetder`,
`167.99.230.155`; 1,140 MiB available RAM, 288 MiB swap in use, 33 GiB disk free;
loopback ports 38034/38035 unused. A snapshot does not establish co-load safety.
Both apex/API public A records still resolve to `147.182.150.145`, TTL 1800.
The complete requested production scope remains infrastructure
`docs/northcloud-beta-approval.md`. No target action is authorized by this report.

Release images must be built off-host from a tracked-source archive, with the
exact candidate SHA as revision label, then tested and exported with SHA256
archive checksums. The release bundle's manifest is the authority for immutable
artifacts and final packaged verification, not the earlier `local-check` tags.
