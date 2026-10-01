# Site contract

This application is governed by `.waaseyaa/site.yaml`.

Before changing application behavior:

1. Read the capability manifest and its active, planned, and excluded decisions.
2. Use the selected first-party Waaseyaa recipes and extension points.
3. Run `tests/Architecture/SiteContractTest.php`.
4. Run the strict site diagnostics.
5. Run `bin/maintenance/site-verify` without network access.

Generated files are owned by `.waaseyaa/generated.json`. Regeneration refuses edits outside the extension region below.

<!-- waaseyaa:extension:start local-guidance -->
# NorthCloud customer application

Use Waaseyaa accounts, authenticated sessions, CSRF and supported extension points.
Read ../docs/beta-implementation.md. The Go service owns all news and API keys.
Never mount/open its SQLite file in PHP. Workspace identity derives only from the
authenticated account UUID, never browser input. No service signer or assertion reaches
browser code. One-time external key reveal must be no-store and never persisted.

Keep public signup and paid features disabled pending explicit product choices.
Use candidate-local Composer dependencies. No donor vendor tree or autoload shim.
Run focused PHP tests, site contract/strict diagnostics and real browser/API tests.
No PRs: locally qualify and independently review immutable changes before direct
landing. Production credentials and DNS/TLS/ingress need exact approval.

<!-- waaseyaa:extension:end local-guidance -->