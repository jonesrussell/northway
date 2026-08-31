# Northway news reader prototype

This dependency-free prototype renders Northway feed snapshots in a browser while keeping the service API key on a local Node.js proxy.

```bash
NORTHWAY_URL=http://127.0.0.1:8080 \
NORTHWAY_API_KEY='<feeds:read key>' \
node prototype/server.mjs
```

Open <http://127.0.0.1:4173>. Override the listener with `PORT` or `HOST`.

The page reads source metadata only. Article text remains on the publisher's site.
