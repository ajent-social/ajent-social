#!/usr/bin/env bash
# Exercise a built production image against real PostgreSQL without publishing it.
set -euo pipefail
image=${1:?Usage: scripts/container-smoke.sh IMAGE}
engine=${CONTAINER_ENGINE:-docker}
name="ajent-smoke-$(date +%s)-$$"
# Invoked by EXIT trap; older ShellCheck versions do not resolve that call.
# shellcheck disable=SC2329,SC2317
cleanup() {
  "$engine" rm -f "$name-app" "$name-db" >/dev/null 2>&1 || true
  "$engine" network rm "$name" >/dev/null 2>&1 || true
}
trap cleanup EXIT
"$engine" network create "$name" >/dev/null
"$engine" run -d --name "$name-db" --network "$name" -e POSTGRES_PASSWORD=test -e POSTGRES_DB=ajent docker.io/library/postgres:16-alpine >/dev/null
for _ in {1..60}; do
  if "$engine" exec "$name-db" pg_isready -U postgres >/dev/null 2>&1; then break; fi
  sleep 1
done
"$engine" exec "$name-db" pg_isready -U postgres >/dev/null
"$engine" run -d --name "$name-app" --network "$name" --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  -e "AJENT_DSN=postgres://postgres:test@$name-db:5432/ajent?sslmode=disable" \
  -e AJENT_SETUP_TOKEN=container-smoke-only-owner-key-not-for-deployment \
  -e AJENT_ORIGIN=https://dogfood.invalid "$image" >/dev/null
for _ in {1..60}; do
  if "$engine" exec "$name-app" /app/ajentd -healthcheck >/dev/null 2>&1; then
    count=$("$engine" exec "$name-db" psql -U postgres -d ajent -Atc 'SELECT count(*) FROM schema_migrations')
    test "$count" -ge 5
    echo "Production image healthy; $count migrations applied."
    exit 0
  fi
  sleep 1
done
"$engine" logs "$name-app"
exit 1
