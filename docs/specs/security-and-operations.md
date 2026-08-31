# Security and operations baseline

Status: requirements with key lifecycle, principal/storage authorization and HTTP authorization wrapper implemented; see [identity](../identity.md) for actual coverage. Fetch, AI, rate/usage controls and deployment remain future work. Covers NW-002, NW-006, NW-008, NW-009, NW-010, NW-013 and NW-014.

## Trust boundaries

Untrusted parties/data include callers, supplied project context, source responses, redirect/DNS results, model output and external billing/webhook events. Privileged surfaces include tenant credentials, source credentials, database file access, provider secrets, build/release tokens and operator configuration. An agent's request is not authorization to change tenant, policy, sources or spending limits.

Phase 1 uses scoped service API keys over TLS: feeds:read and feedback:write initially; source/tenant provisioning remains operator-only. No API key or provider credential is shipped to the browser. Return 404 for unavailable or unauthorized private object IDs to avoid confirming their existence; missing/invalid credentials return 401. Redact secrets and sensitive context in errors and telemetry.

## Fetch safety

Use a controlled HTTP transport with no automatic credentials/cookies from unrelated sessions. Permit only approved http/https destinations, restrict outbound address ranges including IPv4/IPv6 loopback/link-local/private/metadata addresses, validate every redirect and resolved address, and prevent DNS-rebinding checks from diverging from the actual dial. Cap redirects, decompressed body bytes, time and concurrent fetches. Do not disable TLS verification. Test these guarantees before customer-controlled source configuration, and keep operator sources constrained too.

## AI safety and cost

Model calls have no execution tools. Only allowlisted candidate IDs and source references can survive validation. Truncate input deterministically with provenance; reject oversized context rather than silently expanding compute. Never execute text from articles or model responses. Record provider/model/prompt version, latency, token usage, fallback reason and validated outcome without raw secret-bearing payloads.

Enforce tenant/request/concurrency budgets before calling the provider. The per-process rate limiter is not a distributed quota. On provider uncertainty, reconcile reservations rather than blindly refunding or retrying. Declared degraded service is preferable to runaway spend or fabricated explanations.

## Operations and release

Explicit readiness depends on required storage/config; liveness does not claim source freshness. Track last successful fetch per source, oldest pending job, provider failure/fallback rate, quota denials and retention progress. Alerts should detect stuck work and failing backups, not just process uptime.

Run containers non-root, bind administrative/profiling endpoints privately, use read-only filesystem where practical and explicit writable data paths. Expose only the API through TLS; no public database or pprof ports. Configure pool/worker/log/storage limits. Pin release artifacts/images and keep a known-good rollback artifact.

CI uses least privileges and immutable action references. No deployment runs from untrusted PR code or pull_request_target checkout. Production deploy, secret access, paid resource creation, repository visibility changes and archival require explicit authorization. Keep build verification separate from deploy authority.

Before paid beta: security review, tenant export/deletion, source licensing/attribution decisions, provider retention disclosure, key rotation and incident process, restore drill and billing reconciliation. Do not imply compliance certification from checklists.
