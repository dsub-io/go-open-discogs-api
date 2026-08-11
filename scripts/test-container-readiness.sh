#!/bin/sh

set -eu

readonly image="${1:?usage: test-container-readiness.sh IMAGE}"
readonly owner="go-open-discogs-api-readiness-$$"
readonly api_container="${owner}-api"
readonly database_container="${owner}-database"
readonly network="${owner}-network"
readonly postgres_image="postgres:18-alpine"
postgres_image_was_present=false

container_owner() {
  docker inspect \
    --format '{{index .Config.Labels "io.dsub.test-owner"}}' \
    "$1" 2>/dev/null || true
}

network_owner() {
  docker network inspect \
    --format '{{index .Labels "io.dsub.test-owner"}}' \
    "$1" 2>/dev/null || true
}

cleanup_container() {
  name="$1"
  if ! docker container inspect "$name" >/dev/null 2>&1; then
    return
  fi
  if [ "$(container_owner "$name")" != "$owner" ]; then
    printf 'Refusing to remove container with unexpected owner: %s\n' "$name" >&2
    return 1
  fi
  docker rm --force "$name" >/dev/null
  if docker container inspect "$name" >/dev/null 2>&1; then
    printf 'Container cleanup failed: %s\n' "$name" >&2
    return 1
  fi
}

cleanup() {
  status=0
  cleanup_container "$api_container" || status=1
  cleanup_container "$database_container" || status=1
  if docker network inspect "$network" >/dev/null 2>&1; then
    if [ "$(network_owner "$network")" != "$owner" ]; then
      printf 'Refusing to remove network with unexpected owner: %s\n' "$network" >&2
      status=1
    else
      docker network rm "$network" >/dev/null || status=1
    fi
  fi
  if docker network inspect "$network" >/dev/null 2>&1; then
    printf 'Network cleanup failed: %s\n' "$network" >&2
    status=1
  fi
  if [ "$postgres_image_was_present" = false ]; then
    docker image rm "$postgres_image" >/dev/null 2>&1 || true
  fi
  return "$status"
}

trap cleanup EXIT
trap 'exit 130' INT TERM

for resource in "$api_container" "$database_container"; do
  if docker container inspect "$resource" >/dev/null 2>&1; then
    printf 'Container already exists: %s\n' "$resource" >&2
    exit 1
  fi
done
if docker network inspect "$network" >/dev/null 2>&1; then
  printf 'Network already exists: %s\n' "$network" >&2
  exit 1
fi
if docker image inspect "$postgres_image" >/dev/null 2>&1; then
  postgres_image_was_present=true
fi

configured_healthcheck="$(
  API_DATABASE_URL='postgres://reader:secret@database:5432/discogs?sslmode=disable' \
    docker compose config --format json |
    jq -c '.services.api.healthcheck.test'
)"
if [ "$configured_healthcheck" != '["CMD","/go-open-discogs-api","--healthcheck"]' ]; then
  printf 'Unexpected Compose healthcheck: %s\n' "$configured_healthcheck" >&2
  exit 1
fi

docker network create \
  --label "io.dsub.test-owner=$owner" \
  "$network" >/dev/null

docker run \
  --detach \
  --name "$database_container" \
  --label "io.dsub.test-owner=$owner" \
  --network "$network" \
  --network-alias database \
  --tmpfs /var/lib/postgresql:rw,noexec,nosuid,size=512m \
  --env POSTGRES_USER=discogs \
  --env POSTGRES_PASSWORD=discogs \
  --env POSTGRES_DB=discogs \
  "$postgres_image" >/dev/null

volume_mounts="$(
  docker inspect \
    --format '{{range .Mounts}}{{if eq .Type "volume"}}{{println .Name}}{{end}}{{end}}' \
    "$database_container"
)"
if [ -n "$volume_mounts" ]; then
  printf 'Readiness test unexpectedly created Docker volumes: %s\n' "$volume_mounts" >&2
  exit 1
fi

ready=false
consecutive_ready=0
attempt=0
while [ "$attempt" -lt 200 ]; do
  if docker exec "$database_container" \
    pg_isready --username discogs --dbname discogs >/dev/null 2>&1; then
    consecutive_ready=$((consecutive_ready + 1))
    if [ "$consecutive_ready" -eq 5 ]; then
      ready=true
      break
    fi
  else
    consecutive_ready=0
  fi
  attempt=$((attempt + 1))
  sleep 0.1
done
if [ "$ready" = false ]; then
  docker logs "$database_container" >&2
  printf 'PostgreSQL readiness test container did not become ready.\n' >&2
  exit 1
fi

model_directory="$(go list -m -f '{{.Dir}}' github.com/dsub-io/open-discogs-model)"
for migration in "$model_directory"/schema/migrations/*.sql; do
  docker exec --interactive "$database_container" \
    psql --set ON_ERROR_STOP=1 --username discogs --dbname discogs < "$migration" >/dev/null
done

docker run \
  --detach \
  --name "$api_container" \
  --label "io.dsub.test-owner=$owner" \
  --network "$network" \
  --read-only \
  --user 65532:65532 \
  --env API_ADDRESS=:8080 \
  --env API_MANAGEMENT_ADDRESS=:8081 \
  --env API_DATABASE_URL='postgres://discogs:discogs@database:5432/discogs?sslmode=disable' \
  --env API_METRICS_ENABLED=false \
  "$image" >/dev/null

ready=false
attempt=0
while [ "$attempt" -lt 100 ]; do
  if docker exec "$api_container" \
    /go-open-discogs-api --healthcheck >/dev/null 2>&1; then
    ready=true
    break
  fi
  attempt=$((attempt + 1))
  sleep 0.1
done
if [ "$ready" = false ]; then
  docker logs "$api_container" >&2
  printf 'API container did not become ready.\n' >&2
  exit 1
fi

docker stop --time 1 "$database_container" >/dev/null
if docker exec "$api_container" \
  /go-open-discogs-api --healthcheck >/dev/null 2>&1; then
  printf 'Readiness probe succeeded after PostgreSQL stopped.\n' >&2
  exit 1
fi

printf 'Container readiness and dependency failure detection passed.\n'
