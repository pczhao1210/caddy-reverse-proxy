#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TOKEN=${GATEWAY_ADMIN_TOKEN:-change-me}
COMPOSE="docker compose --env-file $ROOT_DIR/.env -f $ROOT_DIR/deploy/vm/docker-compose.yml"

if [ ! -f "$ROOT_DIR/.env" ]; then
  echo 'Missing .env. Run: cp .env.example .env' >&2
  exit 1
fi

cleanup() {
  $COMPOSE down >/dev/null 2>&1 || true
}
trap cleanup EXIT

$COMPOSE up --build -d

for _ in $(seq 1 60); do
  if curl -fsS http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"host":"e2e.localhost","exposure":"public","enabled":true,"https":false,"source":"explicit","upstreams":[{"name":"httpbin","url":"http://httpbin:8080","healthPath":"/status/200"}]}' \
  http://127.0.0.1:8080/api/routes >/dev/null

for _ in $(seq 1 30); do
  if curl -fsS -H 'Host: e2e.localhost' http://127.0.0.1/status/200 >/dev/null 2>&1; then
    echo 'E2E Caddy routing check passed'
    exit 0
  fi
  sleep 1
done

echo 'E2E Caddy routing check failed' >&2
exit 1
