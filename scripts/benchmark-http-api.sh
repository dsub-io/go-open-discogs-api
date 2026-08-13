#!/usr/bin/env bash

set -euo pipefail

readonly postgres_image="postgres:18-alpine"
readonly container_prefix="go-open-discogs-api-performance-postgres"
readonly test_owner="go-open-discogs-api-performance"
readonly default_artist_rows=50000
readonly default_label_rows=20000
readonly default_master_rows=50000
readonly default_release_rows=200000
readonly default_requests=200
readonly default_warmup=20
readonly default_concurrency=4
readonly postgres_tmpfs_size="4g"
readonly readiness_attempts=200
readonly readiness_delay_seconds="0.1"
readonly rss_sample_delay_seconds="0.1"

repository_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
container_name="${container_prefix}-$$"
temporary_root="$(mktemp -d "${TMPDIR:-/tmp}/go-open-discogs-api-performance.XXXXXX")"
result_directory="${API_PERF_RESULT_DIR:-${TMPDIR:-/tmp}/go-open-discogs-api-performance-results-$$}"
artist_rows="${API_PERF_ARTIST_ROWS:-$default_artist_rows}"
label_rows="${API_PERF_LABEL_ROWS:-$default_label_rows}"
master_rows="${API_PERF_MASTER_ROWS:-$default_master_rows}"
release_rows="${API_PERF_RELEASE_ROWS:-$default_release_rows}"
requests="${API_PERF_REQUESTS:-$default_requests}"
warmup="${API_PERF_WARMUP:-$default_warmup}"
concurrency="${API_PERF_CONCURRENCY:-$default_concurrency}"
baseline_path="${API_PERF_BASELINE:-}"
p99_regression_limit="${API_PERF_MAX_P99_REGRESSION_PERCENT:-20}"
p99_regression_min_ms="${API_PERF_MIN_P99_REGRESSION_MS:-1}"
api_pid=""
rss_monitor_pid=""
image_was_present=false

require_integer() {
  local name="$1"
  local value="$2"
  local minimum="$3"
  if [[ ! "$value" =~ ^[0-9]+$ ]] || (( value < minimum )); then
    printf '%s must be an integer greater than or equal to %d: %s\n' "$name" "$minimum" "$value" >&2
    exit 1
  fi
}

cleanup() {
  local cleanup_failed=false
  if [[ -n "$rss_monitor_pid" ]]; then
    kill "$rss_monitor_pid" >/dev/null 2>&1 || true
    wait "$rss_monitor_pid" >/dev/null 2>&1 || true
  fi
  if [[ -n "$api_pid" ]]; then
    kill -TERM "$api_pid" >/dev/null 2>&1 || true
    wait "$api_pid" >/dev/null 2>&1 || true
  fi
  case "$container_name" in
    go-open-discogs-api-performance-postgres-[0-9]*) ;;
    *)
      printf 'Refusing to clean unexpected container name: %s\n' "$container_name" >&2
      cleanup_failed=true
      ;;
  esac
  if [[ "$cleanup_failed" = false ]]; then
    docker rm --force "$container_name" >/dev/null 2>&1 || true
    if docker ps -a --filter "name=^/${container_name}$" --format '{{.Names}}' | grep -q .; then
      printf 'Performance container cleanup failed: %s\n' "$container_name" >&2
      cleanup_failed=true
    fi
  fi
  if [[ "$image_was_present" = false ]]; then
    docker image rm "$postgres_image" >/dev/null 2>&1 || true
  fi
  if [[ "${API_PERF_KEEP_TEMP:-false}" != true ]]; then
    rm -rf "$temporary_root"
  else
    printf 'Temporary files retained at %s\n' "$temporary_root"
  fi
  if [[ "$cleanup_failed" = true ]]; then
    return 1
  fi
}

trap cleanup EXIT
trap 'exit 130' INT TERM

require_integer API_PERF_ARTIST_ROWS "$artist_rows" 2000
require_integer API_PERF_LABEL_ROWS "$label_rows" 100
require_integer API_PERF_MASTER_ROWS "$master_rows" 100
require_integer API_PERF_RELEASE_ROWS "$release_rows" 2000
require_integer API_PERF_REQUESTS "$requests" 1
require_integer API_PERF_WARMUP "$warmup" 0
require_integer API_PERF_CONCURRENCY "$concurrency" 1

for command_name in docker go curl jq awk ps; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Required command is unavailable: %s\n' "$command_name" >&2
    exit 1
  fi
done

model_root="${API_PERF_MODEL_ROOT:-$(go list -m -f '{{.Dir}}' github.com/dsub-io/open-discogs-model)}"
migration_directory="$model_root/schema/migrations"
if [[ ! -d "$migration_directory" ]]; then
  printf 'Canonical model migrations are unavailable: %s\n' "$migration_directory" >&2
  exit 1
