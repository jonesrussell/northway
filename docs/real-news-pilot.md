# Runnable local personal-news pilot

Status: implemented for local operator use. The reviewed `personal-local-v1` profile provisions five real publisher feeds and five saved feeds into an existing tenant. It enables bounded metadata-only polling but starts no timer. AI-provider export and commercial use remain disabled.

## Active profile

| Saved feed | Active sources |
|---|---|
| Development | InfoQ Programming, GitHub Blog |
| Entertainment | Global Entertainment |
| Canada | Global Canada |
| World | Global World |
| Mixed | All five active sources, with the existing publisher cap |

CBC Canada is excluded after another workstation transport failure. Variety, APTN News and BBC World remain excluded because of their recorded publisher-use holds; Anishinabek News remains excluded after the recorded 403 and unresolved permission. The First Nations saved feed is therefore not provisioned yet. This is an explicit release gap, not silent substitution with general Canadian news.

The exact JSON manifest is hash-pinned by the binary. A changed byte, URL, source identity, approval record or policy is rejected. Provisioning is atomic and idempotent; an existing source or feed with conflicting metadata aborts the whole transaction. The command attaches sources, configures deterministic ranking preferences and enables four-hour poll policies with the shared collector limits. It cannot start a scheduler, traverse an article URL or export content to a provider.

## Operator runbook

Stop `serve` before every command because SQLite has one process owner.

```sh
northway migrate --database /private/northway.sqlite
northway tenant create --database /private/northway.sqlite --tenant "$TENANT_UUID"
northway pilot provision \
  --database /private/northway.sqlite \
  --tenant "$TENANT_UUID" \
  --manifest /usr/share/northway/catalogue/personal-local-v1.json
northway ingest once --database /private/northway.sqlite --tenant "$TENANT_UUID"
```

`ingest once` attempts at most one due source. Run it five times for the first fill; after that, calls remain idle until a source is due. A future systemd timer may invoke the same command, but this repository change does not deploy or install one. The command reports counts and status only; it does not print feed bodies or secrets.

Create a `feeds:read` key and run `serve` to query the saved feed IDs printed in the manifest. Claudriel can use the same `/v1/feed-queries` contract later; it is not required for a direct API smoke test.

## Real workstation evidence: 2026-08-31

A fresh private SQLite database was migrated, provisioned from the hash-pinned profile and polled through the production HTTPS collector. All five active feed-document requests returned HTTP 200 and committed 48 metadata-only items:

| Source | Items | Decoded bytes |
|---|---:|---:|
| Global World | 10 | 20,671 |
| Global Canada | 10 | 21,880 |
| GitHub Blog | 10 | 173,457 |
| InfoQ Programming | 8 | 13,272 |
| Global Entertainment | 10 | 21,166 |

The authenticated HTTP API then returned a deterministic snapshot from that corpus with `coverage.status=complete`, five current sources and source-linked metadata. The Mixed request returned Development, Entertainment and Canada; World was explicitly warned as a gap because the Corus publisher group had reached the configured cap. The dedicated World feed contains ten current items. No article page, image or enclosure was requested, and no feed content was committed to this repository or sent to an AI provider.

This proves the local end-to-end path, not Pi reachability, unattended scheduling, publisher permission for commercial redistribution or representative First Nations coverage. Keep those gates open for deployment and productization.
