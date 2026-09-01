import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const host = process.env.HOST ?? "127.0.0.1";
const port = Number.parseInt(process.env.PORT ?? "4173", 10);
const northwayURL = (process.env.NORTHWAY_URL ?? "http://127.0.0.1:8080").replace(/\/$/, "");
const apiKey = process.env.NORTHWAY_API_KEY;

if (!["127.0.0.1", "localhost", "::1"].includes(host)) {
  throw new Error("HOST must be a loopback address; this prototype cannot be exposed.");
}
if (!Number.isInteger(port) || port < 1 || port > 65_535) {
  throw new Error("PORT must be an integer from 1 to 65535.");
}

const allowedHosts = new Set([`127.0.0.1:${port}`, `localhost:${port}`, `[::1]:${port}`]);

const feeds = Object.freeze({
  mixed: {
    id: "bdcc63ec-6932-5abb-8b73-8385b6497ebf",
    label: "Mixed",
    context: "A balanced briefing across software development, entertainment, Canadian news, and world news.",
  },
  development: {
    id: "921bcb1b-e464-5ff4-9a7e-d8b79faa900a",
    label: "Development",
    context: "Software engineering, programming languages, developer tools, open source, and AI-assisted development.",
  },
  entertainment: {
    id: "9978be55-592d-53fc-bdff-a85dda93f24d",
    label: "Entertainment",
    context: "Film, television, music, games, and Canadian entertainment coverage.",
  },
  canada: {
    id: "0fcce4ea-02a1-54e8-bf22-a0bc10dbecbf",
    label: "Canada",
    context: "Canadian public affairs and national news with useful regional context.",
  },
  world: {
    id: "0ee72c1d-f8b9-5144-aabc-bfe68816e2ac",
    label: "World",
    context: "Major international news and events with practical context for a Canadian reader.",
  },
});

const staticFiles = Object.freeze({
  "/": ["index.html", "text/html; charset=utf-8"],
  "/index.html": ["index.html", "text/html; charset=utf-8"],
  "/styles.css": ["styles.css", "text/css; charset=utf-8"],
  "/app.js": ["app.js", "text/javascript; charset=utf-8"],
});

function headers(extra = {}) {
  return {
    "Cache-Control": "no-store",
    "Content-Security-Policy": "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'",
    "Cross-Origin-Opener-Policy": "same-origin",
    "Referrer-Policy": "no-referrer",
    "X-Content-Type-Options": "nosniff",
    ...extra,
  };
}

function json(response, status, body) {
  response.writeHead(status, headers({ "Content-Type": "application/json; charset=utf-8" }));
  response.end(JSON.stringify(body));
}

async function readJSON(request) {
  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    size += chunk.length;
    if (size > 16_384) throw new Error("request_too_large");
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}");
}

async function retrieveNews(request, response) {
  if (!apiKey) {
    json(response, 503, { error: "Northway is not configured. Set NORTHWAY_API_KEY on the local proxy." });
    return;
  }

  const contentType = request.headers["content-type"]?.split(";", 1)[0].trim().toLowerCase();
  if (contentType !== "application/json") {
    json(response, 415, { error: "Content-Type must be application/json." });
    return;
  }
  const fetchSite = request.headers["sec-fetch-site"];
  if (fetchSite && fetchSite !== "same-origin") {
    json(response, 403, { error: "Cross-site requests are not accepted." });
    return;
  }

  let input;
  try {
    input = await readJSON(request);
  } catch {
    json(response, 400, { error: "The request body must be valid JSON under 16 KB." });
    return;
  }

  if (!input || typeof input !== "object" || Array.isArray(input) || !Object.hasOwn(feeds, input.feed)) {
    json(response, 400, { error: "Choose one of the available feeds." });
    return;
  }
  const selected = feeds[input.feed];

  const maxAgeHours = Number.isInteger(input.maxAgeHours) && input.maxAgeHours >= 1 && input.maxAgeHours <= 336
    ? input.maxAgeHours
    : 168;

  try {
    const upstream = await fetch(`${northwayURL}/v1/feed-queries`, {
      method: "POST",
      signal: AbortSignal.timeout(10_000),
      headers: {
        Authorization: `Bearer ${apiKey}`,
        "Content-Type": "application/json",
        "Idempotency-Key": randomUUID(),
      },
      body: JSON.stringify({
        feed_id: selected.id,
        context: {
          intent: selected.context,
          technologies: [],
          focus: [selected.label],
        },
        max_age_hours: maxAgeHours,
        limit: 10,
      }),
    });

    const body = await upstream.json().catch(() => null);
    if (!upstream.ok) {
      const message = body?.error?.message ?? body?.message ?? `Northway returned HTTP ${upstream.status}.`;
      json(response, upstream.status >= 500 ? 502 : upstream.status, { error: message });
      return;
    }

    if (!body || typeof body !== "object" || Array.isArray(body)) {
      json(response, 502, { error: "Northway returned an unreadable response." });
      return;
    }

    json(response, 200, { feed: { key: input.feed, label: selected.label }, snapshot: body });
  } catch (error) {
    const timedOut = error?.name === "TimeoutError";
    json(response, 502, { error: timedOut ? "Northway took too long to respond." : "Northway could not be reached." });
  }
}

async function handleRequest(request, response) {
  if (!allowedHosts.has(request.headers.host)) {
    json(response, 400, { error: "Invalid Host header." });
    return;
  }
  const url = new URL(request.url ?? "/", "http://localhost");

  if (request.method === "GET" && url.pathname === "/healthz") {
    json(response, 200, { status: "ok", northway_configured: Boolean(apiKey) });
    return;
  }

  if (request.method === "GET" && url.pathname === "/favicon.ico") {
    response.writeHead(204, headers());
    response.end();
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/news") {
    await retrieveNews(request, response);
    return;
  }

  if (request.method === "GET" && staticFiles[url.pathname]) {
    const [file, contentType] = staticFiles[url.pathname];
    try {
      const content = await readFile(join(root, file));
      response.writeHead(200, headers({ "Content-Type": contentType }));
      response.end(content);
    } catch {
      json(response, 500, { error: "The prototype assets could not be loaded." });
    }
    return;
  }

  json(response, 404, { error: "Not found." });
}

const server = createServer((request, response) => {
  handleRequest(request, response).catch(() => {
    if (!response.headersSent) {
      json(response, 500, { error: "The prototype could not process the request." });
      return;
    }
    response.destroy();
  });
});

server.listen(port, host, () => {
  console.log(`Northway reader listening on http://${host}:${port}`);
});
