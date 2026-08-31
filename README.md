# Northway

**Contextual news for AI agents.** Give Northway a description of what you are working on; receive a small, relevant feed with source links, freshness information, and explanations grounded in the articles.

Claudriel is the first client. Northway is an independent service, not a Claudriel subsystem. Its eventual product is an agent-facing SaaS API.

> Status: foundation and design only. No server, deployed API, ingestion worker, or SaaS functionality exists yet. JSON examples are synthetic contract fixtures, not real news. Phase 0 establishes the foundation; Phase 1 is the first implementation target.

## First useful experience

While working on a PHP project in Claudriel, ask:

> Keep me informed about developments relevant to this project. Prioritize releases and practical engineering; skip beginner tutorials.

Claudriel submits approved technical context and a saved feed identifier. Northway retrieves recent articles from selected sources, ranks a bounded shortlist, and returns up to five recommendations with an explanation of relevance. Claudriel renders the feed and sends explicit feedback. It may use its own AI to further synthesize results, but a second client must not need Claudriel to obtain contextual recommendations.

## Design constraints

- One Go application with embedded SQLite/WAL/FTS5 initially; scheduled collection and HTTP serving share one process in deployment. PostgreSQL is a later option if measured needs justify it.
- Tenant ownership, scoped credentials, private cache boundaries, and usage budgets from the first usable release; self-service and billing later.
- Collect once, retrieve cheaply, and apply AI to a bounded shortlist. Cache by tenant, feed revision, corpus revision, context, and ranking version.
- Reuse selected North Cloud capabilities with provenance and tests. Do not import its deployment topology.
- Keep project files, secrets, and whole conversations out of requests. Treat retrieved content as untrusted data.
- Raspberry Pi first: approved publisher feeds and optional scheduled external AI search, no home crawling or local AI model. See the content-source policy for external acquisition modes.
- No Elasticsearch, message broker, browser farm, vector database, Kubernetes, or dedicated ML sidecars in the initial deployment.

## Start here

- [Phased roadmap](docs/roadmap.md)
- [Content sources and Raspberry Pi deployment](docs/content-sources.md)
- [Scheduled AI discovery and custom lists](docs/specs/scheduled-discovery.md)
- [Architecture and SaaS boundary](docs/architecture.md)
- [Draft API contract](docs/api.md)
- [Go package layout](docs/project-layout.md)
- [Dependency decisions](docs/dependencies.md)
- [Agent development harness](docs/agent-harness.md)
- [Requirements](docs/specs/requirements.md)
- [Data and transaction specification](docs/specs/data-and-transactions.md)
- [Security and operations](docs/specs/security-and-operations.md)
- [North Cloud migration ledger](docs/migration.md)
- [Evaluation and release gates](docs/evaluation.md)
- [Initial architecture decision](docs/decisions/0001-service-boundary.md)

## Repository checks

Requires Python 3.11+ for documentation/contract checks only; this is not the planned application runtime.

```sh
python3 -m pip install -r requirements-dev.txt
python3 scripts/validate_contracts.py
```

The checker validates request/response examples, rejects invalid contract cases, and checks local Markdown file links. CI runs the same check. It does not demonstrate a running service or tenant isolation in an implementation.

## Relationship to North Cloud

North Cloud remains the reference implementation during migration. Northway starts with a clean history and imports only selected, useful behavior. North Cloud will be archived only after consumer migration, data preservation, and explicit retirement approval. Creating this repository does not archive, stop, or delete North Cloud.

The repository starts private. No open-source license is granted by this initial commit. Review first-party ownership, third-party notices, source-content rights, and the intended business model before making it public or redistributing imported material.
