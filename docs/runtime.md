# Runtime foundation

Implemented in issue #8: one Go 1.27.0 module, standard-library application code, process lifecycle, explicit configuration, health endpoints and reproducible checks. No SQLite database, migration command, feed ingestion, tenant API, provider calls or scheduled jobs exist yet. A running health endpoint is not a usable feed service.

## Run locally

```sh
make build
./bin/northway version
./bin/northway serve --listen=127.0.0.1:8080
```

GET /healthz returns 200 with status alive. GET /readyz returns 503 with status not_ready until storage and application readiness are wired. Both return uncached JSON. No dependency error details are returned. Other API paths are not implemented. Health probes are operational, not tenant APIs; no private data is exposed. There is no public listener by default, and no production deployment is authorized by running these commands.

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

make check runs contract fixtures, gofmt verification, vet, behavior tests, import-boundary checks, native/ARM64 builds and a real-process HTTP/SIGTERM smoke test. make fmt is the explicit formatting command. Race checks require a C compiler on a supported host; production builds disable CGo. govulncheck is pinned to v1.7.0 and runs separately from the application module. Vulnerability results are point-in-time evidence, not a guarantee. Future integration/code-generation targets are added only when their implementations exist.

The import guard reads go list -deps -test -json, including external test packages. It rejects direct SQL access outside the storage adapter, provider SDKs outside their adapter and unregistered layers/dependencies. Policy-negative tests run with the real graph check; it is an architecture guard, not a security sandbox. New layers/dependencies need a reviewed policy update.

## Container and ARM artifact

```sh
docker build --build-arg REVISION="$(git rev-parse HEAD)" -t northway:local .
docker run --rm --read-only --cap-drop ALL --security-opt no-new-privileges \
  -p 127.0.0.1:8080:8080 northway:local
```

`make container-smoke IMAGE=northway:local` tests the explicitly named local image on an ephemeral loopback port, runs it non-root/read-only with dropped capabilities and a 128 MiB test limit, checks health/unavailable readiness and clean termination, then removes only its test container. The limit is a smoke-test condition, not a measured pilot budget.

The image uses a digest-pinned Go builder and a scratch runtime, runs as UID/GID 65532, and contains no shell, PHP, crawler, local model, database server or application dependency modules. Container listen defaults to 0.0.0.0:8080; host port publication remains explicit. No storage volume is created by this foundation. Before network acquisition is implemented, add and review a CA trust bundle; before SQLite, establish a writable persistent directory and ownership/migration contract through waaseyaa-infra.

make arm64 produces bin/northway-linux-arm64 using CGO_ENABLED=0. Artifact architecture inspection proves the build target, not Pi execution, workload capacity or power-loss recovery. Native ARM testing belongs to the infra/device acceptance gate. CPU architecture does not change the service's container/deployment ownership.

## Dependency and license evidence

Application code uses only the Go standard library, so go list -m all contains only this module and no go.sum is necessary. SQLite/gofeed/provider/sqlc/Goose selections remain future dependencies, not installed modules. The separate vulnerability-scanning tool downloads its own pinned tool graph; it is not linked into Northway. Retain the Go license notice with binary distribution; the container includes /licenses/Go-LICENSE. This does not choose a first-party license or claim that future dependency/source-rights review is complete.
