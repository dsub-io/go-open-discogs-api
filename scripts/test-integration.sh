#!/bin/sh

set -eu

readonly postgres_image="postgres:18-alpine"
readonly container_name="go-open-discogs-api-test-postgres-$$"
image_was_present=false

cleanup() {
  case "$container_name" in
    go-open-discogs-api-test-postgres-[0-9]*) ;;
    *)
      printf 'Refusing to clean unexpected container name: %s\n' "$container_name" >&2
      return 1
      ;;
  esac

  docker rm -f "$container_name" >/dev/null 2>&1 || true
  if docker ps -a --filter "name=^/${container_name}$" --format '{{.Names}}' | grep -q .; then
    printf 'Test container cleanup failed: %s\n' "$container_name" >&2
    return 1
  fi
  if [ "$image_was_present" = false ]; then
    docker image rm "$postgres_image" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT
trap 'exit 130' INT TERM

if docker image inspect "$postgres_image" >/dev/null 2>&1; then
  image_was_present=true
fi

docker run \
  --detach \
  --name "$container_name" \
  --label io.dsub.test-owner=go-open-discogs-api \
  --tmpfs /var/lib/postgresql:rw,noexec,nosuid,size=1g \
  --env POSTGRES_USER=discogs \
  --env POSTGRES_PASSWORD=discogs \
  --env POSTGRES_DB=discogs \
  --publish 127.0.0.1::5432 \
  "$postgres_image" >/dev/null

volume_mounts="$(
  docker inspect \
    --format '{{range .Mounts}}{{if eq .Type "volume"}}{{println .Name}}{{end}}{{end}}' \
    "$container_name"
)"
if [ -n "$volume_mounts" ]; then
  printf 'Test container unexpectedly created Docker volumes: %s\n' "$volume_mounts" >&2
  exit 1
fi

ready=false
attempt=0
while [ "$attempt" -lt 100 ]; do
  if docker exec "$container_name" pg_isready --username discogs --dbname discogs >/dev/null 2>&1; then
    ready=true
    break
  fi
  attempt=$((attempt + 1))
  sleep 0.1
done
if [ "$ready" = false ]; then
  docker logs "$container_name" >&2
  printf 'PostgreSQL test container did not become ready.\n' >&2
  exit 1
fi

host_port="$(docker port "$container_name" 5432/tcp | sed 's/.*://')"
if [ -z "$host_port" ]; then
  printf 'Unable to resolve PostgreSQL test port.\n' >&2
  exit 1
fi

if [ "$#" -eq 0 ]; then
  set -- ./...
fi

TEST_DATABASE_URL="postgres://discogs:discogs@127.0.0.1:${host_port}/discogs?sslmode=disable" \
  go test -count=1 "$@"
