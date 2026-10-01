# Curated RSS/Atom catalogue import

Local implementation; not deployed or activated. Uses existing tenant-owned source/feed/poll transactions and preserves global 100 poll-source, 180 daily attempt and 64 MiB daily acquisition caps. No schema change, new credential or timer. The reviewed personal pilot hash gate remains unchanged.

## Operator flow

1. Obtain the typed research import JSON and full audit JSON. Keep both with Library identity and hashes. The researcher's fetched/parsed status is research evidence, not proof of acceptance by NorthCloud's narrower parser/transport or publisher-use approval.
2. `northway catalogue prepare --research PATH --output NEW_PATH --limit 10` creates a new private inert register. It retains all candidates, publisher, original discovery URL, topic, region, language, publication-rights status and restriction notes. Initial candidates span at most ten topics and distinct publisher groups; explicit restrictive entries and transport-incompatible URLs are not selected. No network requests occur.
3. Review the resulting choices, full audit, redirect/alias pairs, rights and metadata-use basis. Unsupported query URLs, HTTP URLs and canonical aliases require explicit source qualification, not transport weakening. Do not silently substitute the researcher's resolved URL. Change selected entries and policy only through a new reviewed register revision.
4. `northway catalogue validate --manifest PATH --sha256 EXACT_DIGEST` validates without opening storage. Enabled policy requires an approval record and an explicit metadata-use basis for every selected publisher, including applicability of restriction notes; research availability alone is insufficient. Enabled manifests do not grant original content, article text, images, video or commercial redistribution rights.
5. A coordinated operator with exclusive offline database ownership can run `northway catalogue inventory --database PATH --tenant UUID`. It reports global poll rows and the named tenant's source/policy inventory. It uses supported storage opening, not a direct database-copy/permission workaround. Stop serve only under release coordination; no production inventory was run by this implementation.
6. Once live counts are known, `northway catalogue provision --database PATH --tenant UUID --manifest PATH --sha256 EXACT_DIGEST` atomically admits selected sources. Disabled manifests create unapproved/disabled poll policies. Enabled reviewed manifests can transition matching disabled policies to approved/enabled. No existing pause, URL, membership or interval conflict is silently rewritten. Capacity denial rolls back the complete transaction. No tenant, grant, schedule or backup is created.
7. Enable only a qualified canary in one selected tenant; retain inactive research candidates in the file, not additional poll rows. The 113 candidates are not a requirement to fit/activate all under a cap of 100. Expansion needs current counts, resource and useful-output evidence, exact reviewed subset and release coordination.

## Identity, metadata and scheduling

Canonical admitted URL derives a stable UUID; distinct IDs for an already-owned exact source URL conflict. Duplicate input URLs fail rather than creating ambiguous identities. Known redirect aliases in the full research audit still need explicit operator reconciliation. Publisher grouping is stable and case-normalized; stored source titles include publisher attribution. The immutable register is the complete external rights/topic/region/language policy record: mount/retain it and the full audit with the release; these fields are not newly duplicated into the corpus database.

Each original topic becomes a saved feed, up to 20. Existing retrieval categories remain conservative: technology maps to development, Indigenous to first_nations, Canada to canada, culture/lifestyle topics to entertainment, other topics to world. Original topic labels stay in the register/feed title; this does not change public category contracts.

Daily-or-slower polling is required. First eligibility is deterministically staggered across one interval; retries do not reset existing schedules. Ten daily sources at a 2 MiB ceiling reserve at most 20 MiB/day before existing work. Actual upstream cache/backoff may delay refresh. All tenants share attempt/byte ceilings and one active acquisition. Whole-workspace replication multiplies demand: qualifying one tenant does not approve replication or a budget increase.

Existing topic-feed membership and policy are immutable/idempotent. A different selection within an already-provisioned topic fails visibly; prepare an explicit reviewed reconciliation/migration rather than auto-adopting it.

## Optional customer catalogue

Keep `developer-v1` compatibility. Explicit `NORTHCLOUD_CATALOGUE=curated-v1` requires operator-controlled `NORTHCLOUD_CURATED_MANIFEST` and exact `NORTHCLOUD_CURATED_SHA256`, plus existing first-party assertion verification. Startup validates the register before opening the listener; authenticated customer provisioning uses the same transactional policy, never request-supplied URLs. No activation default is added. This mode still provisions per workspace: review replication against remaining global capacity and byte budgets before changing production configuration. Existing developer sources are not silently removed.

## PostgreSQL handoff

Port catalogueProfile, ProvisionCuratedCatalogue, ProvisionCustomerCuratedCatalogue and CatalogueInventory with existing profile transactions. Additional generated queries are OtherSourceURL and CatalogueSources in db/queries/ingest.sql. New PilotSource fields represent disabled admission, explicitly reviewed activation and first-interval staggering. There is no new schema to merge; PostgreSQL must preserve atomic source cap/dedup/conflict behavior and tenant boundaries. Isolated branch feat/curated-feed-import; no shared push/deploy or production seeding.
