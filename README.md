# go-open-discogs-api

High-performance, read-only Go API for PostgreSQL databases populated from public Discogs data dumps. This is an independent DSUB project and is not affiliated with Discogs.

The project is pre-release. The Java [`open-discogs-api`](https://github.com/dsub-io/open-discogs-api) remains supported until this implementation passes contract, production-data, and performance verification.

## Design boundaries

- Public HTTP (`:8080`) serves the catalog and OpenAPI contract.
- Management HTTP (`127.0.0.1:8081`) serves liveness, readiness, and optional Prometheus metrics.
- PostgreSQL access is read-only. Schema and index migrations belong to [`open-discogs-model`](https://github.com/dsub-io/open-discogs-model), never this service.
- OTLP tracing is opt-in. With tracing disabled, no exporter or telemetry network activity is created.
- Prometheus metrics are local pull telemetry and can be disabled independently.
- Pages are capped at 30 records; DB connections, query duration, CPU use, memory use, response buffer retention, and count caching are bounded.

The dependency direction is `catalog domain <- HTTP/PostgreSQL adapters <- app bootstrap`. HTTP, persistence, telemetry, and process lifecycle have separate packages and focused interfaces.

## Run

Use a complete PostgreSQL URL:

```sh
API_DATABASE_URL='postgres://readonly:password@127.0.0.1:5432/discogs?sslmode=require' \
go run .
```

Or use split settings:

```sh
go run . \
  --db-host=127.0.0.1:5432 \
  --db-username=readonly \
  --db-password=password
```

CLI values override ENV values, which override defaults. Prefer ENV, Docker secrets, or Kubernetes Secrets for credentials because CLI arguments can be visible in the host process list.

`--help` and `--version` do not connect to PostgreSQL.

## Configuration inventory

Every runtime setting exposed by CLI has an ENV equivalent with identical meaning.

| CLI flag | ENV | Type | Default | Requirement | Sensitive | Purpose |
|---|---|---:|---|---|---:|---|
| `--address` | `API_ADDRESS` | string | `:8080` | optional | no | Public HTTP listener. |
| `--management-address` | `API_MANAGEMENT_ADDRESS` | string | `127.0.0.1:8081` | optional | no | Management HTTP listener. |
| `--server-url` | `API_SERVER_URL` | string | `http://localhost:8080` | optional | no | Public URL used in API links. |
| `--cache-control` | `API_CACHE_CONTROL` | string | `public, max-age=60, stale-while-revalidate=300` | optional | no | Successful API response cache policy. |
| `--access-log` | `API_ACCESS_LOG` | boolean | `false` | optional | no | Structured request log. |
| `--max-procs` | `API_MAX_PROCS` | integer | `0` (unlimited) | optional | no | Positive values set exact `GOMAXPROCS`; zero adds no application limit. |
| `--memory-limit-mib` | `API_MEMORY_LIMIT_MIB` | integer | `0` (unlimited) | optional | no | Positive values set the Go soft memory limit; zero adds no application limit. |
| `--log-level` | `API_LOG_LEVEL` | string | `info` | optional | no | `debug`, `info`, `warn`, or `error`. |
| `--query-timeout` | `API_QUERY_TIMEOUT` | duration | `10s` | optional | no | Maximum PostgreSQL operation duration. |
| `--shutdown-timeout` | `API_SHUTDOWN_TIMEOUT` | duration | `30s` | optional | no | Graceful shutdown deadline. |
| `--database-url` | `API_DATABASE_URL` | string | empty | conditional | yes | Complete PostgreSQL URL; overrides split settings. |
| `--db-host` | `API_DB_HOST` | string | empty | conditional | no | PostgreSQL `host:port` when URL is unset. |
| `--db-username` | `API_DB_USERNAME` | string | empty | conditional | no | PostgreSQL username when URL is unset. |
| `--db-password` | `API_DB_PASSWORD` | string | empty | conditional | yes | PostgreSQL password when URL is unset. |
| `--db-database` | `API_DB_DATABASE` | string | `discogs` | optional | no | PostgreSQL database name. |
| `--db-sslmode` | `API_DB_SSLMODE` | string | `prefer` | optional | no | PostgreSQL `sslmode`. |
| `--db-max-conns` | `API_DB_MAX_CONNS` | integer | `10` | optional | no | Production-safe connection upper bound protecting the shared database. |
| `--db-min-conns` | `API_DB_MIN_CONNS` | integer | `0` | optional | no | Warm idle connection floor. |
| `--db-statement-cache` | `API_DB_STATEMENT_CACHE` | integer | `128` | optional | no | Prepared statements retained per connection. |
| `--metrics-enabled` | `API_METRICS_ENABLED` | boolean | `true` | optional | no | Expose local Prometheus metrics. |
| `--tracing-enabled` | `API_TRACING_ENABLED` | boolean | `false` | optional | no | Enable OTLP HTTP trace export. |
| `--otlp-endpoint` | `OTEL_EXPORTER_OTLP_ENDPOINT` | string | empty | conditional | no | Required only when tracing is enabled. |
| `--trace-sample-ratio` | `OTEL_TRACES_SAMPLER_ARG` | float | `0.1` | optional | no | Parent-based trace sampling ratio from 0 through 1. |

`--version` is a process control command, not a runtime setting.

## HTTP contracts

- OpenAPI 3.1: `GET /openapi.json` or compatibility path `GET /v3/api-docs`
- Build version: `GET /version`
- Liveness: management `GET /healthz`
- Readiness: management `GET /readyz` or `GET /actuator/health`
- Metrics, when enabled: management `GET /metrics` or `GET /actuator/prometheus`

The management listener defaults to loopback. Kubernetes explicitly binds it to `:8081` so kubelet probes can reach the pod.

## Containers and Kubernetes

Build and run the non-root, read-only image:

```sh
docker build -t go-open-discogs-api:local .
docker run --rm --read-only \
  -p 8080:8080 -p 127.0.0.1:8081:8081 \
  -e API_DATABASE_URL='postgres://readonly:password@database:5432/discogs?sslmode=require' \
  go-open-discogs-api:local
```

[`compose.yaml`](compose.yaml) and [`deploy/kubernetes`](deploy/kubernetes) use the same ports and ENV names. The Kubernetes example separates non-secret ConfigMap values from the database URL Secret and includes startup, readiness, liveness, resource, security, and termination settings.

Release tags publish multi-architecture `linux/amd64` and `linux/arm64` images to `ghcr.io/dsub-io/go-open-discogs-api`.

## Resource sizing

The application imposes no CPU or Go memory limit unless one is configured. Negative values fail startup, zero is unlimited from the application's perspective, and positive values are applied without an arbitrary policy ceiling. `API_MEMORY_LIMIT_MIB` is a Go runtime soft limit, not a hard RSS cap; container policy remains authoritative. The database pool is the exception because it protects a shared service: its production-safe default is 10 maximum and 0 minimum connections. The response buffer pool retains at most 4 MiB and the count cache is bounded.

For a 512 MiB container, a reasonable starting point is `API_MAX_PROCS=2`, `API_MEMORY_LIMIT_MIB=384`, `API_DB_MAX_CONNS=4`, and `API_DB_STATEMENT_CACHE=64`. These are explicit limits, not CPU-ratio guesses.

## Telemetry

Prometheus reports bounded route-template labels for request count, duration, in-flight work, Go/process state, and PostgreSQL pool state. OTLP traces cover HTTP and PostgreSQL without SQL text, query parameters, connection details, search terms, or credentials.

Set `API_METRICS_ENABLED=false` to remove metrics routes. Leave `API_TRACING_ENABLED=false` for a zero-exporter, zero-egress tracing path. Exporter delivery errors are logged and do not change public API responses.

## Verification and performance

The quality gate runs formatting, module checks, `vet`, race tests, 100% coverage, OpenAPI validation, integration/E2E tests against canonical migrations, container checks, CodeQL, and vulnerability scanning.

Performance reports belong in [`docs/performance`](docs/performance). Each report must record the same data, hardware, concurrency, warm-up, and run count before and after, plus p50/p95/p99 latency, throughput, memory/allocation, and relevant DB query count or plan. No full-dataset claim is made without a representative dataset measurement.

## License

MIT
