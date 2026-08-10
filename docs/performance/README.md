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
