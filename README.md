# go-open-discogs-api

High-performance, read-only Go API for PostgreSQL databases populated from public Discogs data dumps. This is an independent DSUB project and is not affiliated with Discogs.

The project is pre-release. The Java [`open-discogs-api`](https://github.com/dsub-io/open-discogs-api) remains supported until this implementation passes contract, production-data, and performance verification.

## Design boundaries

- Public HTTP (`:8080`) serves the catalog and OpenAPI contract.
- Management HTTP (`127.0.0.1:8081`) serves liveness, readiness, and optional Prometheus metrics.
- PostgreSQL access is read-only. Schema and index migrations belong to [`open-discogs-model`](https://github.com/dsub-io/open-discogs-model), never this service.
- `API_DATABASE_SCHEMA` selects the already-imported schema. The API verifies schema usage, required tables, and `SELECT` privileges at startup but never creates or migrates database objects.
- Every deployment serves only data imported from the public monthly Discogs data dumps. The service never calls the Discogs API, accepts Discogs credentials, performs live hydration, or accepts catalog writes.
- OTLP tracing is opt-in. With tracing disabled, no exporter or telemetry network activity is created.
- Prometheus metrics are local pull telemetry and can be disabled independently.
- Cursor pages are capped at 30 records; DB connections, query duration, CPU use, memory use, and response buffer retention are bounded.

The dependency direction is `catalog domain <- HTTP/PostgreSQL adapters <- app bootstrap`. HTTP, persistence, telemetry, and process lifecycle have separate packages and focused interfaces.

## Data source and freshness

OpenDiscogs is a query layer over the dump snapshot currently present in PostgreSQL. It does not promise real-time parity with discogs.com. A detail request returns `404` when the resource is absent from the imported snapshot; that response does not assert that the resource is absent from Discogs. The next successful batch import is the only path by which the catalog changes.

`API_CACHE_CONTROL` controls reuse of OpenDiscogs HTTP responses. It is not a source-data freshness guarantee and is unrelated to the age of the imported monthly snapshot.

This boundary is intentional. The current [Discogs API Terms of Use](https://support.discogs.com/hc/en-us/articles/360009334593-API-Terms-of-Use) impose conditions including six-hour freshness, limited caching, rate-limit non-circumvention, required attribution, restrictions on transferring Restricted Data, revocable access, and broad accuracy and availability disclaimers. OpenDiscogs does not proxy that API or shift those obligations through an optional authenticated or anonymous mode. Applications that require data newer than the imported dump must evaluate and integrate the Discogs API independently under its then-current terms.

The MIT license covers this project's source code. Data remains subject to the rights and terms applicable at its source.

## Run

Use a complete PostgreSQL URL:

```sh
API_DATABASE_URL='postgres://readonly:password@127.0.0.1:5432/discogs?sslmode=require' \
API_DATABASE_SCHEMA='open_discogs' \
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

If `--database-schema` / `API_DATABASE_SCHEMA` is omitted, the API uses `public` and emits a `WARN` at startup. Set the same schema used by the batch importer to avoid mixing OpenDiscogs tables with unrelated public objects. The API role needs `CONNECT` on the database, `USAGE` on that schema, and read access to its tables; it does not need schema creation or migration privileges.

`--help` and `--version` do not connect to PostgreSQL. `--healthcheck` probes
`http://127.0.0.1:8081/readyz` and exits non-zero unless the management
listener, PostgreSQL, and the canonical dump snapshot are ready. A first import
does not become ready until deferred foreign keys are created and validated and
the imported tables are analyzed; Compose uses this process control.

## Configuration inventory

Every runtime setting exposed by CLI has an ENV equivalent with identical meaning.

| CLI flag | ENV | Type | Default | Requirement | Sensitive | Purpose |
|---|---|---:|---|---|---:|---|
| `--address` | `API_ADDRESS` | string | `:8080` | optional | no | Public HTTP listener. |
| `--management-address` | `API_MANAGEMENT_ADDRESS` | string | `127.0.0.1:8081` | optional | no | Management HTTP listener. |
| `--server-url` | `API_SERVER_URL` | string | `http://localhost:8080` | optional | no | Public URL used in API links. |
| `--cache-control` | `API_CACHE_CONTROL` | string | `public, max-age=60, stale-while-revalidate=300` | optional | no | Successful API response cache policy; not dump freshness. |
| `--access-log` | `API_ACCESS_LOG` | boolean | `false` | optional | no | Structured request log. |
| `--max-procs` | `API_MAX_PROCS` | integer | `0` (unlimited) | optional | no | Positive values set exact `GOMAXPROCS`; zero adds no application limit. |
| `--memory-limit-mib` | `API_MEMORY_LIMIT_MIB` | integer | `0` (unlimited) | optional | no | Positive values set the Go soft memory limit; zero adds no application limit. |
| `--log-level` | `API_LOG_LEVEL` | string | `info` | optional | no | `debug`, `info`, `warn`, or `error`. |
| `--query-timeout` | `API_QUERY_TIMEOUT` | duration | `10s` | optional | no | Maximum PostgreSQL operation duration. |
| `--shutdown-timeout` | `API_SHUTDOWN_TIMEOUT` | duration | `30s` | optional | no | Graceful shutdown deadline. |
| `--database-url` | `API_DATABASE_URL` | string | empty | conditional | yes | Complete PostgreSQL URL; overrides split settings. |
| `--database-schema` | `API_DATABASE_SCHEMA` | string | `public` | optional | no | Schema containing canonical OpenDiscogs tables; `public` emits a startup warning. |
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

`--version` and `--healthcheck` are process control commands, not runtime
settings, so they do not have ENV equivalents.

## HTTP contracts

- OpenAPI 3.1: `GET /openapi.json` or compatibility path `GET /v3/api-docs`
- Build version: `GET /version`
- Liveness: management `GET /healthz`
- Readiness: management `GET /readyz` or `GET /actuator/health`; returns down
  while the canonical catalog is bootstrap-pending, importing, or failed
- Metrics, when enabled: management `GET /metrics` or `GET /actuator/prometheus`

Collections use ascending resource-ID keyset pagination. Omit `after_id` for
the first page, then pass the response's non-null `next_after_id` while
`has_more` is true. Responses intentionally omit exact totals and page counts
because calculating them for arbitrary filters does not remain bounded as the
dump grows. `size` defaults to 20 and values above 30 are rejected.

Nested relation arrays are unordered and do not expose database row order as a
public contract. Multiple catalog numbers for the same Label and Release are
preserved losslessly in the Label release
`catnos` array; one resource row per Release keeps resource-ID pagination from
splitting or skipping it. Release format `qty` is a canonical decimal string so
values beyond signed integer storage remain lossless.

Substring search is limited to artist and label names, artist real names, and
master and release titles. Terms must contain 3 to 200 Unicode characters so
PostgreSQL can use the canonical trigram indexes. Large profile, contact, and
notes fields are returned in detail responses but are not searchable. Release
month filtering requires a year.

Reproducible before/after measurements and their full-dump limitations are in
[`docs/performance/2026-08-11-cursor-query-scalability.md`](docs/performance/2026-08-11-cursor-query-scalability.md)
and the OpenAPI-wide HTTP report at
[`docs/performance/2026-08-13-openapi-http-benchmark.md`](docs/performance/2026-08-13-openapi-http-benchmark.md).

The management listener defaults to loopback. Kubernetes explicitly binds it to `:8081` so kubelet probes can reach the pod.

## Containers and Kubernetes

Build and run the non-root, read-only image:

```sh
docker build -t go-open-discogs-api:local .
docker run --rm --read-only \
  -p 8080:8080 -p 127.0.0.1:8081:8081 \
  -e API_DATABASE_URL='postgres://readonly:password@database:5432/discogs?sslmode=require' \
  -e API_DATABASE_SCHEMA='open_discogs' \
  go-open-discogs-api:local
```

[`compose.yaml`](compose.yaml) and [`deploy/kubernetes`](deploy/kubernetes) use the same ports and ENV names. The Kubernetes example separates non-secret ConfigMap values from the database URL Secret and includes startup, readiness, liveness, resource, security, and termination settings.

Release tags publish multi-architecture `linux/amd64` and `linux/arm64` images to `ghcr.io/dsub-io/go-open-discogs-api`.

## Resource sizing

The application imposes no CPU or Go memory limit unless one is configured. Negative values fail startup, zero is unlimited from the application's perspective, and positive values are applied without an arbitrary policy ceiling. `API_MEMORY_LIMIT_MIB` is a Go runtime soft limit, not a hard RSS cap; container policy remains authoritative. The database pool is the exception because it protects a shared service: its production-safe default is 10 maximum and 0 minimum connections. The response buffer pool retains at most 4 MiB.

For a 512 MiB container, a reasonable starting point is `API_MAX_PROCS=2`, `API_MEMORY_LIMIT_MIB=384`, `API_DB_MAX_CONNS=4`, and `API_DB_STATEMENT_CACHE=64`. These are explicit limits, not CPU-ratio guesses.

## Telemetry

Prometheus reports bounded route-template labels for request count, duration, in-flight work, Go/process state, and PostgreSQL pool state. OTLP traces cover HTTP and PostgreSQL without SQL text, query parameters, connection details, search terms, or credentials.

Set `API_METRICS_ENABLED=false` to remove metrics routes. Leave `API_TRACING_ENABLED=false` for a zero-exporter, zero-egress tracing path. Exporter delivery errors are logged and do not change public API responses.

## Verification and performance

The quality gate runs formatting, module checks, `vet`, race tests, 100% coverage, OpenAPI validation, integration/E2E tests against canonical migrations, container checks, CodeQL, and vulnerability scanning.

Performance reports belong in [`docs/performance`](docs/performance). Each report must record the same data, hardware, concurrency, warm-up, and run count before and after, plus p50/p95/p99 latency, throughput, memory/allocation, and relevant DB query count or plan. No full-dataset claim is made without a representative dataset measurement.

Run `scripts/benchmark-http-api.sh` to exercise every public data operation in
OpenAPI with deterministic data and collect HTTP, Go runtime, and PostgreSQL
evidence without downloading a monthly dump.

## License

MIT
