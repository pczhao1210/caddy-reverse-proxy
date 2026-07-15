#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/.env}
case "$ENV_FILE" in
  /*) ;;
  *) ENV_FILE="$ROOT_DIR/$ENV_FILE" ;;
esac

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing environment file: $ENV_FILE" >&2
  exit 1
fi

TOKEN=${GATEWAY_ADMIN_TOKEN:-$(sed -n 's/^GATEWAY_ADMIN_TOKEN=//p' "$ENV_FILE" | tail -n 1)}

compose() {
  docker compose --env-file "$ENV_FILE" -f "$ROOT_DIR/deploy/vm/docker-compose.yml" "$@"
}

cleanup() {
  compose down --volumes >/dev/null 2>&1 || true
}
trap cleanup EXIT

cleanup
compose up --build -d --wait --wait-timeout 120

curl -fsS \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"host":"e2e.localhost","exposure":"public","enabled":true,"https":false,"source":"explicit","upstreams":[{"name":"httpbin","url":"http://httpbin:8080","healthPath":"/status/200"}]}' \
  http://127.0.0.1:8080/api/routes >/dev/null

curl -fsS --retry 30 --retry-all-errors --retry-delay 1 \
  -H 'Host: e2e.localhost' \
  http://127.0.0.1/status/200 >/dev/null

echo 'E2E Caddy routing check passed'
