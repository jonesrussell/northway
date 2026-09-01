# Tenant identity and operator provisioning

Issue #11 implements key storage/lifecycle, authenticated principals, corpus authorization and a reusable HTTP authorization wrapper. Issue #16 now wires the real query, snapshot and feedback routes through that wrapper; see [the API contract](api.md). Key-management and signup HTTP endpoints remain unimplemented. The original identity probe tests remain as focused authorization tests, and product integration tests exercise the actual shipped handlers over real SQLite.

## Credentials

Service keys have the format `nw1_<32 lowercase hex lookup characters>_<43 base64url secret characters>`. The lookup handle has 128 random bits and is not secret; the independent secret has 256 bits from Go crypto/rand. Only a SHA-256 digest of the complete key is stored. This fast digest is appropriate for generated random keys, not human passwords. No caller-selected secret is accepted by operator commands. Comparisons use crypto/subtle over equal-length digests; syntax and lookup handling are not claimed to make the entire HTTP request constant time.

Migration 3 adds api_keys with tenant ownership, immutable scopes, digest, creation time, nullable last-used time and revocation time. No plaintext or recoverable encrypted key is stored. The service validates a strict canonical token, looks up its nonsecret handle, compares the digest, and conditionally records usage only while the key is still active. Missing, malformed, wrong-secret and revoked keys are unauthorized. Storage/cancellation failures are unavailable, never authorization success. Last-used metadata is monotonic even if the clock moves backwards. Each successful authentication uses one short serialized write; measure contention before changing this design.

There is no credential/principal cache. Revocation is checked for every new authentication, including a conditional write to close the lookup/revocation race. Requests already authenticated may finish; revocation does not cancel an in-flight operation. Keys currently have no automatic expiry. Rotate/revoke them manually; self-service lifecycle, rate limits and usage budgets remain separate work. Do not expose an unfinished API publicly based on this slice.

## Authority boundaries

| Capability | Authority |
|---|---|
| feeds:read | Read tenant-owned feeds/articles and bounded FTS candidates |
| feedback:write | Reserved for the feedback service; does not permit corpus or key writes |
| Local operator principal | Provision or mutate corpus/keys inside one explicit tenant |

Scope bits are persisted in a schema CHECK constraint; adding a scope requires a forward migration and upgrade/authorization tests, not just a Go constant.

Principal has private fields and a deny-all zero value. Corpus methods accept it explicitly and derive tenant scope from it. A raw TenantID, request field, JSON object or model output cannot serve as an authenticated principal. Malformed or unavailable private read IDs produce the same storage not-found error; trusted operator writes retain useful UUID-validation diagnostics. Missing identity and insufficient scope fail before accessing corpus SQL. Existing composite tenant foreign keys remain in force.

The only lookup without an already known tenant is the narrow credential-handle lookup used by identity.Service. It returns key metadata, never corpus data. Usage updates and revocation are tenant-scoped. Tenant creation itself is an operator-only bootstrap seam, not a public endpoint. Generated SQL remains confined to the SQLite package.

identity.Operator is an explicit trusted-code escape hatch for local provisioning and internal jobs, not authentication. Never call it with a tenant taken from HTTP, model output or another untrusted payload. File-access operators already have access to every tenant; SQLite and Go package boundaries do not defend against malicious trusted application code. The publisher scheduler accepts one canonical tenant only through trusted startup configuration, carries that principal through the serial loop and rechecks persisted source policy on every database claim; it never derives authority from a request or source response. Future jobs must derive tenant authority from their persisted, validated owner, carry it through batches, and recheck policy when starting work. Do not retain a service principal across requests as an authorization cache. Any batch endpoint or customer-authored schedule must extend the isolation matrix when implemented.

## HTTP integration contract

httpapi.Require authenticates one Authorization header, rejects ambiguous/duplicate bearer credentials, enforces the route scope, and passes a principal explicitly to the handler. Only the header is a credential source; query strings, body fields and tenant headers confer no identity. The wrapper bounds authentication to one second within the request context. Responses use no-store; authentication errors match the problem schema with a fresh UUID request ID, fixed public text, and 401/403/503 semantics. Private object endpoints must map the adapter's not-found error to 404 without confirming another tenant's object. Test-only HTTP handlers exercise that mapping with two tenants, shared IDs and private IDs.

