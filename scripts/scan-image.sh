#!/bin/sh
set -eu

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

[ "$#" -eq 2 ] || fail "Usage: sh scripts/scan-image.sh local-image output-directory"
image=$1
output=$2
case "$image" in ''|-*) fail "Supply a local image name or ID" ;; esac
[ -n "$output" ] || fail "An output directory is required"
for tool in docker jq trivy sha256sum env sed; do
  command -v "$tool" >/dev/null 2>&1 || fail "Required scan tool is missing: $tool"
done
[ ! -e "$output" ] || fail "Output directory already exists: $output"
mkdir -p -- "$output"
output=$(CDPATH='' cd -- "$output" && pwd)
container=
cleanup() {
  status=$?
  trap - EXIT HUP INT TERM
  if [ -n "$container" ]; then
    docker rm -v "$container" >/dev/null || status=1
  fi
  rm -f "$output/caddy" "$output/platform" || status=1
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' HUP TERM

scan_with_policy() (
  # Keep transport settings, not inherited or future filtering options.
  for name in $(env | sed -n 's/^\(TRIVY_[A-Za-z0-9_]*\)=.*/\1/p'); do
    case "$name" in
      TRIVY_CACHE_DIR|TRIVY_DB_REPOSITORY|TRIVY_JAVA_DB_REPOSITORY|TRIVY_USERNAME|TRIVY_PASSWORD|TRIVY_REGISTRY_TOKEN|TRIVY_TIMEOUT|TRIVY_NO_PROGRESS) ;;
      *) unset "$name" ;;
    esac
  done
  scan_command=$1
  shift
  trivy "$scan_command" --config "" --ignorefile "" --ignore-unfixed=false \
    --severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL "$@"
)

id=$(docker image inspect "$image" --format '{{.Id}}')
printf '%s\n' "$id" | jq -Re 'test("^sha256:[a-f0-9]{64}$")' >/dev/null || fail "Invalid local image identity"
architecture=$(docker image inspect "$id" --format '{{.Architecture}}')
container=$(docker create --platform "linux/$architecture" --network none "$id")
docker cp "$container:/usr/share/gateway/security/." "$output/"
docker cp "$container:/usr/bin/caddy" "$output/caddy"
docker cp "$container:/usr/local/bin/platform" "$output/platform"
[ ! -e "$output/patched-dependencies.cdx.json" ] || fail "An embedded SBOM is not an executable; rebuild without embedding the dependency SBOM"

jq -e --arg architecture "$architecture" '
  .version == 1 and
  ([.sources[].Path] | sort == ["github.com/smallstep/certificates", "go.step.sm/crypto"]) and
  (.artifacts | length == 2) and
  ([.artifacts[].path] | sort == ["/usr/bin/caddy", "/usr/local/bin/platform"]) and
  all(.artifacts[]; .goos == "linux" and .goarch == $architecture and
    (.sha256 | test("^[a-f0-9]{64}$")) and
    (.cryptoVersion | test("^v[0-9]+[.][0-9]+[.][0-9]+$")) and
    .packageCount > 0 and .moduleCount > 0)
' "$output/evidence.json" >/dev/null || fail "Invalid or mismatched build evidence"
for binary in caddy platform; do
  expected=$(jq -er --arg binary "$binary" '.artifacts[] | select(.path | endswith("/" + $binary)) | .sha256' "$output/evidence.json")
  printf '%s  %s\n' "$expected" "$output/$binary" | sha256sum -c - || fail "Binary hash does not match evidence: $binary"
done
proof=$(sha256sum "$output/evidence.json")
proof=${proof%% *}
jq -e --arg proof "$proof" --slurpfile evidence "$output/evidence.json" '
  .["@context"] == "https://openvex.dev/ns/v0.2.0" and
  .["@id"] == ("urn:sha256:" + $proof) and
  (.statements | length == 1) and
  .statements[0].vulnerability.name == "GO-2026-5932" and
  .statements[0].status == "not_affected" and
  .statements[0].justification == "vulnerable_code_not_present" and
  ([.statements[0].products[]."@id"] | sort | unique) ==
    ([$evidence[0].artifacts[] | "pkg:golang/golang.org/x/crypto@" + .cryptoVersion] | sort | unique)
' "$output/openpgp.vex.json" >/dev/null || fail "VEX scope does not match the binary evidence"
jq '{
  bomFormat: "CycloneDX", specVersion: "1.6", version: 1,
  components: [.sources[] | {
    type: "library", name: .Path, version: .Version,
    purl: ("pkg:golang/" + .Path + "@" + .Version),
    properties: [{name: "gateway:upstream:go-checksum", value: .Sum}]
  }]
}' "$output/evidence.json" > "$output/patched-dependencies.cdx.json"

jq -n --arg image "$image" --arg id "$id" --arg architecture "$architecture" \
  '{image: $image, imageID: $id, architecture: $architecture}' > "$output/image.json"
scan_with_policy image --image-src docker --scanners vuln --format json --output "$output/raw.json" "$id"
jq -e '
  ([.Results[] | select(.Type == "gobinary") | .Target | ltrimstr("/")] | sort) ==
  ["usr/bin/caddy", "usr/local/bin/platform"]
' "$output/raw.json" >/dev/null || fail "Unexpected Go binaries; do not apply this VEX to unverified components"
scan_with_policy sbom --scanners vuln --list-all-pkgs --format json --exit-code 1 --output "$output/dependencies.json" "$output/patched-dependencies.cdx.json"
jq -e --slurpfile evidence "$output/evidence.json" '
  ([.Results[].Packages[]? | .Name + "@" + .Version] | sort | unique) ==
  ([$evidence[0].sources[] | .Path + "@" + .Version] | sort | unique)
' "$output/dependencies.json" >/dev/null || fail "Pinned upstream source versions were not fully analyzed"
scan_with_policy image --image-src docker --scanners vuln --format json --exit-code 1 --vex "$output/openpgp.vex.json" --output "$output/filtered.json" "$id"
rm "$output/caddy" "$output/platform"
printf 'Scanned immutable local image %s; raw and evidence-filtered reports: %s\n' "$id" "$output"
