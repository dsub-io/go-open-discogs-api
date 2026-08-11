# Cursor query scalability benchmark (2026-08-11)

## Result

The collection contract now uses ascending-ID keyset pagination, fetches one
bounded look-ahead row, and omits exact totals. The matching canonical model
migration adds indexes for supported filters and reverse relationships.

| Path | Before p50 / p95 / p99 | After p50 / p95 / p99 | p95 change |
| --- | ---: | ---: | ---: |
| Deep release page | 165.913 / 183.106 / 184.574 ms | 0.032 / 0.038 / 0.044 ms | 99.979% lower (4,818.6x) |
| Release title substring | 163.887 / 194.535 / 199.306 ms | 0.111 / 0.136 / 0.142 ms | 99.930% lower (1,430.4x) |
| Releases by artist | 16.717 / 17.309 / 22.187 ms | 0.040 / 0.061 / 0.070 ms | 99.648% lower (283.8x) |

The old collection path also executed an exact count query, measured separately
at 34.572 / 35.706 / 35.725 ms. The new path reduces database query count from
two to one and performs no count. The title plan changed from a parallel scan
that rejected 999,997 rows to a trigram bitmap scan; the artist relationship
plan changed from a parallel sequential scan of 1,000,000 rows to an index-only
scan.

All V007 indexes increased the synthetic database from 314,308,287 to
486,389,439 bytes: 164.1 MiB, or 54.7%. The storage cost belongs in capacity
planning alongside the latency improvement.

## Conditions and reproduction

- Apple M2 Pro host, arm64, 12 Docker CPUs, 8 GiB Docker memory
- PostgreSQL 18.4 Alpine on a 4 GiB tmpfs; no persistent Docker volume
- 1,000,000 releases, 1,000,000 release-artist relations, 250,000 artists,
  250,000 masters, and 100,000 labels
- one client, JIT disabled, five warm-up executions, then 30 measured executions
- identical data and container for both phases, with `ANALYZE` before each phase

The benchmark is owned by `open-discogs-model`, which owns the schema and index
contract:

```sh
../open-discogs-model/scripts/benchmark-api-query-indexes.sh
```

The API suite separately verifies the one-query cursor behavior against a real
PostgreSQL instance:

```sh
./scripts/test-integration.sh -race -coverpkg=./... \
  -covermode=atomic -coverprofile=coverage.out ./...
```

## Limits

This is a warm-cache synthetic query benchmark, not a 200-million-row or
full-dump capacity claim. Concurrent throughput, cold storage I/O, import
duration, RSS, Go heap and allocation changes, and production index size were
not measured. Query count and PostgreSQL execution plans are the relevant
resource evidence for this database-bound change. The omitted dimensions remain
mandatory in the full-dump pre-production rehearsal.
