# Performance reports

Do not publish unmeasured speedup claims. Every performance change must add or update a report with:

- commit and implementation under comparison;
- dataset identity and row counts;
- CPU architecture, CPU limit, memory limit, Go/JVM version, and PostgreSQL version;
- connection limits, concurrency, warm-up, duration, and repeated-run count;
- p50, p95, and p99 latency plus requests per second;
- RSS, Go heap, allocations per operation, and GC where relevant;
- SQL query count and `EXPLAIN (ANALYZE, BUFFERS)` evidence for changed queries;
- raw command and result artifact locations.

If representative measurement is impossible, state why and record the closest reproducible validation without extrapolating to the full dataset.

## OpenAPI HTTP benchmark

Run every public data operation against an isolated deterministic PostgreSQL
fixture:

```sh
API_PERF_MODEL_ROOT=../open-discogs-model \
  scripts/benchmark-http-api.sh
```

The runner derives the required operation inventory from OpenAPI and fails when
a data operation has no scenario. It records per-scenario p50, p95, p99,
throughput and failures plus RSS, Go runtime metrics, PostgreSQL statement
statistics, and fixture metadata. Docker storage is tmpfs; the exact labeled
container is removed and verified absent after success, failure, or interrupt.

Set `API_PERF_BASELINE` to a prior JSON result to compare p99. The comparison
requires identical fixture sizes, requests, warm-up, and concurrency. A single
short run is useful for diagnosis but is not a stable release gate.

Do not run the HTTP benchmark on every pull request. Shared CI runners do not
provide stable performance evidence and the fixture setup would add unnecessary
cost. Pull requests rely on the normal test and coverage gates. Run
`--validate-only` locally before a manual benchmark to verify the OpenAPI
operation inventory without issuing HTTP requests or opening a database.