fi

mkdir -p "$result_directory"
result_directory="$(cd "$result_directory" && pwd)"

if docker image inspect "$postgres_image" >/dev/null 2>&1; then
  image_was_present=true
fi

docker run \
  --detach \
  --name "$container_name" \
  --label "io.dsub.test-owner=$test_owner" \
  --tmpfs "/var/lib/postgresql:rw,noexec,nosuid,size=$postgres_tmpfs_size" \
  --env POSTGRES_USER=discogs \
  --env POSTGRES_PASSWORD=discogs \
  --env POSTGRES_DB=discogs \
  --publish 127.0.0.1::5432 \
  "$postgres_image" \
  -c shared_preload_libraries=pg_stat_statements \
  -c track_io_timing=on >/dev/null

volume_mounts="$(
  docker inspect \
    --format '{{range .Mounts}}{{if eq .Type "volume"}}{{println .Name}}{{end}}{{end}}' \
    "$container_name"
)"
if [[ -n "$volume_mounts" ]]; then
  printf 'Performance container unexpectedly created Docker volumes: %s\n' "$volume_mounts" >&2
  exit 1
fi

database_ready=false
for ((attempt = 0; attempt < readiness_attempts; attempt++)); do
  if docker exec "$container_name" \
    psql --no-psqlrc --quiet --username discogs --dbname discogs \
      --command 'select 1' >/dev/null 2>&1; then
    database_ready=true
    break
  fi
  sleep "$readiness_delay_seconds"
done
if [[ "$database_ready" = false ]]; then
  docker logs "$container_name" >&2
  printf 'PostgreSQL performance container did not become ready.\n' >&2
  exit 1
fi

host_port="$(docker port "$container_name" 5432/tcp | sed 's/.*://')"
if [[ -z "$host_port" ]]; then
  printf 'Unable to resolve PostgreSQL performance port.\n' >&2
  exit 1
fi

