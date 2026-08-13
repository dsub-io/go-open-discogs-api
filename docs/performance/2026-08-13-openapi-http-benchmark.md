# OpenAPI HTTP benchmark (2026-08-13)

## Result

The benchmark covers every one of the 11 public data operations in
`internal/httpapi/openapi.json`. Sixteen scenarios separate cursor, text,
numeric, combined-filter, relationship, and rich-detail paths.

| Scenario | p50 ms | p95 ms | p99 ms | requests/s |
| --- | ---: | ---: | ---: | ---: |
| `artists-page-deep` | 0.712 | 1.304 | 7.512 | 4,354 |
| `artists-search-name` | 2.351 | 2.903 | 3.186 | 1,621 |
| `artists-search-real-name` | 2.707 | 3.377 | 3.716 | 1,430 |
| `artist-detail-rich` | 1.255 | 2.311 | 5.787 | 2,782 |
| `artist-releases-deep` | 1.587 | 3.263 | 3.962 | 2,237 |
| `labels-search-name` | 1.386 | 2.179 | 2.524 | 2,611 |
| `label-detail-rich` | 1.187 | 3.994 | 4.908 | 2,239 |
| `label-releases-deep` | 1.455 | 2.080 | 2.693 | 2,592 |
| `masters-search-title` | 2.275 | 2.973 | 3.324 | 1,669 |
| `masters-search-year` | 0.680 | 1.005 | 1.830 | 5,402 |
| `master-detail-rich` | 0.903 | 1.471 | 2.407 | 4,014 |
| `master-releases-deep` | 1.128 | 1.467 | 1.757 | 3,348 |
| `releases-page-deep` | 0.851 | 1.305 | 1.649 | 4,342 |
| `releases-search-title` | 1.617 | 2.029 | 2.242 | 2,390 |
| `releases-search-combined` | 1.019 | 1.591 | 2.568 | 3,571 |
| `release-detail-rich` | 1.816 | 3.908 | 5.375 | 1,879 |

No measured request failed.

Two query changes were made from the first identical-condition run:

| Changed path | Before | After | Change |
| --- | ---: | ---: | ---: |
| Artist releases p50 | 2.627 ms | 1.587 ms | 39.6% lower |
| Artist releases p95 | 3.474 ms | 3.263 ms | 6.1% lower |
| Artist releases p99 | 4.698 ms | 3.962 ms | 15.7% lower |
| Artist releases throughput | 1,441 requests/s | 2,237 requests/s | 55.3% higher |
| Combined release filter p50 | 7.867 ms | 1.019 ms | 87.0% lower |
| Combined release filter p95 | 36.932 ms | 1.591 ms | 95.7% lower |
| Combined release filter p99 | 57.608 ms | 2.568 ms | 95.5% lower |
| Combined release filter throughput | 374 requests/s | 3,571 requests/s | 855.3% higher |

The artist relationship query now finds the bounded candidate release IDs
before merging roles. Across 220 warm-up and measured calls, PostgreSQL mean
execution time changed from 1.645 to 0.214 ms and shared-buffer hits changed
from 268,629 to 73,929; no temporary blocks were used.

The combined release filter uses canonical model migration
`V022__release_combined_filter_index.sql`. Its isolated 1,000,000-release
result is recorded in the model repository performance report.

## Conditions

- Apple M2 Pro, 12 cores, 32 GiB host memory
- Go 1.26.5 darwin/arm64
- PostgreSQL 18.4 Alpine, 4 GiB tmpfs, no persistent Docker volume
- deterministic fixture: 50,000 artists, 20,000 labels, 50,000 masters, and
  200,000 releases
- four HTTP workers and four PostgreSQL connections
- 20 warm-up requests followed by 200 measured requests per scenario
- metrics and `pg_stat_statements` enabled; tracing and access logs disabled
- warm database cache; scenarios run sequentially and requests within each
  scenario run concurrently

The final process peak RSS was 28,752 KiB. The final scrape reported
2,298,920 Go heap bytes, 102,507,712 cumulative allocated bytes, 1,917,127
cumulative allocations, 0.0070 seconds of GC pause time, and 0.965 process CPU
seconds. The synthetic database was 175,331,007 bytes.

Run from the API repository root while using a model checkout containing V022:

```sh
API_PERF_MODEL_ROOT=../open-discogs-model \
  scripts/benchmark-http-api.sh
```

The run writes bounded JSON and TSV HTTP results, PostgreSQL statement totals,
Prometheus metrics, RSS samples, API logs, and condition metadata to the path
printed at completion. Set `API_PERF_RESULT_DIR` to retain them at a chosen
location. `API_PERF_BASELINE` optionally enables the p99 regression comparison;
use enough samples and repeated runs before treating a low-millisecond tail
change as a release gate.

This benchmark is intentionally manual and must not run on every pull request.
Pull requests use the normal test and coverage gates. The local
`--validate-only` mode starts no database or API and issues no load.

## Limits

This is a synthetic warm-cache API benchmark, not a 200-million-row production
capacity claim. It validates route inventory, bounded query behavior, HTTP
serialization, pool concurrency, and telemetry collection without requiring a
monthly dump. Cold storage, production data distribution, and sustained mixed
traffic require a separate pre-production rehearsal.
