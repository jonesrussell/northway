# Customer beta verification checkpoint

2026-10-01, approximately 04:36 UTC. Local reviewed application candidate:
`814432f6feb36e5efd3fb1445a8f7dae89ecc08c`, on `northcloud-beta`. Not pushed or
deployed. Remote main remains documentation commit `6af49bd`.

## Passed evidence

- Full Go suite, including five-workspace ceiling, catalogue retries, durable
  assertion replay, scoped key expiry/revocation, budgets and tenant isolation.
- Offline support refuses live database ownership; suspension removes polling
  eligibility; deletion preserves other workspaces and anonymous acquisition
  charges; a tombstone prevents automatic recreation.
- Strict Waaseyaa site diagnostics and seven PHP tests / 34 assertions, using
  the candidate's own exact Composer lock on a Linux filesystem.
- Real four-worker browser journey: two operator-verified invited accounts,
  simultaneous single-use invitation consumption, sessions/CSRF, live selected
  Go/Kubernetes metadata, feedback, one-time API key reveal, independent HTTP
  client and revocation, third-browser recovery and old-session denial.
- Unsupported framework 2FA/OIDC/admin routes are denied before routing. Recovery
  login/stamping shares the registration/reset lock; independent review approved
  this security/lifecycle slice at the above immutable commit.
- Populated private account/workspace export and offline suspension/deletion
  passed after the real browser journey, preserving the other workspace.
- Local API/PHP/web images build. Their temporary tags/revisions are diagnostic
  candidates, not qualified immutable release artifacts.

## Reproduced production bootstrap blocker

`scripts/smoke_beta_containers.py` uses disposable names/volumes, proposed runtime
caps and production configuration. The maintained `db:init --no-sync-schema`
prepares schema, but `install:init` then fails before activation:

```text
ConfigurationAuthorityUnavailableException:
Active configuration generation is unavailable; apply the CFG-02 activation migration and bind its authority.
```

Value-redacted trace identifies pinned framework `0.1.0-alpha.299`:

1. `ConfigurationAuthorityServiceProvider::capabilityDeclarations()` calls
   `requireActiveGenerationId()` for production (line 214).
2. `ProviderRegistry::discoverAndRegister()` validates capabilities even during
   `ConsoleKernel::bootForSchemaSync()` restricted discovery.
3. That prevents `InstallInitHandler` from reaching its supported genesis
   activation, which is supposed to precede normal runtime consumers.

The existing desktop framework checkout at `75388b2` contains the same check.
No vendor patch, development-mode production boot, runtime check bypass or
manual configuration-generation row was applied.

Recommend a focused framework fix: capability declaration must remain valid
during restricted installation discovery, while normal production boot must
still require the active generation. Prove fresh production installation,
idempotent installation, uninitialized ordinary startup refusal and unchanged
read-authority enforcement. Consume only a reviewed exact framework release;
then repeat packaged acceptance and combined backup/restore.

## Production status

No NorthCloud containers, credentials, DNS, TLS or Pi changes have been applied.
The concrete Intersnipe proposal is in the deployment checkout's
`docs/northcloud-beta-approval.md`: existing host `167.99.230.155`, 416 MiB total
memory cap, loopback ports 38034/38035, apex/API stale-record replacement and
Certbot TLS. Explicit scoped approval is pending. Target co-load, off-host backup
custody/restore and real customer acceptance remain gates, not completed tests.
