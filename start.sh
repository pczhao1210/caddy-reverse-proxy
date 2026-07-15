#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
IMAGE=${IMAGE:-pczhao1210/caddy-reverse-proxy:latest}
PUSH_IMAGE=${PUSH_IMAGE:-}
CONTAINER_NAME=${CONTAINER_NAME:-caddy-reverse-proxy}
DATA_DIR=${DATA_DIR:-"$HOME/docker_files/caddy-reverse-proxy"}
HTTP_PORT=${HTTP_PORT:-80}
HTTPS_PORT=${HTTPS_PORT:-443}
MANAGEMENT_PORT=${MANAGEMENT_PORT:-8080}
DOCKER_NETWORKS=${DOCKER_NETWORKS:-}
WAIT_TIMEOUT=${WAIT_TIMEOUT:-120}

if [ -n "${ENV_FILE:-}" ]; then
  case "$ENV_FILE" in
    /*) ;;
    *) ENV_FILE="$ROOT_DIR/$ENV_FILE" ;;
  esac
elif [ -f "$ROOT_DIR/.env" ]; then
  ENV_FILE="$ROOT_DIR/.env"
else
  ENV_FILE="$ROOT_DIR/.env.example"
fi

usage() {
  cat <<EOF
Usage: ./start.sh <command>

Commands:
  build, --build      Build the local gateway image
  push, --push        Push the image to Docker Hub
  start, --start      Start one gateway container and wait for readiness
  stop, --stop        Stop the gateway container and preserve data
  restore, --restore  Remove the container, persisted data directory, and image
  help, --help        Show this help

Environment:
  IMAGE             Gateway image (default: pczhao1210/caddy-reverse-proxy:latest)
  PUSH_IMAGE        Docker Hub image name (default: logged-in-user/IMAGE)
  ENV_FILE          Optional environment file (default: .env, then .env.example)
  CONTAINER_NAME    Container name (default: caddy-reverse-proxy)
  DATA_DIR          Persistent data (default: ~/docker_files/caddy-reverse-proxy)
  HTTP_PORT         Host HTTP port (default: 80)
  HTTPS_PORT        Host HTTPS port (default: 443)
  MANAGEMENT_PORT   Loopback-only Console port (default: 8080)
  DOCKER_NETWORKS   Optional comma-separated workload networks
  WAIT_TIMEOUT      Readiness timeout in seconds (default: 120)

Docker Hub example:
  ./start.sh push
  PUSH_IMAGE=pczhao1210/caddy-reverse-proxy:latest ./start.sh push
EOF
}

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

require_docker() {
  command -v docker >/dev/null 2>&1 || fail "Docker is not installed or not in PATH."
  docker info >/dev/null 2>&1 || fail "Docker daemon is not reachable."
}

ensure_image() {
  if docker image inspect "$IMAGE" >/dev/null 2>&1; then
    return
  fi
  case "$IMAGE" in
    */*)
      printf 'Pulling %s...\n' "$IMAGE"
      docker pull "$IMAGE"
      ;;
    *)
      printf 'Building %s...\n' "$IMAGE"
      docker build -f "$ROOT_DIR/backend/Dockerfile" -t "$IMAGE" "$ROOT_DIR"
      ;;
  esac
}

