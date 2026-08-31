# ADR 0001: independent contextual-feed service

Date: 2026-08-30. Status: initial design decision; implementation pending.

## Context

North Cloud grew into a broad microservice pipeline with many fixed dependencies. The desired immediate experience is personal contextual news in Claudriel. The longer-term destination is an agent-facing SaaS product named Northway.

## Decision

Create a new repository with a clean history. Implement one Go process backed by embedded SQLite with WAL and FTS5. This records the user-selected lightweight-first direction; PostgreSQL was considered but is deferred. Claudriel is the first client, not the owner of Northway's business logic. Northway ranks and explains relevant source-backed news from a small context envelope; the client owns private memory, context extraction and UI.

Version an HTTP/JSON contract first and add MCP as an adapter to the same domain logic. Include tenant ownership, scoped keys, budget enforcement and private cache isolation in the first implementation. Defer signup, billing, broad ingestion and distributed infrastructure until the pilot is useful.

Port selected North Cloud behavior with provenance. Do not fork the complete runtime or assume cherry-picking a commit will remove its service dependencies. Archive North Cloud only after a separate verified retirement process.

## Consequences

The initial build is slightly more work than a local script, but avoids making identity and cost controls a retrofit. SQLite avoids a database-server process and stores search, queue/checkpoint and metadata state together. It permits only one writer and has no row-level security; tenant-scoped queries and exhaustive negative integration tests are mandatory. Migration to PostgreSQL later is a SQL/data/operations change, not a driver switch. A provider abstraction allows the selected model to change without making clients depend on it; it does not require supporting multiple providers at launch.

Ranking quality, content rights and unit economics remain hypotheses to validate. The service will not know the user's project unless the client provides authorized context. No repository license or source-content redistribution rights are implied by this decision.

## Revisit when

Measured write contention exceeds the pilot latency budget, multi-host writers or stronger availability require a server database; ranking evaluations justify embeddings; customers need private sources; or procurement/community use cases justify importing another domain. Do not change the architecture solely because the product is called SaaS.
