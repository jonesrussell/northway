# Northway

**Contextual news for AI agents.** Choose a personal feed or provide approved project context; receive a small, relevant selection with source links, freshness information and explanations supported by available evidence.

Claudriel is the first client. Northway is an independent service, not a Claudriel subsystem. Its eventual product is an agent-facing SaaS API.

> Status: the local real-news path works end to end. A hash-pinned operator profile provisions five saved feeds and five publisher RSS feeds into SQLite; bounded polling has ingested real metadata, and authenticated deterministic queries return source-linked snapshots. The server can run that same bounded collector serially for one explicitly configured tenant; polling remains disabled by default and no Pi deployment is installed. First Nations coverage remains blocked by source access/permission, and AI-provider export and commercial redistribution remain disabled.

## First useful experience

Choose Development (including PHP), Entertainment, Canada, First Nations or World, or an explicitly selected mixed digest. Personal feeds work without repository context. The PHP sources are the technical bootstrap, not the product boundary. See [the broader personal-feed design and candidates](docs/personal-feeds.md).

For a project-focused Development query in Claudriel, ask:

> Keep me informed about developments relevant to this project. Prioritize releases and practical engineering; skip beginner tutorials.

Claudriel submits a saved feed identifier and a minimal explicit intent; technical context is included only when useful and approved for that query. Northway retrieves recent articles from selected sources, ranks a bounded shortlist, and returns up to five recommendations with an explanation of relevance. Claudriel renders the feed and sends explicit feedback. It may use its own AI to further synthesize results, but a second client must not need Claudriel to obtain contextual recommendations.

## Design constraints

- One Go application with embedded SQLite/WAL/FTS5 initially; scheduled collection and HTTP serving share one process in deployment. PostgreSQL is a later option if measured needs justify it.
- Tenant ownership, scoped credentials, private cache boundaries, and usage budgets from the first usable release; self-service and billing later.
- Collect once, retrieve cheaply, and apply AI to a bounded shortlist. Cache by tenant, feed revision, corpus revision, context, and ranking version.
- Reuse selected North Cloud capabilities with provenance and tests. Do not import its deployment topology.
- Keep project files, secrets, and whole conversations out of requests. Treat retrieved content as untrusted data.
- Raspberry Pi first: approved publisher feeds and optional scheduled external AI search, no home crawling or local AI model. See the content-source policy for external acquisition modes.
- No Elasticsearch, message broker, browser farm, vector database, Kubernetes, or dedicated ML sidecars in the initial deployment.

## Start here

- [Run and verify the Go foundation](docs/runtime.md)
- [SQLite storage, migrations and tests](docs/storage.md)
- [Run the local real-news pilot](docs/real-news-pilot.md)
- [Bounded RSS/Atom ingestion and operator command](docs/ingestion.md)
- [Tenant identity and operator commands](docs/identity.md)
- [Query transactions, caching and spending holds](docs/query-transactions.md)
- [Deterministic personal retrieval and metadata-grounded snapshots](docs/retrieval.md)
- [Delivery project and issue index](docs/delivery/README.md)
- [Deployment ownership and first implementation slice](docs/deployment.md)
- [Phased roadmap](docs/roadmap.md)
- [Content sources and Raspberry Pi deployment](docs/content-sources.md)
- [Personal feeds, broader source candidates and shared budgets](docs/personal-feeds.md)
- [Approved PHP bootstrap selection and device evidence](docs/pilot-readiness.md)
- [Scheduled AI discovery and custom lists](docs/specs/scheduled-discovery.md)
- [Firecrawl or North Cloud-derived external collection](docs/specs/external-collection.md)
- [Architecture and SaaS boundary](docs/architecture.md)
- [HTTP API contract](docs/api.md)
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

Requires Go 1.27.0 and Python 3.11+ for development checks. Python is not an application runtime dependency. See [runtime commands](docs/runtime.md) for builds, race checks and vulnerability scanning.

```sh
python3 -m pip install -r requirements-dev.txt
make check
```

The contract checker validates synthetic API examples and local links. Runtime checks cover lifecycle, credential/storage isolation, import boundaries and builds. CI runs the same commands. Tests establish only implemented paths; HTTP tests additionally exercise actual endpoint isolation and contract behavior; none of these establishes feed usefulness or Pi capacity.

## Relationship to North Cloud

North Cloud remains the reference implementation during migration. Northway starts with a clean history and imports only selected, useful behavior. North Cloud will be archived only after consumer migration, data preservation, and explicit retirement approval. Creating this repository does not archive, stop, or delete North Cloud.

The repository is public by explicit owner decision. No open-source license has been selected yet; public visibility alone is not a license grant. Review first-party ownership, third-party notices and source-content rights before redistributing imported material.
