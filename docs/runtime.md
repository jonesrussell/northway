# Runtime foundation

Implemented in issues #8, #10 and #53: one Go 1.27.0 module, process lifecycle, explicit configuration, health endpoints, SQLite corpus storage and migrations, reproducible checks and explicitly enabled in-process publisher polling. Scoped credentials, query coordination and [operator-only ingestion](ingestion.md) are implemented. Authenticated deterministic query/snapshot and reversible feedback APIs are implemented in #16; provider calls and scheduled AI/custom-list jobs do not exist yet. A running health endpoint is not a usable feed service. See [storage](storage.md) for schema and migration details.

## Run locally

```sh
make build
./bin/northway version
./bin/northway migrate --database ./data/northway.sqlite
./bin/northway serve --database ./data/northway.sqlite
# With serve stopped, prepare a private destination and create a new snapshot.
mkdir -m 0700 -p ./backups
./bin/northway backup --database ./data/northway.sqlite --output ./backups/northway.sqlite
# Or health only, with readiness unavailable:
./bin/northway serve --listen=127.0.0.1:8080
```

GET /healthz returns 200 with status alive. GET /readyz returns 200 with status ready when configured storage is usable, otherwise 503 with status not_ready. GET /statusz reports only disabled, idle, polling or degraded. When polling is enabled, it combines the in-memory loop with persisted per-source freshness, fixed failure state and lease health; it does not expose the affected tenant, source, URL or error. Feed completeness remains in authenticated query coverage. All probes return uncached JSON. With storage configured, the three product routes in [the API contract](api.md) use persisted scoped-key authentication.

Background publisher polling is opt-in:

```sh
northway serve --database ./northway.sqlite --poll-tenant 00000000-0000-4000-8000-000000000001
```

The equivalent environment setting is `NORTHWAY_POLL_TENANT`. It accepts one already provisioned tenant and uses only approved, enabled poll policies stored by the operator profile. It does not activate a source. The same background owner recovers expired query claims and runs bounded retention at startup and then hourly before publisher work. A cleanup error keeps `/statusz` degraded until a later hourly success but does not starve publisher polling; timeout and failure are distinct fixed log outcomes. A busy checkpoint, saturated batch or preserved unreconciled charge emits a bounded operator warning without exposing content. Leave the setting empty only for bounded API development: an API-only sustained-traffic mode has no maintenance schedule in this pilot and is not release-ready.

Configuration precedence is defaults → explicitly present environment → flags. Empty/zero values are validated, not silently replaced. NORTHWAY_LISTEN_ADDR accepts a literal IP:port, default 127.0.0.1:8080; IPv6 must be bracketed. Port zero is allowed for local tests. NORTHWAY_SHUTDOWN_TIMEOUT defaults to 10s and accepts 1s–1m. NORTHWAY_LOG_LEVEL accepts debug/info/warn/error. The matching flags are --listen, --shutdown-timeout and --log-level. No .env file is loaded automatically.

SIGINT/SIGTERM stops accepting connections and drains active requests within the configured timeout. Active request contexts retain their values and are canceled only on forced close or completion, rather than being canceled by the initial termination signal. Exceeding the deadline closes connections, cancels request contexts and exits with failure. Fatal bind/serve/configuration errors also exit nonzero. The service does not support hijacked connections or WebSockets.

## Verification

All fatal command errors use the same JSON stderr format as server logs and exit nonzero, including invalid configuration and bind failures. The process smoke checks these failure paths and retains raw startup diagnostics if a log line is incomplete or is not JSON.

Host Makefile builds append `-dirty` to the HEAD revision when tracked changes or untracked, non-ignored files exist. Ignored build outputs do not mark the tree dirty. An explicit `REVISION` remains a caller-supplied override; container build arguments and matching labels are metadata, not a source attestation. Build distributable artifacts from a verified clean checkout.

```sh
python3 -m pip install -r requirements-dev.txt
make check
make race
make vuln
```

make check runs contract fixtures, gofmt verification, vet, behavior and real-file SQLite tests, import-boundary checks, sqlc generation/notice drift checks, native/ARM64 builds and process migration/restart/HTTP/SIGTERM smoke tests. make integration isolates SQLite tests; make generate regenerates SQL bindings. make fmt is the explicit formatting command. Race checks require a C compiler on a supported host; production builds disable CGo. govulncheck is pinned to v1.7.0 and runs separately from the application module. Vulnerability results are point-in-time evidence, not a guarantee. The runtime and check scripts target Linux amd64/arm64 (WSL for Windows development).

The import guard reads go list -deps -test -json, including external test packages. It rejects direct SQL access outside the storage adapter, provider SDKs outside their adapter and unregistered layers/dependencies. Policy-negative tests run with the real graph check; it is an architecture guard, not a security sandbox. New layers/dependencies need a reviewed policy update.

## Container and ARM artifact

```sh
docker build --build-arg REVISION="$(git rev-parse HEAD)" -t northway:local .
docker run --rm --read-only --cap-drop ALL --security-opt no-new-privileges \
  -p 127.0.0.1:8080:8080 northway:local
```

`make container-smoke IMAGE=northway:local` tests the explicitly named local image on ephemeral loopback ports, non-root/read-only with dropped capabilities and a 128 MiB limit. It checks health-only behavior, then migration, the executable readiness probe, storage readiness, restart, offline snapshot creation and migration-compatible reopening using a disposable named volume; it removes only its own test resources. The limit is a smoke-test condition, not a measured pilot budget.

The image uses a digest-pinned Go builder and a scratch runtime, runs as UID/GID 65532, and contains no shell, PHP, crawler, local model or separate database server. `northway healthcheck` performs one two-second, no-redirect request to the fixed container-local `http://127.0.0.1:8080/readyz` endpoint and accepts only the exact bounded ready response. It is intended only for the documented storage-backed container on port 8080; health-only mode deliberately remains not ready, and a custom listen port requires a different orchestrator probe. Container listen defaults to 0.0.0.0:8080; host port publication remains explicit. /data/northway is private and owned by 65532; mount storage at /data and use /data/northway/northway.sqlite. Production storage mounting and migration orchestration remain in waaseyaa-infra. No implicit volume is created by the image. The CA trust bundle is copied from the same digest-pinned builder for verified outbound HTTPS; no publisher certificates or private trust roots are added.

make arm64 produces bin/northway-linux-arm64 using CGO_ENABLED=0. Artifact architecture inspection proves the build target, not Pi execution, workload capacity or power-loss recovery. Native ARM testing belongs to the infra/device acceptance gate. CPU architecture does not change the service's container/deployment ownership.

## Dependency and license evidence

go.mod/go.sum pin modernc SQLite and the Goose migration library and their resolved dependencies. sqlc is a checksum-pinned development binary; gofeed v1.4.2 is installed for bounded feed parsing; no AI SDK is installed. make licenses records notices for actually linked application modules. The separate vulnerability scanner downloads its own tool graph and is not linked into Northway. The container includes /licenses/Go-LICENSE and /licenses/THIRD_PARTY_NOTICES.txt. No first-party license is selected and no publisher-content rights are implied.
