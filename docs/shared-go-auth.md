# Shared Go authentication adoption

Updated 2026-09-30. Proposed extraction and adoption, not implemented.

The canonical cross-product extraction plan is [GoFormX's shared Go authentication roadmap](https://github.com/goformx/goformx/blob/main/docs/shared-go-auth-roadmap.md). This page owns NorthCloud-specific integration gates and links it into the [launch roadmap](launch-roadmap.md).

## Boundary

Propose a small independently versioned Go module with assertion verification and JWKS key handling. Both applications import it; neither imports the other application. Keep it independent of Echo, databases, Waaseyaa and product domain objects. Module/repository name, owner and license remain decisions.

Northway `377d00696d6597b646e4e48a80ab948a05f3f361` currently implements `nw1_` service keys, two scope bits and private-field deny-all principals, but no first-party assertion adapter. The recorded Pi `2035fac064b97b49196c5682aa000f675a56bf8f` is a different baseline. GoFormX's source reference is `b5308f5c2a6df7d86506a4adca21f5b2bfab4e1d`, recorded deployed but still unmerged in PR #204 during inspection. Revalidate before coding; deployment provenance is not main-branch provenance.

## NorthCloud adoption order

This sequence governs shared-module adoption, not the entire NorthCloud launch.
A reviewed product-local assertion implementation may precede extraction, with
the same conformance, durable replay and tenant-isolation gates. Do not create a
generic platform or delay customer acceptance solely to publish a library.

1. Agree an explicit assertion profile: product-specific type/issuer/audience, key trust, version, scope registry, operation binding, claim syntax, TTL/skew and key-refresh failure policy. Shared code never implies cross-product trust.
2. Wait for GoFormX's behavior-preserving extraction and tagged conformance suite; pin an exact reviewed version, never a floating branch or production local `replace`.
3. Implement atomic durable replay consumption and bounded expiry cleanup in the existing SQLite adapter. Bind authenticated tenant identity into existing private-field principals; never construct the trusted local `Operator` principal from HTTP claims or payload.
4. Keep `net/http` middleware, canonical IDs, scope policy, resource predicates, API errors, source entitlements and quotas local. Extend the import-boundary allowlist intentionally rather than weakening it.
5. Integrate the customer control plane and test online provisioning/management through real HTTP and file-backed SQLite. Keep external API-key credentials distinct, with no fallback from rejected assertions.

## Acceptance

- Correct product signature/profile/operation succeeds; wrong product/issuer/audience/type/version/scope/tenant/operation fails. Wrong operation does not consume a valid replay ID.
- Concurrent use accepts one assertion only; replay survives process restart and multiple API workers if ever supported. Store failure denies access, never downgrades credentials.
- Rotation overlap, revoked-key tombstones, unknown-key fetch bounds, redirects, oversized/malformed JWKS and out-of-order refreshes retain the agreed behavior. Explicitly decide stale-known-key behavior rather than changing it accidentally.
- Two tenants remain isolated through actual HTTP, all tenant-scoped SQL/FTS paths, cache and background jobs. A transport principal cannot be forged from a browser tenant selector.
- Keep existing `nw1_` keys, revocation-race, redaction and zero-principal tests passing. Key expiry/online management changes require their own schema and compatibility review.
- Both product consumers pass a shared conformance suite; each keeps its own real-database replay and authorization tests. PHP signer fixtures must match the selected NorthCloud wire profile.

Do not extract API-key lifecycle, billing, ingestion, ranking, database migrations, backups, generic configuration/logging or product errors in this first slice. A bounded strict-JSON primitive can be considered later only after both products' differing decoding policies are documented and tested.
