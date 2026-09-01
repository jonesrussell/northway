# Northway news reader prototype

This dependency-free prototype renders Northway feed snapshots in a browser while keeping the service API key on a local Node.js proxy. It is a UX reference, not the production reader or Pi runtime. The proxy endpoint is unauthenticated and spends the configured API key, so keep it bound to loopback and do not expose it to a LAN or the internet. Node.js 18 or newer is required.

```bash
NORTHWAY_URL=http://127.0.0.1:8080 \
NORTHWAY_API_KEY='<feeds:read key>' \
node prototype/server.mjs
```

Open <http://127.0.0.1:4173>. `PORT` may be overridden for local use; leave `HOST` unset so the server remains on `127.0.0.1`.

The page reads source metadata only. Article text remains on the publisher's site.
