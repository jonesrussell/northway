# Repository development harness

Northway uses a portable, spec-driven repository harness. It is not a new agent runtime. Claudriel or another coding agent can drive the same issues, commands, contracts and review gates.

## Sources of truth

1. User-authorized scope and decisions.
2. Stable requirement IDs in docs/specs/requirements.md; ADRs explain architecture choices.
3. api/schemas and checked examples define the draft transport shape; docs/api.md defines semantics.
4. Feature specs and Go package/dependency decisions constrain implementation.
5. GitHub issues hold bounded tasks and acceptance criteria; milestones hold release gates; the Project displays delivery state.
6. Tests, CI artifacts and review evidence demonstrate implementation. A green checklist or agent summary alone does not.

AGENTS.md is the short entry point. Feature issues link only the necessary specs instead of copying a growing instruction book into every prompt. Do not maintain conflicting plans in several agent-specific memory directories.

## Work cycle

Select one Ready issue with prerequisites satisfied. Read its requirements/specs, inspect current code and status, and create a focused branch. Record an implementation plan, allowed scope, affected invariants and verification commands in the PR. Implement a small vertical slice, run relevant checks, inspect the diff, and attach evidence. A reviewer verifies the original acceptance criteria and failure cases. After acceptance, merge through normal repository controls, update the Project and close only completed scope.

If a requirement is wrong or incomplete, update the spec/ADR and issue openly; do not weaken tests or rewrite acceptance criteria silently to make an implementation pass. When blocked, record the actual dependency and leave work resumable. Never mark a future phase complete because scaffolding exists.

## Definition of Ready

An issue has a milestone, type/area/priority labels, requirements/spec links, explicit scope/non-goals, acceptance criteria, dependency links and validation expectations. Its prerequisites are done or an explicit parallel path is safe. High-risk changes identify tenant/data/operational impact before work starts.

## Definition of Done

Behavior and failure criteria demonstrated; relevant unit/integration/contract/evaluation checks pass; source/spec/schema/code agree; dependencies and migrations reviewed; logs do not expose sensitive data; review findings resolved; rollback/operations implications documented. “Enterprise grade” is evidence from these gates, not additional layers or a claim in README.

## Commands and enforcement

Implemented now: `make check` covers contracts, format verification, vet, behavior tests, import boundaries, native/ARM64 builds and process smoke tests. `make fmt`, `make race` and `make vuln` are separate real commands. CI uses these same targets. `make integration` runs real file-backed SQLite tests; `make generate-check` verifies sqlc output without modifying committed bindings, and `make licenses-check` checks linked-module notices. Both generation and license checks also run in `make check` and CI. Do not add success-returning stubs or claim nonexistent checks ran.

Boundary enforcement should use Go's actual import graph (go list), not just a text grep. SQLite tenant scoping requires real file-backed integration tests, including FTS and write contention; mocks cannot establish isolation. Generators must be reproducible; contract drift and code generation drift fail CI. Vulnerability scan exceptions require documented reachability assessment, owner and expiry; an agent cannot suppress them unilaterally.

Branch protections and CI configuration should enforce what the repository plan permits. Record any GitHub account/plan limitation honestly; documentation alone does not enforce a rule. No automatic production deployment is part of this foundation.

## Handoff record

Each PR or issue progress update includes: issue/branch/commit, what changed, relevant requirement IDs, tests actually run with results, known gaps, migration/rollback concerns and exact next step. Keep resumable state in version control/GitHub rather than relying on one conversation. Do not commit credentials or raw user context as agent memory.

Separate implementation from evaluation for tenant isolation, billing, unsafe fetch and release changes; a review may be human or explicitly commissioned independent agent review, but the author cannot treat a self-written approval as independent evidence. This document does not automatically authorize spawning agents, external messaging, merges or deployments.

## Harness tools considered

GitHub Spec Kit's [agentic spec-driven process](https://github.github.com/spec-kit/reference/agentic-sdd.html) and Anthropic's [long-running harness guidance](https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents) support explicit specifications, incremental work and durable handoff evidence. We adopt those practices without installing another orchestration framework now. Spec Kit or another tool can be added later if it reduces maintenance and uses these same sources of truth rather than creating a competing task system. Productivity percentages from marketing material are not acceptance criteria.