validate_data_dir() {
  command -v realpath >/dev/null 2>&1 || fail "realpath is required to validate DATA_DIR."
  data_root=$(realpath -m -- "$HOME/docker_files")
  resolved_data_dir=$(realpath -m -- "$DATA_DIR")
  case "$resolved_data_dir" in
    "$data_root"/*) ;;
    *) fail "DATA_DIR must resolve below $data_root so restore cannot remove an unrelated path." ;;
  esac
  [ "$resolved_data_dir" != "$data_root" ] || fail "DATA_DIR cannot be $data_root."
  DATA_DIR=$resolved_data_dir
}

prepare_data_dir() {
  validate_data_dir
  mkdir -p "$DATA_DIR/platform" "$DATA_DIR/caddy"
  chmod 700 "$DATA_DIR" "$DATA_DIR/platform"
}

resolve_admin_token() {
  ADMIN_TOKEN=$(sed -n 's/^GATEWAY_ADMIN_TOKEN=//p' "$ENV_FILE" | tail -n 1)
  token_file="$DATA_DIR/platform/admin-token"
  if [ -z "$ADMIN_TOKEN" ] || [ "$ADMIN_TOKEN" = "change-me" ]; then
    if [ -f "$token_file" ]; then
      ADMIN_TOKEN=$(sed -n '1p' "$token_file")
    else
      ADMIN_TOKEN=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
      umask 077
      printf '%s\n' "$ADMIN_TOKEN" >"$token_file"
    fi
  fi
}

stop_container() {
  if docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
    docker rm -f "$CONTAINER_NAME" >/dev/null
  fi
}

validate_networks() {
  [ -z "$DOCKER_NETWORKS" ] && return
  old_ifs=$IFS
  IFS=,
  set -- $DOCKER_NETWORKS
  IFS=$old_ifs
  for network in "$@"; do
    docker network inspect "$network" >/dev/null 2>&1 || fail "Docker network does not exist: $network"
  done
}

connect_networks() {
  [ -z "$DOCKER_NETWORKS" ] && return
  old_ifs=$IFS
  IFS=,
  set -- $DOCKER_NETWORKS
  IFS=$old_ifs
  for network in "$@"; do
    if ! docker network connect "$network" "$CONTAINER_NAME"; then
      stop_container
      fail "Failed to connect $CONTAINER_NAME to Docker network $network."
    fi
  done
}

wait_for_readiness() {
  started_at=$(date +%s)
  while :; do
    if docker exec "$CONTAINER_NAME" wget -q -T 2 -O /dev/null http://127.0.0.1:8080/readyz; then
      return
    fi
    if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER_NAME" 2>/dev/null || true)" != "true" ]; then
      docker logs --tail 50 "$CONTAINER_NAME" >&2 || true
      fail "Gateway container stopped before becoming ready."
    fi
    now=$(date +%s)
    if [ $((now - started_at)) -ge "$WAIT_TIMEOUT" ]; then
      docker logs --tail 50 "$CONTAINER_NAME" >&2 || true
      fail "Gateway did not become ready within $WAIT_TIMEOUT seconds."
    fi
    sleep 2
  done
}

validate_docker_hub_image() {
  image_without_digest=${PUSH_IMAGE%%@*}
  first_component=${image_without_digest%%/*}

  [ "$first_component" != "$image_without_digest" ] || fail \
    "Docker Hub image must include a username or organization, for example: pczhao1210/caddy-reverse-proxy:latest"

  case "$first_component" in
    docker.io|index.docker.io)
      docker_hub_path=${image_without_digest#*/}
      case "$docker_hub_path" in
        */*) ;;
        *) fail "Docker Hub image must include a username or organization." ;;
      esac
      ;;
    *.*|*:*|localhost)
      fail "PUSH_IMAGE must target Docker Hub, not $first_component."
      ;;
  esac
}

resolve_push_image() {
  push_image_required=${1:-true}
  if [ -n "$PUSH_IMAGE" ]; then
    return
  fi
  case "$IMAGE" in
    */*) PUSH_IMAGE=$IMAGE ;;
    *)
      docker_hub_username=$(docker info 2>/dev/null | sed -n 's/^[[:space:]]*Username:[[:space:]]*//p' | sed -n '1p')
      if [ -z "$docker_hub_username" ]; then
        if [ "$push_image_required" = true ]; then
          fail "Docker Hub username was not reported by Docker. Set PUSH_IMAGE=pczhao1210/caddy-reverse-proxy:latest."
        fi
        return 1
      fi
      PUSH_IMAGE="$docker_hub_username/$IMAGE"
      ;;
  esac
}

remove_image() {
  image_to_remove=$1
  if docker image inspect "$image_to_remove" >/dev/null 2>&1; then
    docker image rm "$image_to_remove"
  fi
}