TLS termination and access-log redaction belong to the trusted private reverse proxy in waaseyaa-infra. Service keys remain in the client backend, never the browser. Do not accept credentials over an externally reachable plaintext connection or put them in URLs. This change does not configure production TLS, proxy trust, CORS, rate limiting or deployment. Health endpoints remain public operational checks and reveal neither keys nor corpus contents.

## Local operator commands

Stop serve before provisioning, rotation or revocation: all commands use the same exclusive database ownership lock. Run migrations first, and use the same service UID and private local storage. Example IDs below are synthetic; no commands below fetch content or deploy anything.

```sh
northway migrate --database ./data/northway.sqlite
northway tenant create --database ./data/northway.sqlite --tenant 00000001-0000-4000-8000-000000000000
mkdir -m 700 ./credentials
northway key create --database ./data/northway.sqlite \
  --tenant 00000001-0000-4000-8000-000000000000 \
  --scopes feeds:read --output ./credentials/claudriel.key
northway key revoke --database ./data/northway.sqlite \
  --tenant 00000001-0000-4000-8000-000000000000 --key-id NONSECRET_LOOKUP_ID
```

Tenant provisioning is safe to repeat and retains the existing tenant state. Credential issuance is intentionally once-only: a repeated deployment must reuse the existing credential or rotate it deliberately.

Use only needed scopes; explicitly add feedback:write when that client needs the future feedback service. New credentials are written once to a newly created 0600 file in an existing 0700 directory. Existing files, destination symlinks and public immediate parents are rejected. Ancestors must be operator-controlled. No secret is printed, accepted in a flag, or returned through HTTP. Standard formatting/JSON of the internal Secret type is redacted; Reveal is an explicit private-file handoff operation. This does not erase immutable Go strings from memory or protect against a debugger/file-access operator.

The file and directory are synced before inserting the digest. A crash in between can leave an inactive credential file. This is not a transaction across filesystem and database: reconcile uncertain outcomes before retrying, and never overwrite an existing key file. Provisioning failures attempt to remove the newly created file; if removal fails, a redacted cleanup warning tells the operator to inspect the requested output path before retrying. Existing files are untouched. A stdout failure after successful creation does not remove the credential. stdout contains only status, tenant ID and the nonsecret lookup ID. Move the credential through your secret-management process into Claudriel's backend; keep it out of source control, terminal transcripts, screenshots and browser code.

Rotate by creating a replacement, moving the client to it, then revoking the old handle. Revocation is idempotent within the owning tenant. Wrong-tenant and nonexistent handles produce the same public operator error. Current local commands require a maintenance window; live administrative APIs are deliberately not introduced here.

## Verification and limits

`make check race vuln` includes key format/redaction, invalid scopes, JSON principal forgery, every corpus entry point, read/write separation, wrong-tenant access, sequential batches/asynchronous calls, revocation races, cancellation/storage failure, rotation/restart, key-free SQLite/WAL files, and populated schema-2 upgrade tests. HTTP tests use httptest's real server and SQLite, including duplicate headers, same-ID tenant isolation, private-object 404 equivalence, FTS isolation, tenant-override attempts, revocation and redacted 503 errors. Operator tests exercise repeat tenant provisioning, private file mode, no overwrite, symlinks, failed provisioning cleanup and cross-tenant revocation. Native process smoke tests also exercise provisioning and revocation.

No additional Go module was added. Schema 1→current and 2→3 upgrades are tested; backup/restore, actual-device disk-full/power-loss, rate/usage budgets and the deployment gates remain future work. Before a public product API or new job/cache path ships, exercise its actual routes and state transitions with two tenants; these foundation tests do not establish correctness of code that does not yet exist.

References: [Go cryptographic randomness](https://pkg.go.dev/crypto/rand), [constant-time comparison](https://pkg.go.dev/crypto/subtle#ConstantTimeCompare), [OWASP REST security](https://cheatsheetseries.owasp.org/cheatsheets/REST_Security_Cheat_Sheet.html).
