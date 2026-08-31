# Synthetic collector inputs

These fixtures were authored for Northway on 2026-08-31. No publisher content was copied. All network-looking URLs use reserved `.invalid` domains and must never be requested.

- `atom-initial.xml`: two entries with stable opaque IDs; one lacks publication time. Encoded markup includes a link and an instruction-like string as untrusted content, not an agent instruction.
- `atom-updated.xml`: the same two IDs with one changed title/summary and update timestamp. It supports future changed-item/deduplication tests without inventing a successful poll.
- `rss-initial.xml`: RSS 2.0 item with a stable GUID, timestamp, description and enclosure. Future ingestion must not follow its article/enclosure links.
- `malformed.xml`: intentionally truncated input for future parse-failure tests.

The offline catalogue check screens all four files as bounded UTF-8 without DTD/entities and rejects non-reserved literal URLs before parsing; well-formed documents also have decoded attribute/text/tail URLs checked. It verifies fixture shapes and identities only. It does not implement feed normalization, safe fetching, HTML sanitization, resource limits or persisted poll state. #12 must add real behavior tests for 200/304, malformed/oversized bodies, redirects/private addresses, bounded retries, restart and commit/lease fencing. Do not hardcode these fixtures as evidence that the collector exists.
