# Architecture

The service has four runtime roles:

1. `catalog` owns API-domain request and response types plus consumer-focused read ports.
2. `httpapi` validates transport input, invokes catalog ports, and renders the OpenAPI contract.
3. `postgres` implements catalog ports with bounded, timeout-aware `pgx` queries.
4. `app`, `management`, `observability`, and `telemetry` compose process lifecycle and optional operational adapters.

The API does not import dumps or migrate the database. Import execution belongs to `go-open-discogs-batch`; canonical migrations belong to `open-discogs-model`; serving belongs here. A deployment account should have `CONNECT`, `USAGE`, and `SELECT` only.

Public and management listeners are separate. Liveness checks only the process. Readiness performs a bounded PostgreSQL ping. Prometheus is pull-based and local. OTLP tracing is opt-in and does not exist at runtime when disabled.

CPU and Go memory limits are optional: unset or zero means no application limit, negative values are rejected, and positive values are applied as configured. The Go memory value is a soft runtime limit. The database connection limit remains a production-safe finite value because it protects shared database capacity. The service does not derive any of these values from a percentage of CPU cores.