remove_data_dir() {
  [ -e "$DATA_DIR" ] || return
  if rm -rf -- "$DATA_DIR" 2>/dev/null; then
    return
  fi
  cleanup_image=$IMAGE
  if ! docker image inspect "$cleanup_image" >/dev/null 2>&1; then
    cleanup_image=alpine:3.22
  fi
  docker run --rm \
    --network none \
    --entrypoint /bin/sh \
    -v "$DATA_DIR:/data" \
    "$cleanup_image" \
    -c 'find /data -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +'
  rmdir "$DATA_DIR" 2>/dev/null || fail "Failed to remove data directory: $DATA_DIR"
}

command_name=${1:-help}
case "$command_name" in
  --*) command_name=${command_name#--} ;;
esac

[ "$#" -le 1 ] || fail "Only one command is accepted. Run ./start.sh help for usage."

case "$command_name" in
  build)
    require_docker
    printf 'Building %s...\n' "$IMAGE"
    docker build -f "$ROOT_DIR/backend/Dockerfile" -t "$IMAGE" "$ROOT_DIR"
    ;;
  push)
    require_docker
    resolve_push_image true
    validate_docker_hub_image
    docker image inspect "$IMAGE" >/dev/null 2>&1 || fail "Local image $IMAGE does not exist. Run ./start.sh build first."
    if [ "$PUSH_IMAGE" != "$IMAGE" ]; then
      docker tag "$IMAGE" "$PUSH_IMAGE"
    fi
    printf 'Pushing %s to Docker Hub...\n' "$PUSH_IMAGE"
    docker push "$PUSH_IMAGE"
    ;;
  start)
    require_docker
    [ -f "$ENV_FILE" ] || fail "Environment file not found: $ENV_FILE"
    ensure_image
    prepare_data_dir
    resolve_admin_token
    validate_networks
    stop_container
    printf 'Starting %s with data in %s...\n' "$CONTAINER_NAME" "$DATA_DIR"
    docker run -d \
      --name "$CONTAINER_NAME" \
      --restart unless-stopped \
      --env-file "$ENV_FILE" \
      -e GATEWAY_PROFILE=vm \
      -e GATEWAY_ADMIN_TOKEN="$ADMIN_TOKEN" \
      -e GATEWAY_DOCKER_ENABLED=true \
      -p "$HTTP_PORT:80" \
      -p "$HTTPS_PORT:443" \
      -p "127.0.0.1:$MANAGEMENT_PORT:8080" \
      --add-host host.docker.internal:host-gateway \
      -v "$ROOT_DIR/config:/config:ro" \
      -v "$DATA_DIR:/data" \
      -v /var/run/docker.sock:/var/run/docker.sock:ro \
      --security-opt no-new-privileges:true \
      --label io.caddy-reverse-proxy.managed=true \
      "$IMAGE" >/dev/null
    connect_networks
    wait_for_readiness
    printf 'Gateway is ready at http://127.0.0.1:%s\n' "$MANAGEMENT_PORT"
    printf 'Admin token: %s\n' "$ADMIN_TOKEN"
    printf 'The token is stored in %s/platform/admin-token when generated automatically.\n' "$DATA_DIR"
    ;;
  stop)
    require_docker
    stop_container
    printf 'Gateway stopped. Data remains in %s.\n' "$DATA_DIR"
    ;;
  restore)
    require_docker
    validate_data_dir
    printf 'Removing %s, %s, and the local image...\n' "$CONTAINER_NAME" "$DATA_DIR"
    stop_container
    remove_data_dir
    remove_image "$IMAGE"
    if resolve_push_image false && [ "$PUSH_IMAGE" != "$IMAGE" ]; then
      remove_image "$PUSH_IMAGE"
    fi
    printf '%s\n' 'Local gateway state has been removed.'
    ;;
  help)
    usage
    ;;
  *)
    usage >&2
    fail "Unknown command: $command_name"
    ;;
esac