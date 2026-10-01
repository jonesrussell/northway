# Invitation beta implementation contract

Started 2026-10-01 from `92d5742fe58180465994a0fac9e50ed509038d09`.
The owner authorized continuing implementation and deployment with the Waaseyaa
customer app. Production credential creation, exact DNS/TLS/ingress changes and
paid resources remain separately approved actions. The Pi remains unchanged.

## Bounded design

The initial candidate uses one verified human account and one personal workspace.
The immutable Waaseyaa user UUID is the personal workspace UUID; request fields
and browser selectors cannot choose it. Public signup and billing stay disabled
pending the invitation-beta decision. Customer source rights remain a launch gate;
tests use synthetic fixtures and do not establish live-news acceptance.

Keep the PHP application in `control-plane/` for atomic local contract development;
it has its own Composer lock, database, image and ownership. A later repository
split needs no runtime coupling. PHP never opens the Go database. This is a
packaging adjustment to the proposed separate repository, not a merged data plane.

Go owns online idempotent workspace provisioning, key lifecycle, feeds, queries,
feedback and tenant enforcement. Waaseyaa owns accounts, verified sessions, CSRF,
browser presentation and the private signing key. Reuse maintained framework auth,
not a second password/session implementation. Intersnipe owns deployment config.

## First-party contract

Use a NorthCloud-local Ed25519 assertion profile, `ncl-fpa+jwt` version 1. This is
not the shared-library extraction and does not change GoFormX. Pin verification
keys in deployment configuration initially; dynamic JWKS and independent library
publication remain later work. Key IDs, issuer and audience must be exact and
product-specific. Rotation uses an explicitly deployed bounded key set.

Claims are `iss`, `aud`, `sub`, `tenant`, `iat`, `nbf`, `exp`, `jti`, `op`, `ver`.
Subject and tenant are the same canonical UUID for this personal-workspace beta.
`op` binds the exact method and escaped path. Maximum lifetime is 60 seconds,
clock skew five seconds, one durable consumption per issuer/JTI before dispatch.
Reject invalid operation/profile before consuming replay state. Storage failure
denies access; rejected assertions never fall back to external API keys.

Management principal is distinct from the local operator principal. First-party
claims never construct `identity.Operator`. Existing `nw1_` keys cannot provision,
issue keys or gain arbitrary source/collection authority. One-time key responses
are no-store and returned only to the verified owner; stored credentials are hashes.

## Candidate acceptance

- Concurrent/retried workspace provisioning yields one personal workspace.
- Wrong product/key/profile/operation/tenant, duplicate JSON claims and expired
  assertions fail; same assertion across concurrent requests/restart succeeds once.
- Real file-backed SQLite tests cover tenant isolation, service-key denial of
  management, bounded key issuance, revoke/expiry and resource limits.
- Browser session fixation, CSRF, verification, account recovery and distinct
  customer journeys pass on candidate-local dependencies.
- Release tests include source entitlement, query/ingest quotas, backup/restore,
  export/deletion and adjacent-workload qualification before customer activation.

This file records a work contract, not completion. No customer or production keys
are created by implementation tests. No mock-only journey is launch evidence.
