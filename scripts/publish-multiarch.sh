#!/bin/sh
set -eu

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

[ "$#" -eq 1 ] || fail "Usage: sh scripts/publish-multiarch.sh repository:tag"
target=$1
case "$target" in
  ''|-*|*@*) fail "Supply a repository and tag, not a digest." ;;
esac
case "${target##*/}" in
  *:*) ;;
  *) target="$target:latest" ;;
esac
repository=${target%:*}
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
builder=${MULTIARCH_BUILDER:-gateway-multiarch}
for tool in docker jq curl; do
  command -v "$tool" >/dev/null 2>&1 || fail "Required publishing tool is missing: $tool"
done
docker info >/dev/null
docker buildx version >/dev/null
docker buildx inspect "$builder" --bootstrap || fail "Configure the $builder Buildx builder and ARM64 execution support; see docs/operations.md."

work=$(mktemp -d)
container=
promotion_started=false
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ -n "$container" ]; then
    docker rm -f -v "$container" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$work"
  if [ "$status" -ne 0 ] && [ "$promotion_started" = false ]; then
    printf 'Validation failed; this process has not updated %s.\n' "$target" >&2
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

valid_digest() {
  printf '%s\n' "$1" | jq -Re 'test("^sha256:[a-f0-9]{64}$")' >/dev/null
}

target_digest() {
  if docker buildx imagetools inspect "$target" --format '{{json .Manifest}}' >"$work/target.json" 2>"$work/inspect-error"; then
    digest=$(jq -er '.digest' "$work/target.json")
    valid_digest "$digest" || fail "Invalid registry digest for $target"
    printf '%s\n' "$digest"
  elif grep -Eq 'manifest unknown|MANIFEST_UNKNOWN|not found' "$work/inspect-error"; then
    printf '%s\n' absent
  else
    cat "$work/inspect-error" >&2
    fail "Cannot inspect $target. Check registry access; run docker login in your terminal when needed."
  fi
}

validate_index() {
  jq -e '
    (.manifests | type == "array") and
    ([.manifests[] | select(.annotations["vnd.docker.reference.type"] != "attestation-manifest") |
      .platform.os + "/" + .platform.architecture] | sort == ["linux/amd64", "linux/arm64"])
  ' "$1" >/dev/null || fail "Candidate must contain exactly linux/amd64 and linux/arm64 runtime manifests."
}

previous=$(target_digest)
suffix=$(od -An -N6 -tx1 /dev/urandom | tr -d ' \n')
candidate="$repository:multiarch-$(date -u +%Y%m%d%H%M%S)-$suffix"
printf 'Previous %s: %s\nCandidate: %s\n' "$target" "$previous" "$candidate"
docker buildx build --builder "$builder" --platform linux/amd64,linux/arm64 \
  --file "$root/backend/Dockerfile" --tag "$candidate" \
  --metadata-file "$work/build.json" --push "$root" || fail "Candidate build/push failed. Check the build output and registry login; target tag was not promoted."
index=$(jq -er '."containerimage.digest"' "$work/build.json")
valid_digest "$index" || fail "Build did not return a valid immutable index digest."
candidate_ref="$repository@$index"
docker buildx imagetools inspect "$candidate_ref" --format '{{json .Manifest}}' >"$work/candidate.json"
[ "$(jq -r '.digest' "$work/candidate.json")" = "$index" ] || fail "Candidate digest changed."
validate_index "$work/candidate.json"

for architecture in amd64 arm64; do
  platform="linux/$architecture"
  child=$(jq -er --arg architecture "$architecture" '.manifests[] | select(.platform.os == "linux" and .platform.architecture == $architecture) | .digest' "$work/candidate.json")
  valid_digest "$child" || fail "Invalid $platform child manifest."
  reference="$repository@$child"
  docker pull --platform "$platform" "$reference" >/dev/null
  docker image inspect "$reference" --format '{{json .}}' >"$work/image.json"
  jq -e --arg architecture "$architecture" '.Architecture == $architecture and .Os == "linux"' "$work/image.json" >/dev/null || fail "Image config does not match $platform."
  container=$(docker run -d --platform "$platform" --label io.caddy-reverse-proxy.test=multiarch \
    --security-opt no-new-privileges:true --tmpfs /data:rw,nosuid,nodev,size=64m \
    -p 127.0.0.1::8080 -e GATEWAY_DOCKER_ENABLED=false -e GATEWAY_AZURE_ENABLED=false \
    -e GATEWAY_AUTH_REQUIRED=true -e GATEWAY_ADMIN_TOKEN=multiarch-smoke-only "$reference")
  case "$architecture" in
    amd64) machine=62 ;;
    arm64) machine=183 ;;
  esac
  docker exec "$container" /bin/sh -ec '
    for binary in /usr/bin/caddy /usr/local/bin/platform; do
      machine=$(od -An -tu2 -j18 -N2 "$binary" | tr -d " ")
      test "$machine" = "$1"
    done
  ' check "$machine" || fail "Wrong ELF architecture in $platform image."
  docker exec "$container" caddy version | grep -q '^v2[.]11[.]4 '
  docker exec "$container" caddy list-modules | grep -qx 'dns.providers.azure'
  port=$(docker container inspect "$container" --format '{{json .NetworkSettings.Ports}}' | jq -er '."8080/tcp"[] | select(.HostIp == "127.0.0.1") | .HostPort')
  base="http://127.0.0.1:$port"
  if ! curl --noproxy '*' --silent --show-error --fail --retry 45 --retry-connrefused \
    --retry-all-errors --retry-delay 1 --retry-max-time 120 --max-time 2 \
    --output /dev/null "$base/readyz"; then
    docker logs --tail 30 "$container" >&2
    fail "$platform readiness check failed."
  fi
  curl --noproxy '*' --silent --show-error --fail --max-time 5 --output /dev/null "$base/livez"
  code=$(curl --noproxy '*' --silent --show-error --max-time 5 --output /dev/null --write-out '%{http_code}' "$base/api/status")
  [ "$code" = 401 ] || fail "$platform accepted an unauthenticated management request ($code)."
  curl --noproxy '*' --silent --show-error --fail --max-time 5 \
    --header 'Authorization: Bearer multiarch-smoke-only' --output "$work/status.json" "$base/api/status"
  jq -e 'type == "object"' "$work/status.json" >/dev/null
  docker rm -f -v "$container" >/dev/null
  container=
  printf 'Verified %s: %s\n' "$platform" "$child"
done

[ "$(target_digest)" = "$previous" ] || fail "$target changed during validation; refusing to replace another publisher's update."
promotion_started=true
docker buildx imagetools create --tag "$target" "$candidate_ref"
docker buildx imagetools inspect "$target" --format '{{json .Manifest}}' >"$work/published.json"
validate_index "$work/published.json"
[ "$(jq -r '.digest' "$work/published.json")" = "$index" ] || fail "Published digest differs from the verified candidate; inspect the registry before retrying."
printf 'Published %s\nIndex: %s\nPlatforms: linux/amd64, linux/arm64\nPrevious digest: %s\n' "$target" "$index" "$previous"