# Changelog

## [1.2.0](https://github.com/dsub-io/go-open-discogs-api/compare/v1.1.1...v1.2.0) (2026-08-14)


### Features

* expand catalog query API ([#12](https://github.com/dsub-io/go-open-discogs-api/issues/12)) ([b58dce2](https://github.com/dsub-io/go-open-discogs-api/commit/b58dce22efeb866a4c67fbdcc4ba2e3f2b6efe9d))

## [1.1.1](https://github.com/dsub-io/go-open-discogs-api/compare/v1.1.0...v1.1.1) (2026-08-13)


### Bug Fixes

* make readiness follow canonical catalog bootstrap and refresh state instead of database connectivity alone ([#10](https://github.com/dsub-io/go-open-discogs-api/issues/10)) ([66ccd93](https://github.com/dsub-io/go-open-discogs-api/commit/66ccd933dc43805c88daa1e56205b6cfdc40aae5))
* preserve collision-safe relation identities, multiple label catalog numbers, oversized format quantities, and unordered relation semantics
* bound artist-release role aggregation after the cursor and consume `open-discogs-model` `v0.3.2`


### Performance Improvements

* on the documented 50,000-artist / 200,000-release fixture at concurrency 4, artist-release HTTP p99 fell from `4.698 ms` to `3.962 ms` (`15.7%` lower) while throughput rose from `1,441` to `2,237 requests/s` (`55.2%` higher)
* PostgreSQL mean execution time for that path fell from `1.645 ms` to `0.214 ms` (`87.0%` lower)
* after canonical V022, combined release-filter HTTP p99 fell from `57.608 ms` to `2.568 ms` (`95.5%` lower); model-owned SQL p99 fell from `6.856 ms` to `0.225 ms` (`96.7%` lower) at an index-size cost of `31,522,816 bytes` (`6.5%` growth on the post-V007 fixture)


### Validation and Upgrade

* validate all 11 public OpenAPI operations through 16 manual HTTP scenarios with zero failures
* pass race-enabled tests, `100.0%` aggregate statement coverage, CodeQL, vulnerability checks, Compose/Kubernetes validation, and container readiness/dependency-failure tests without Docker residue
* the API remains read-only and never migrates; deploy `open-discogs-model` `v0.3.2` V022 through either batch before starting this version

## [1.1.0](https://github.com/dsub-io/go-open-discogs-api/compare/v1.0.0...v1.1.0) (2026-08-11)


### Features

* add operator-selected PostgreSQL schemas through `--database-schema` and `API_DATABASE_SCHEMA` ([e2a9b7c](https://github.com/dsub-io/go-open-discogs-api/commit/e2a9b7c871143f662a048f876e30f9e40a7fe2c3))
* validate schema existence, `USAGE`, required tables, and `SELECT` privileges before opening listeners
* retain `public` as the compatibility default while warning on every startup and keeping the API read-only


### Bug Fixes

* download and validate canonical model migrations before fresh-runner readiness tests ([bf6ab76](https://github.com/dsub-io/go-open-discogs-api/commit/bf6ab7612583affd3de9656169bd877b11f23b85))
* reject out-of-range database connection counts before narrowing to pgx `int32`, resolving CodeQL CWE-190 and CWE-681 ([7d83a61](https://github.com/dsub-io/go-open-discogs-api/commit/7d83a614404d54ea39811f80109eb525776f53c9))

## 1.0.0 (2026-08-11)


### Features

* provide a read-only Go API for Artist, Label, Master, and Release data imported from Discogs monthly public dumps, with a validated OpenAPI contract ([1698e9f](https://github.com/dsub-io/go-open-discogs-api/commit/1698e9f97d9d7f2475e885693224771c925f1148))
* expose matching typed CLI and environment configuration, separate public and management listeners, optional metrics and tracing, graceful shutdown, Docker Compose, and Kubernetes examples


### Bug Fixes

* make the container healthcheck probe database-backed `/readyz`, report unhealthy after PostgreSQL becomes unavailable, and validate exact Docker resource cleanup ([1efe561](https://github.com/dsub-io/go-open-discogs-api/commit/1efe5616c57960e14cd6e13319ee07eb2847afff))


### Performance Improvements

* replace offset pagination and exact totals with ascending-ID keyset pagination, one bounded look-ahead row, and one database query per collection request ([4c19830](https://github.com/dsub-io/go-open-discogs-api/commit/4c19830a82d684ded71e39140c4faf8265d6bae9))
* on the same warm-cache PostgreSQL 18.4 synthetic dataset, deep release pagination p95 fell from `183.106 ms` to `0.038 ms` (`99.979%` lower, `4,818.6x` faster), title-contains search p95 from `194.535 ms` to `0.136 ms` (`99.930%` lower, `1,430.4x` faster), and artist-relation lookup p95 from `17.309 ms` to `0.061 ms` (`99.648%` lower, `283.8x` faster)
* eliminate the old exact-count query (`35.706 ms` p95 in the benchmark) and reduce collection query count from two to one

### Schema, Validation, and Distribution

* validate against `open-discogs-model` `v0.2.2` and its `V007` indexes; the API remains read-only and never runs schema migrations
* all local and CI Go suites pass with race detection and `100.0%` aggregate statement coverage; readiness validation uses tmpfs and leaves no test containers, networks, or volumes
* publish binaries plus non-root `linux/amd64` and `linux/arm64` GHCR images with provenance, SBOM, and post-publish architecture verification
* the indexed synthetic database grew by `164.1 MiB` (`54.7%`); full 200M+ dump import duration, production index size, cold-I/O behavior, concurrent throughput, RSS, heap, and allocations remain pre-production measurements rather than inferred claims
