# Synthetic personal-feed cases

All titles, IDs, domains and queries here are authored test data. No publisher story or private project content is included. `topics.xml` covers all five owner-requested interest areas; the First Nations item deliberately lacks a publication date. It describes a fictional community programme, not a real Nation or an inference about the owner.

`queries.json` demonstrates the existing API shape without repository context: a generic explicit intent, empty technologies and a topic focus. UUIDs are synthetic saved-feed placeholders, not provisioned objects. The offline check validates shape and fixture safety only; no ingestion, entitlement, ranking, mixed-digest balancing, source freshness or UI behavior is implemented or proved here.

For #12/#13/#19 add real persisted-state and transport tests for: non-development items without technology context; no topic-based entitlement expansion; an explicitly multi-category item emitted once in mixed output; publisher cap preventing three Global sections from counting as three independent publishers; a missing First Nations source reported as a coverage gap; unknown date preserved; instruction-like title data never becoming instructions; partial/stale feeds; and no extra collection when switching panels. Label relevance by category rather than testing a ranker against its own output.