for migration in "$migration_directory"/*.sql; do
  docker exec --interactive "$container_name" \
    psql --no-psqlrc --quiet --set ON_ERROR_STOP=1 \
      --username discogs --dbname discogs < "$migration"
done

docker exec "$container_name" \
  psql --no-psqlrc --quiet --set ON_ERROR_STOP=1 \
    --username discogs --dbname discogs \
    --command 'create extension if not exists pg_stat_statements' >/dev/null

printf 'Seeding deterministic fixture: artists=%s labels=%s masters=%s releases=%s\n' \
  "$artist_rows" "$label_rows" "$master_rows" "$release_rows"
docker exec --interactive "$container_name" \
  psql --no-psqlrc --quiet --set ON_ERROR_STOP=1 \
    --set "artist_rows=$artist_rows" \
    --set "label_rows=$label_rows" \
    --set "master_rows=$master_rows" \
    --set "release_rows=$release_rows" \
    --username discogs --dbname discogs < "$repository_root/scripts/api-performance-fixture.sql"

allocate_port() {
  go run "$repository_root/scripts/free-port.go"
}

public_port="$(allocate_port)"
management_port="$(allocate_port)"
if [[ "$public_port" = "$management_port" ]]; then
  management_port="$(allocate_port)"
fi
public_url="http://127.0.0.1:$public_port"
management_url="http://127.0.0.1:$management_port"
database_url="postgres://discogs:discogs@127.0.0.1:${host_port}/discogs?sslmode=disable"

go build -trimpath -o "$temporary_root/open-discogs-api" .
API_ADDRESS="127.0.0.1:$public_port" \
API_MANAGEMENT_ADDRESS="127.0.0.1:$management_port" \
API_SERVER_URL="$public_url" \
API_DATABASE_URL="$database_url" \
API_DATABASE_SCHEMA=public \
API_DB_MAX_CONNS="$concurrency" \
API_METRICS_ENABLED=true \
API_TRACING_ENABLED=false \
API_ACCESS_LOG=false \
  "$temporary_root/open-discogs-api" >"$result_directory/api.log" 2>&1 &
api_pid=$!

api_ready=false
for ((attempt = 0; attempt < readiness_attempts; attempt++)); do
  if curl --fail --silent --show-error "$management_url/healthz" >/dev/null 2>&1 && \
     curl --fail --silent --show-error "$public_url/version" >/dev/null 2>&1; then
    api_ready=true
    break
  fi
  if ! kill -0 "$api_pid" >/dev/null 2>&1; then
    break
  fi
  sleep "$readiness_delay_seconds"
done
if [[ "$api_ready" = false ]]; then
  sed -n '1,240p' "$result_directory/api.log" >&2
  printf 'OpenDiscogs API did not become ready.\n' >&2
  exit 1
fi

(
  while kill -0 "$api_pid" >/dev/null 2>&1; do
    ps -o rss= -p "$api_pid" | awk -v timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      '{$1=$1; if ($1 != "") print timestamp "\t" $1}'
    sleep "$rss_sample_delay_seconds"
  done
) >"$result_directory/rss-kib.tsv" &
rss_monitor_pid=$!

docker exec "$container_name" \
  psql --no-psqlrc --quiet --username discogs --dbname discogs \
    --command 'select pg_stat_statements_reset()' >/dev/null

load_command=(
  go run "$repository_root/scripts/api-performance-load.go"
  --base-url "$public_url"
  --openapi "$repository_root/internal/httpapi/openapi.json"
  --output "$result_directory/http-results.json"
  --requests "$requests"
  --warmup "$warmup"
  --concurrency "$concurrency"
  --artist-rows "$artist_rows"
  --label-rows "$label_rows"
  --master-rows "$master_rows"
  --release-rows "$release_rows"
)
if [[ -n "$baseline_path" ]]; then
  load_command+=(
    --baseline "$baseline_path"
    --max-p99-regression-percent "$p99_regression_limit"
    --min-p99-regression-ms "$p99_regression_min_ms"
  )
fi

set +e
"${load_command[@]}" | tee "$result_directory/http-results.tsv"
load_status="${PIPESTATUS[0]}"
set -e

curl --fail --silent --show-error "$management_url/metrics" > "$result_directory/prometheus.txt"

docker exec "$container_name" \
  psql --no-psqlrc --no-align --tuples-only --field-separator=$'\t' \
    --username discogs --dbname discogs \
    --command "
      select calls,
             round(total_exec_time::numeric, 3),
             round(mean_exec_time::numeric, 3),
             rows,
             shared_blks_hit,
             shared_blks_read,
             temp_blks_read,
             temp_blks_written,
             regexp_replace(query, E'[\\n\\r\\t ]+', ' ', 'g')
      from pg_stat_statements
      where query not like '%pg_stat_statements%'
      order by total_exec_time desc
      limit 30
    " > "$result_directory/postgres-statements.tsv"

peak_rss_kib="$(awk 'BEGIN { peak=0 } $2 > peak { peak=$2 } END { print peak }' "$result_directory/rss-kib.tsv")"
go_heap_bytes="$(awk '/^go_memstats_heap_alloc_bytes / {print $2}' "$result_directory/prometheus.txt")"
go_allocated_bytes_total="$(awk '/^go_memstats_alloc_bytes_total / {print $2}' "$result_directory/prometheus.txt")"
go_mallocs_total="$(awk '/^go_memstats_mallocs_total / {print $2}' "$result_directory/prometheus.txt")"
go_gc_seconds="$(awk '/^go_gc_duration_seconds_sum / {print $2}' "$result_directory/prometheus.txt")"
process_cpu_seconds="$(awk '/^process_cpu_seconds_total / {print $2}' "$result_directory/prometheus.txt")"
database_size_bytes="$(docker exec "$container_name" psql --no-psqlrc --quiet --tuples-only --username discogs --dbname discogs --command 'select pg_database_size(current_database())' | tr -d '[:space:]')"

{
  printf 'result_directory\t%s\n' "$result_directory"
  printf 'go_version\t%s\n' "$(go version)"
  printf 'postgres_image\t%s\n' "$postgres_image"
  printf 'hardware\t%s %s\n' "$(uname -m)" "$(uname -s)"
  printf 'artist_rows\t%s\n' "$artist_rows"
  printf 'label_rows\t%s\n' "$label_rows"
  printf 'master_rows\t%s\n' "$master_rows"
  printf 'release_rows\t%s\n' "$release_rows"
  printf 'requests_per_scenario\t%s\n' "$requests"
  printf 'warmup_requests\t%s\n' "$warmup"
  printf 'concurrency\t%s\n' "$concurrency"
  printf 'peak_rss_kib\t%s\n' "$peak_rss_kib"
  printf 'go_heap_alloc_bytes\t%s\n' "$go_heap_bytes"
  printf 'go_allocated_bytes_total\t%s\n' "$go_allocated_bytes_total"
  printf 'go_mallocs_total\t%s\n' "$go_mallocs_total"
  printf 'go_gc_duration_seconds\t%s\n' "$go_gc_seconds"
  printf 'process_cpu_seconds_total\t%s\n' "$process_cpu_seconds"
  printf 'database_size_bytes\t%s\n' "$database_size_bytes"
} | tee "$result_directory/metadata.tsv"

printf 'Performance artifacts: %s\n' "$result_directory"
if [[ "$load_status" -ne 0 ]]; then
  exit "$load_status"
fi
