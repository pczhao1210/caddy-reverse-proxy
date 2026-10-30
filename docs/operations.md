# Operations Guide

[简体中文](operations.zh-CN.md)

This guide consolidates local startup, standalone and co-located VM deployment notes, Docker discovery labels, and runtime configuration.

## Environment File

`.env.example` is the sample file. A local `.env` is optional for `start.sh`; create one only when deployment-specific environment overrides are needed:

```sh
cp .env.example .env
```

`.env` is ignored by Git. Keep deployment-specific values there. When the token is empty or still `change-me`, `start.sh` generates a strong token under the persistent data directory and overrides the sample value. Paths such as `/config/platform.example.json` and `/data/platform/routes.json` are paths inside the container.

Boolean values accept `1`, `true`, `yes`, `y`, or `on` as true. Any other non-empty value is treated as false.

## Quick Local Run

```sh
./start.sh start
```

The command starts one container, publishes 80/443, binds the management UI to `127.0.0.1:8080`, and bind-mounts `~/docker_files/caddy-reverse-proxy` at `/data`. Use the token printed by the script. `stop` preserves that directory; `restore` removes it after enforcing the guarded `~/docker_files` path.

With the default bridge network, Docker discovery uses inspected container IP addresses for upstreams when available. That allows the gateway to proxy containers on the same bridge network without relying on Docker DNS names.

## Docker Network Reachability

Caddy can only proxy an upstream that the gateway container can reach at the network layer. If the gateway is attached only to a new custom Docker network, containers that remain only on Docker's default `bridge` network are normally not reachable by container DNS name, and direct container IP access can be blocked by Docker's bridge isolation rules.

Use one of these patterns instead:

- Keep the gateway on the same network as the workloads, such as the default bridge for local preview.
- Attach each workload that should be routed to the gateway's custom network as a second network.
- Attach the gateway to multiple Docker networks when it needs to route workloads from multiple isolated groups.
- Publish the workload on the host and route explicitly to a host-reachable address, for example `http://host.docker.internal:<port>` when configured or `http://172.17.0.1:<port>` on typical Linux Docker bridge setups.
- Use host networking intentionally when the gateway should proxy host-local services.

Recommended VM practice: run the gateway as a normal container, publish only the gateway ports on the host, and attach the gateway to the workload networks it must serve. Keep workload ports private to Docker networks. This preserves container isolation while still letting Caddy route across multiple application networks.

Do not treat `network_mode: host` as a way to attach the gateway to multiple Docker networks. Host networking puts the container in the host network namespace, and Docker does not combine that mode with normal per-network attachments. Use host mode only when the upstreams are host-local or when the deployment explicitly needs host namespace behavior.

### Mixed Network Example

Assume Portainer runs on the host network, the gateway runs on `proxy-net`, and the remaining apps are still on Docker's default `bridge` network. The gateway needs a reachable path to each upstream class:

- Portainer: if Portainer listens on host port `9443`, route to `https://host.docker.internal:9443`. On Linux, add `--add-host=host.docker.internal:host-gateway` to the gateway container; without that name, use a host-reachable bridge gateway address such as `https://172.17.0.1:9443`.
- Services on `proxy-net`: attach the service and gateway to `proxy-net`, then route to `http://service-name:port`.
- Services only on the default `bridge`: preferably attach that service to `proxy-net` as a second network and route to `http://service-name:port`. If the service network cannot be changed, use the inspected bridge IP such as `http://172.17.0.5:8080`, or publish the service on a host port and route through the host address.

Example commands:

```sh
docker network create proxy-net

docker run -d --name gateway \
	--network proxy-net \
	--add-host=host.docker.internal:host-gateway \
	-p 80:80 -p 443:443 -p 127.0.0.1:8080:8080 \
	-v "$HOME/docker_files/caddy-reverse-proxy:/data" \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	--env-file .env \
	pczhao1210/caddy-reverse-proxy:latest

docker network connect proxy-net app-on-bridge
```

Explicit route example:

```json
{
	"routes": [
		{
			"host": "portainer.example.com",
			"exposure": "protected",
			"enabled": true,
			"https": true,
			"source": "explicit",
			"upstreams": [{ "name": "portainer", "url": "https://host.docker.internal:9443" }]
		},
		{
			"host": "app.example.com",
			"exposure": "public",
			"enabled": true,
			"https": true,
			"source": "explicit",
			"upstreams": [{ "name": "app", "url": "http://app-on-bridge:8080" }]
		}
	]
}
```

In this topology, the important question is not whether the gateway itself uses host networking. It is whether each upstream URL is reachable from inside the gateway container. Prefer adding proxied containers to `proxy-net`; expose host-local services to the gateway through `host.docker.internal` or the host's bridge gateway address.

## Host Network Mode

Host networking can proxy upstreams, but it changes the tradeoffs:

- The gateway can bind host ports 80/443 directly without `-p` mappings.
- It can reach host-local services through `127.0.0.1:<port>`.
- Container discovery still works only if the Docker socket or socket proxy is mounted.
- Discovered container IPs can usually be proxied, but host networking removes container network isolation for the gateway.
- It is Linux-only for normal Docker Engine usage and is not recommended as the default preview path.

Use explicit routes for host-local upstreams, for example `http://127.0.0.1:3000`.

## Make Targets

| Target | Purpose |
|---|---|
| `make test` | Run Go tests in a Go toolchain container. |
| `make docker-build` | Build `IMAGE`, default `pczhao1210/caddy-reverse-proxy:latest`. |
| `make docker-build-companions` | Build the controlled socket-proxy and sample httpbin images locally; does not publish them. |
| `make docker-push` | Build, verify, and publish AMD64 + ARM64 for `IMAGE`. |
| `make docker-run` | Run the image locally with `ENV_FILE`, default `.env`, on Docker bridge. |
| `make compose-up` | Start the VM sample stack. |
| `make compose-up-proxy` | Start the VM stack with Docker discovery through a socket proxy. |
| `make compose-prod-up` | Start the production VM stack and wait for readiness. |
| `make compose-prod-down` | Stop the production VM stack while preserving its data volume. |
| `make azure-vm-deploy` | Interactively create and start a standalone Azure VM gateway. |
| `make test-e2e` | Exercise Caddy routing with the sample VM stack. |
| `make compose-down` | Stop the VM sample stack. |

The default build and push target is the published Docker Hub repository:

```sh
make docker-build
make docker-push
```

Override `IMAGE` when publishing another repository or immutable tag.

## Multi-Architecture Publishing

`./start.sh push` and `make docker-push` share `scripts/publish-multiarch.sh`. They build both `linux/amd64` and `linux/arm64` from source, regardless of locally tagged images. `start.sh` always publishes `latest`; Make preserves an explicit `IMAGE=repository:tag`. `build` remains a local, single-platform command and is not a prerequisite for publishing. Do not subsequently run plain `docker push ...:latest` on a single-platform local image: that would replace the multi-platform index.

The publishing machine needs Docker, Buildx, `jq`, `sha256sum`, Trivy (validated with 0.76.1), and `curl` with `--retry-all-errors` support. Authenticate with `docker login` directly in your terminal. Both the builder and local Docker daemon must be able to execute the two target architectures for runtime checks. The following one-time setup is for a Linux AMD64 development host. The binfmt command is privileged and changes host-wide execution support; run it only with the host owner's approval, not on the gateway VM:

```sh
docker run --privileged --rm tonistiigi/binfmt@sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0 --install arm64
docker buildx create --name gateway-multiarch --driver docker-container \
	--driver-opt image=moby/buildkit@sha256:6c2fa84a6b61ccd72899dde4239f8d5717f05f9a8ca6f3cad185fb1a95a94de3
docker buildx inspect gateway-multiarch --bootstrap
./start.sh push
```

Reuse an existing suitable builder via `MULTIARCH_BUILDER`; the script does not select a new default builder or install emulators automatically. An ARM publishing host needs equivalent AMD64 execution support. Go and xcaddy compile on `BUILDPLATFORM` with explicit target OS/architecture; the target Alpine stage still executes Caddy version/module checks. The Ubuntu host does not require an Ubuntu container base. The prepared builder and cache are retained for subsequent releases.

BuildKit's bundled emulation can build an ARM64 image even when the host Docker daemon cannot run it. An exited foreign-architecture container with `exec format error` is therefore not evidence of a wrong ELF header. The publisher now reports container state and startup logs separately from an actual ELF-machine mismatch. Scan-only containers explicitly select the inspected image platform. The approved binfmt command registers the current host kernel; after a host reboot, execution support may need registration again. Do not silently run privileged setup or recreate the builder to work around a startup failure.

The 2026-10-30 execution follow-up registered ARM64 binfmt only with explicit host-owner approval. Both actual immutable candidates passed startup, ELF, Caddy/Azure module, readiness/liveness and authenticated/unauthenticated API checks without promoting a registry tag. The complete backend ordinary Go suite ran as ARM64 executables under QEMU against the candidate's real ARM64 Caddy, with read-only test/source mounts and no external network, cloud integration or Docker socket. This covers routing, TLS, HTTP/3, internal CA and certificate tests; AMD64 passed native race/vet. Both candidates also passed raw/SBOM/scoped scans, and 44 scan/publish regressions passed, including startup exits and exit-before-ELF races. ARM64 race instrumentation, native ARM hardware/performance, and real cloud/ACME validation remain unrun. Earlier entries that say ARM64 execution was unavailable describe the pre-registration baseline.

Publication first pushes a unique `multiarch-*` candidate tag. It validates the immutable index and per-platform image configuration, runs the evidence-bound vulnerability gate below for both architectures, verifies ELF architectures, Caddy version and Azure DNS module, then runs isolated containers for readiness, liveness, unauthenticated 401 and authenticated API checks. Missing/tampered evidence or remaining vulnerabilities block promotion. State is temporary, only a random loopback management port is published, and neither real Docker sockets nor cloud integration are enabled. Failed checks do not promote the target tag; temporary containers are removed, and candidate tags remain available for investigation/manual retention management.

Immediately before promotion, the script checks that the target digest has not changed since the build began. This is an optimistic check, not registry compare-and-swap: serialize publishers to the same tag. Promotion uses the verified index digest and verifies it again afterwards. A post-promotion error requires registry inspection rather than blind retry. Keep the printed previous digest for rollback. With appropriate authorization, restore that reference using `docker buildx imagetools create --prefer-index=false --tag <repository:tag> <repository>@<previous-digest>`; restoring a legacy single-architecture manifest also removes ARM64 availability.

Verify the published platform list with `docker buildx imagetools inspect pczhao1210/caddy-reverse-proxy:latest`. Consumers use the same ordinary `docker pull pczhao1210/caddy-reverse-proxy:latest` on AMD64 or ARM64. Buildx provenance entries may appear as `unknown/unknown`; they are attestations, not additional runnable architectures. Publishing does not restart deployed containers.

The 2026-09-22 release index is `sha256:21edcc864fa5405df5ceecab2e0368220fa9a8ad2de51d5833020b9941eec852`. Compressed runtime layers total approximately 29.1 MiB for AMD64 and 27.3 MiB for ARM64. The previous AMD64-only manifest was `sha256:d493c1622f67c5b905ef9abc212bb7b6ac5b0de4a146925f78ad92400555a255`.

Verification: `node --test scripts/publish-multiarch.test.cjs` passes 15 isolated publishing tests, including failure paths and tag handling. Native full Go race/vet and real Caddy tests passed. Cross-compiled Caddy/certificate test packages passed in network-isolated ARM64 containers under QEMU, including HTTP/TLS/Host/auth and storage-lock checks. Both published architectures passed the shipping-image smoke tests. Native ARM hardware performance, real cloud/ACME and production workloads were not tested. Security exceptions below remain open; the publisher does not automatically run a vulnerability scanner.

## Dependency Upgrade And Configuration Fixes (2026-10-30)

This is the current **source/local-build** baseline, not a published release. Neither `latest` nor a running deployment was changed. The September results below are historical, not verification of these changes.

| Component | Selected version |
|---|---|
| Both Go build toolchains | 1.27.3 |
| Caddy / xcaddy / Azure DNS plugin | 2.11.7 / 0.4.7 / 0.6.0 |
| CertMagic, control plane and Caddy | 0.25.6 |
| Azure azcore / azidentity | 1.23.3 / 1.15.0 |
| Azure DNS / Network SDK | 1.2.0 / 12.1.0 (`2026-03-01` Network API) |
| `x/crypto` / `x/net` / `x/text` | 0.58.0 / 0.62.0 / 0.43.0 |
| Caddy CEL / Chi / go-jose v4 | `cel.dev/cel-go` 0.33.1 / 5.5.0 / 4.1.6 |
| Caddy OpenTelemetry / gRPC / compress / smallstep crypto | 1.47.0 / 1.85.0 / 1.20.3 / 0.91.1 with local v4 migration |
| Smallstep certificates | 0.30.2 with the local migration test/module adaptation |
| Caddy QUIC / Prometheus client | 0.64.1 / 1.25.0 |
| Gateway runtime Alpine Linux / UI Alpine.js | 3.24.3 / 3.17.5 |
| Rebuilt socket proxy / sample httpbin | 0.5.0 on HAProxy 3.4.7 / 2.26.0 built with Go 1.27.3 |

Both binaries use the same digest-pinned Go/Alpine base. Caddy 2.11.7 uses the renamed CEL module, so the old incompatible `github.com/google/cel-go` replacement was removed. OpenTelemetry resolves as one compatible family: core and Logs API/SDK 1.47.0, log exporters 0.23.0, and contrib 0.72.0. Parent-module pins bring in their matching exporters and SDKs without replacing every child separately. Latest-version checks also cover newly introduced transitive modules, not just the original graph. Commit-based modules are pinned after review: SCEP's update is documentation-only, fallback roots refresh the CA bundle, and the RPC protobuf snapshot aligns with the API snapshot. The two Go dependency graphs remain independent and must both be scanned. HTTPS upstream Host behavior and CertMagic storage-lock compatibility remain regression gates.

The control plane now imports Azure Network SDK `/v12`; its SecurityRules requests use API `2026-03-01`. Mock-transport regressions preserve instance-scoped names, ownership checks, no-op behavior, create-after-404, delete-after-404, read-error handling and the serialized rule body. DNS remains on its current stable SDK with ETag-conditional updates/deletes. Real Azure compatibility, including sovereign/cloud-specific API availability, is not established by these mock tests. The matching quic-go module in the control-plane manifest is used for real HTTP/3 regression tests, not linked into the shipped control-plane executable. Graph-only dependencies are not forcibly added to the executable merely to change their reported versions.

Before the local JOSE migration, the AMD64 gateway's Trivy 0.76.1 scan decreased from 62 to 5 package records: three go-jose v3 advisories and two OpenPGP module records. The migration below removes the linked v3 module rather than hiding or renaming it. Raw package matching and evidence-filtered results are kept separately; zero applicable findings are not a guarantee of permanent security.

`govulncheck` 1.9.0, rebuilt with Go 1.27.3, found no reachable vulnerabilities from `./cmd/server` and one module-only OpenPGP warning. Build the scanner with `GOTOOLCHAIN=go1.27.3 go install golang.org/x/vuln/cmd/govulncheck@v1.9.0`; its default installation can otherwise select an older compiler that cannot analyze Go 1.27 source. The earlier Caddy wildcard “symbol” result was not proof that OpenPGP was linked: stripped-binary analysis can fall back to module-level precision. Exact source import graphs now establish applicability during each build. Both controlled companion images scanned with zero findings in the earlier database snapshot, not a permanent security guarantee.

### Local JOSE Migration And Evidence-Bound Scanning

The [source patches](../backend/patches/) start from Smallstep Crypto 0.91.1 and Certificates 0.30.2, verified against fixed Go module checksums before zero-fuzz patch application. Provenance and Apache-2.0 licensing are in [NOTICE](../backend/patches/NOTICE). The reviewed basis is [smallstep/crypto#1157](https://github.com/smallstep/crypto/pull/1157), head `00c45432f48558f92a6e7da6d885d94e90db4d7a`, but the local build uses **go-jose v4.1.6**, not that PR's vulnerable v4.1.4.

The compatibility layer retains all-expected-audience validation, claim error precedence, compact/JSON JWT builder interfaces, and XEdDSA signing/key IDs through the supported non-embedding opaque signer API. It does not relax go-jose's key checks. Previously accepted mismatched Ed25519 public/private pairs and undersized HMAC keys are now rejected; replace invalid keys rather than disabling validation. JSON JWT parsing requires one protected signature and rejects unprotected headers or multiple signatures. Encryption tests use round trips because v4 removed `RandReader`. PKI, the internal CA, the local ACME server module and Azure DNS remain available; no Caddy capability was removed to reduce scanner counts.

Each build's native [security-evidence tool](../backend/cmd/security-evidence/) compares the actual target binaries with their exact source dependency graphs, rejects imported OpenPGP packages, checks JOSE v4.1.6/no linked v3, and writes binary hashes, target architecture, package/module counts, upstream checksums and patch hashes under `/usr/share/gateway/security`, alongside an OpenPGP-only OpenVEX statement. The patched upstream base-version SBOM is reconstructed from that evidence on the scanning machine, not embedded in the runtime image: Trivy otherwise counts its embedded SBOM as another language target. The separate SBOM scan prevents local replacements' unversioned build metadata from creating a new advisory-monitoring blind spot.

Use the [scan wrapper](../scripts/scan-image.sh) with a new output directory:

```sh
sh scripts/scan-image.sh gateway-jose-v4-check:20261030 /tmp/gateway-security-report
```

It locks the local image ID, extracts but never starts it, checks both binary hashes and evidence/VEX/SBOM consistency, and refuses extra unverified Go binaries. It saves `raw.json`, `dependencies.json`, `filtered.json` and the evidence; patched-source advisories or any remaining filtered findings return failure. Do not reuse the VEX as a global ignore file or apply it directly to another image. These are trusted-build applicability records, not signed attestations authenticating arbitrary third-party images. Source, plugins, architecture or binaries changing requires regeneration. `publish-multiarch.sh` runs this gate for both pulled candidates before target promotion; it now requires Trivy and does not silently skip scanning.

All three Trivy invocations use a controlled policy: configuration/ignore-file loading is disabled, unfixed findings are included, and every severity (including UNKNOWN) is retained. The wrapper removes inherited `TRIVY_*` settings except `TRIVY_CACHE_DIR`, `TRIVY_DB_REPOSITORY`, `TRIVY_JAVA_DB_REPOSITORY`, registry username/password/token, timeout and progress settings. Proxy, Docker and custom-CA environment settings remain available. Thus global filtering variables, external VEX settings, and current-directory `trivy.yaml`/`.trivyignore` files cannot weaken the gate; only the hash-verified VEX is supplied to the filtered scan. The policy-isolation follow-up passed 41 scan/publish regressions and real Trivy same-image control/hostile-environment checks: raw findings stayed at two OpenPGP records and applicable findings stayed at zero. No image rebuild or ARM64 runtime verification was performed for this shell-only follow-up.

These are local dependency patches, not an upstream release or publication of `latest`. Remove them only after upstream releases replace the linked v3 code with a fixed v4 version and downstream compatibility is verified. Re-run dependency unit tests, real Caddy HTTP/TLS/HTTP3/Host/auth/internal-CA/storage-lock regressions, target-specific evidence checks and both raw/applicability scans. Keep auditing the pinned upstream base modules while local replacements remain.

JOSE-migration verification passed: patched Crypto JOSE/Azure KMS and Certificates ACME/ACME API/StepCAS/authority/provisioner race suites; uncached full control-plane Go race/vet with the actual shipping Caddy, including HTTP/3, authentication, metrics, storage locks and isolated internal-CA issuance without installing host trust; 37 UI/publishing/scanning Node regressions; all three Compose configurations; and final-image desktop/390px browser login, security save, token rotation, injected HTTP 503 and nested certificate forms. The final AMD64 raw scan contains only two `GO-2026-5932` module records, no JOSE v3 or OS findings; the scoped scan contains zero applicable findings, and both patched upstream base versions are recognized and have zero findings in the supplementary SBOM scan. Source reachability scanning finds no called/imported control-plane vulnerability. Target-specific Caddy/control-plane cross-builds and no-OpenPGP evidence generation pass for ARM64; ARM64 execution/full-image validation remains unrun because this host lacks execution support. Prior companion/httpbin checks are historical, not rerun by this JOSE-only change. No privileged emulation changes, real cloud/ACME operations, production certificate actions or destructive Compose E2E were performed.

### Controlled Companion Builds

The upstream socket-proxy release has not been rebuilt with the required fixes, and upstream httpbin 2.26.0 still contains an older Go toolchain. The repository therefore provides [Dockerfile.socket-proxy](../deploy/vm/Dockerfile.socket-proxy) and [Dockerfile.httpbin](../deploy/vm/Dockerfile.httpbin). The proxy copies the entrypoint and ACL template from digest-pinned upstream v0.5.0 onto digest-pinned HAProxy 3.4.7/Alpine 3.24.3. It preserves all upstream application permission defaults, command, root user and stop signal; it does not copy the old vulnerable runtime libraries. Httpbin is rebuilt from versioned Go source with Go 1.27.3 on Alpine 3.24.3 and still runs as UID/GID 65532.

Build on a development machine for the deployment architecture:

```sh
make docker-build-companions
docker save -o gateway-companions-20261030.tar \
  caddy-reverse-proxy-socket-proxy:0.5.0-haproxy3.4.7 \
  caddy-reverse-proxy-httpbin:2.26.0-go1.27.3
```

Transfer the archive through an approved channel, then `docker load -i gateway-companions-20261030.tar` on the target host. The production Compose profile deliberately has **no build step** and uses `pull_policy: never` for the proxy: preload the approved image before starting it. An approved registry image may be selected with `SOCKET_PROXY_IMAGE`, but it must also be pulled beforehand. Sample profiles have local build definitions; `SOCKET_PROXY_IMAGE` and `HTTPBIN_IMAGE` override their tags. The normal multi-architecture publisher still publishes only the gateway, not these companion images.

### Configuration Update Safety

Security policy, token rotation and certificate policy updates are serialized with Caddy configuration commits. They use applied routes, confirm Caddy loading, then persist and publish the new management state. A failed reload (including a lost acknowledgment or canceled request) or failed persistence triggers restoration of the previous runtime configuration and returns HTTP 503 without changing browser credentials. Rollback failure is explicitly reported and requires operator attention; this is compensating rollback, not crash-atomic storage across Caddy and files. A later health/cloud failure does not undo an already committed token rotation.

Empty and `/` routing-rule prefixes now normalize to the same empty root prefix. Duplicate roots are rejected by CRUD, imports and v1/v2/v3 file loading; valid single-root legacy configurations still work. Ambiguous legacy files are preserved and rejected, not silently deduplicated. Before upgrading such an installation, use the old Console to remove the redundant rule and Apply, then back up state.

## Dependency Upgrade Baseline (2026-09-22)

| Component | Selected version |
|---|---|
| Go build/test toolchain | 1.26.8 |
| Caddy / xcaddy / Azure DNS plugin | 2.11.4 / 0.4.7 / 0.6.0 |
| CertMagic, control plane and Caddy | 0.25.4 |
| Azure azcore / azidentity | 1.23.1 / 1.14.1 |
| `x/crypto` / `x/net` / `x/text` | 0.57.0 / 0.59.0 / 0.42.0 |
| Gateway runtime Alpine Linux / UI Alpine.js | 3.22.6 / 3.17.4 |
| Docker socket proxy / sample go-httpbin | v0.5.0 / 2.25.0 |

Build-base and companion images are pinned by tag and registry digest. The control plane's `go.mod` and xcaddy's generated module graph are independent: update and scan both. The Dockerfile also pins security fixes for Chi, compress, OpenTelemetry, and gRPC. These pins do not lock every transitive build dependency. The UI vendor notice records the npm source and file hash.

Caddy 2.11 changes HTTPS upstream Host defaults. The renderer explicitly preserves the incoming Host for compatibility; an explicitly configured Host still takes precedence. TLS verification remains enabled. Control-plane and Caddy CertMagic versions are aligned because certificate archive operations use its native storage locks. When upgrading again, update the runtime-version assertion and rerun the storage-lock and real-Caddy tests.

Local verification passed: full Go race tests and vet, real Caddy HTTP/TLS/auth/Host/active-config regressions, Node tests (6/6), browser fixtures for Alpine 3.17.4 login, nested forms, certificate filtering/archive confirmation, 401/503 handling, and desktop/390px layouts. All Compose files rendered successfully. An isolated socket proxy with a mock Unix socket allowed GET containers/info/networks and denied POST create and GET secrets; sample httpbin health and GET checks passed. No existing gateway or real Docker socket was used.

Security results are a dated snapshot, **not a clean production release gate**:

- `govulncheck` 1.8.0 found no reachable or imported-package vulnerabilities in `./cmd/server`; its module-only OpenPGP warning remains. Trivy 0.74.0 found no OS-package vulnerabilities in the final gateway image (Alpine 3.22.6, 18 packages).
- The final Caddy binary still reports [GO-2026-6094](https://pkg.go.dev/vuln/GO-2026-6094) in CEL 0.28.1 (`NativeTypes`/`ParseStructTag`). The fixed CEL 0.30.0 fails to compile against Caddy 2.11.4's interpreter API. It also reports [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932) in the unmaintained OpenPGP package, for which no patched version is listed. Binary findings do not establish exposure through this gateway's configuration; compatibility fixes and reachability review remain required.
- The multi-architecture release's full Trivy Go-package scan also reports `CVE-2026-81871` (medium, OTLP log gRPC exporter 0.19.0; fixed 0.21.0) and `CVE-2026-81870` (low, trace exporters/SDK; fixed 1.45.0). AMD64 binaries are byte-identical to the preceding release, so these are newly recorded findings, not dependencies introduced by the architecture change. Both architectures have identical findings and no OS-package findings. Trivy lists CEL 0.29.0 as fixed while the Go advisory lists 0.30.0; reconcile this difference before selecting a compatible fix. Version findings alone do not prove reachable exploitation.
- The pinned socket proxy image contains OpenSSL `libcrypto3`/`libssl3` 3.5.7-r0: 10 distinct CVEs, including one high-severity CVE, across 20 package findings. Alpine lists 3.5.8-r0 as fixed. Await an updated upstream image or explicitly approve a maintained rebuilt image; do not substitute a mutable nightly tag.
- The sample httpbin image has no reported OS findings, but its Go 1.26.5 binary has eight high-severity standard-library CVE findings. These are version-based findings, not a call-path analysis. The relevant fixes start at Go 1.26.6; an upstream rebuild is needed. This sample is not part of the production Compose file and should not be publicly exposed.

Next smallest step: resolve or explicitly review these upstream exceptions, then rescan the exact images before an authorized staging rollout. No deployment, real Azure integration, public ACME renewal, production certificate archive, or destructive sample-stack E2E was performed. Back up gateway state and retain the previous image before staging; verify HTTPS Host behavior, certificate inventory and renewal there before production.

## Core Runtime Variables

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_PROFILE` | `vm` | Deployment profile. `vm` is the only supported value. |
| `GATEWAY_DEPLOYMENT_MODE` | `container-socket` | UI/runtime deployment identity: `container-socket` or `azure-vm`. The launchers set this explicitly. |
| `GATEWAY_ADMIN_TOKEN` | `change-me` | Admin token for the management API and protected routes. Replace this before real deployment. |
| `GATEWAY_ADMIN_TOKENS` | empty | Optional comma-separated token allowlist for multiple operators. |
| `GATEWAY_AUTH_REQUIRED` | `true` | Enables token authentication for `/api/*`. |
| `GATEWAY_RECONCILE_SECONDS` | `30` | Periodic reconcile interval in seconds. Background runs use the applied snapshot even while drafts are pending; only manual Apply activates drafts. |
| `GATEWAY_CONFIG_FILE` | `/config/platform.example.json` | JSON platform config inside the container. Environment variables override it. |
| `GATEWAY_ROUTES_FILE` | `/data/platform/routes.json` | Writable v3 envelope containing desired/applied v2 resources and their revisions. Legacy routes and Docker binds use the compatibility adapter. |
| `GATEWAY_STATE_DIR` | `/data/platform` | Platform state directory. |
| `GATEWAY_CADDY_DATA_DIR` | `/data/caddy` | Caddy certificate/runtime data. Persist this in production. |
| `GATEWAY_CERTIFICATE_FILE` | `/data/platform/certificate.json` | Console-managed certificate settings. Stored atomically and created with mode `0600` on POSIX filesystems. |
| `GATEWAY_INTERNAL_SOURCE_RANGES` | RFC1918, loopback, IPv6 private/link-local | Comma-separated IP/CIDR ranges permitted to use `internal` routes. |

## Listeners And Management Access

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CONTROL_LISTEN` | `:8080` | Management API/UI listener inside the container. |
| `GATEWAY_MANAGEMENT_HOST` | empty | Optional public hostname for the management UI through Caddy on 80/443. Static login assets are public; `/api/*` requires API authentication. Participates in Azure DNS/NSG reconciliation. |
| `GATEWAY_HTTP_LISTEN` | `:80` | Caddy HTTP listener inside the container. |
| `GATEWAY_HTTPS_LISTEN` | `:443` | Caddy HTTPS listener inside the container. |
| `GATEWAY_CADDY_ADMIN_ENDPOINT` | `http://127.0.0.1:2019` | Local Caddy Admin API endpoint. Keep it loopback-only. |

Default recommendation: leave `GATEWAY_MANAGEMENT_HOST` empty and access the UI through SSH tunnel, VPN, Bastion, Tailscale, or WireGuard.

A management hostname requires `auth.required=true` and at least one admin token; rendering fails otherwise. Caddy preserves credentials only on its internally generated loopback management proxy. Ordinary protected routes still strip gateway credentials before forwarding to business upstreams. A generic HTTP 503 displays an error without clearing the Console token; 401 requires login again.

HTTP and HTTPS listeners use separate Caddy servers, including custom ports. Conflicting protocols on overlapping endpoints are rejected. HTTPS routes redirect from the default HTTP listener to their actual TLS port, after explicit HTTP routes have been evaluated.

## Routing UI Semantics

The Console composes a route from a Listener, Backend Pool, and Routing Rule:

- A backend pool normally contains one address per line without a scheme or port. Private IPs such as `10.0.0.5`, reachable public IPs, and DNS names such as `ex.example.com` are accepted. DNS names are resolved from the gateway runtime, and every address must be network-reachable from that runtime. For HTTPS backends, the certificate must validate for the address or hostname used.
- The routing rule's backend protocol and port are shared by all pool targets. The API and migration layer retain an optional per-target port override for legacy routes, but new Console configurations should use the shared rule port.
- A backend address controls where Caddy connects; it does not change the incoming HTTP `Host` by default. Leave **Backend Host header** empty for applications that route on the frontend hostname. Set it to `ex.example.com` when proxying to an external virtual host that expects that hostname.
- An empty route path prefix or `/` matches every path on the listener; both are stored as an empty prefix and cannot coexist as separate root rules on one listener. `/api` matches `/api` and `/api/...`; Caddy preserves that prefix when forwarding upstream.
- The health path is optional. Empty uses `health.defaultPath`, which defaults to `/`. HTTP 2xx and 3xx responses are healthy. If `/` returns 404, enter a real readiness path such as `/healthz`. Probe failure marks status only; it does not stop proxying or withdraw DNS.
- `public` permits any client that can reach the listener, subject to the global security baseline. `protected` additionally requires an enabled gateway token header. `internal` accepts only direct client IPs in `gateway.internalSourceRanges` and does not participate in managed public DNS/NSG reconciliation.
- Caddy's HTTP reverse proxy automatically handles WebSocket Upgrade requests over HTTP or HTTPS. No routing-rule WebSocket switch is required.

Listener, backend-pool, routing-rule, legacy-route, and Docker-bind mutations are persisted as drafts. They do not activate those changes until **Apply pending changes** is selected. Multiple edits can be prepared as one batch. `/api/status` exposes `routingChangesPending` by comparing desired and applied revisions. Manual `POST /api/reconcile` confirms only the snapshot that Caddy accepted; a newer draft remains pending. Startup, periodic reconciliation, certificate, security-policy, and token reloads use the applied snapshot. Discovery and health checks for that snapshot continue while a draft is pending.

The v3 route file stores desired and applied snapshots together using atomic file replacement. Ordinary drafts survive restart without becoming active. Back up state before upgrading: legacy v1/v2 files migrate with their current contents as the applied baseline, because those formats cannot identify previously unapplied drafts. The older binary cannot read v3; restore a compatible backup before downgrading. Configuration ZIP resources remain v2.

Health probes use at most 8 concurrent requests and a 10-second budget per round, in addition to the configured per-request timeout. Results retain route order; any failed upstream marks its route unhealthy. Configuration loading does not wait for an older round's probes or cloud operations. New rounds cancel superseded checks, and only the current round publishes status; Azure operations are separately serialized.

## Runtime Logs

The **Logs** page combines recent gateway/Caddy runtime entries from `GET /api/logs` with persisted configuration audit events from `GET /api/audit`. Entries can be filtered by level or source and searched by message or structured fields. Runtime entries are held in a bounded in-memory ring: at most 1,000 entries, 8 MiB of serialized entry data in total, and 64 KiB per input line. The oldest entries are evicted when either limit is reached, and all runtime entries are cleared whenever the gateway process restarts. They are intended for recent diagnosis, not durable log retention. Audit reads scan backwards in 64 KiB blocks until enough valid records are found, preserving chronological order and skipping malformed lines; a line larger than 1 MiB returns an error. Audit history is not automatically rotated or deleted and should be managed as part of host disk retention. The API returns structured runtime messages only and does not provide arbitrary file access.

The source filter defaults to **Routing activity**. This view includes Caddy HTTP access events alongside persisted audit events for routes, listeners, backend pools, routing rules, configuration imports, and reconciliation, plus routing-related reconcile warnings or errors. An access-event summary shows client IP, method and destination, matched listener, routing rule, selected backend pool and actual upstream, response status, and duration. Expand **Details** for the complete structured request fields; Caddy redacts credential headers by default. A successful runtime `reconcile complete` entry is omitted when the persisted `audit: reconcile.complete` record represents the same operation, avoiding the common duplicate. Select **All sources** or an exact source to inspect other Caddy and control-plane logs. Filtering changes only the Console view; it does not delete records. Route and reconciliation audit events survive restarts when audit persistence is enabled, while HTTP access events and other `gateway/*` or `caddy/*` runtime entries do not.

## Console-Managed Settings

The **Security** page edits the global request baseline, internal route source ranges, and protected-route token header policy. It persists and reloads Caddy immediately. The **Settings** page edits the desired deployment mode, Azure integration values, and admin login token.

Console-managed settings are atomically stored in `/data/platform/settings.json` with mode `0600` on POSIX filesystems. They are loaded after the JSON config and environment variables for the fields the Console owns. Protect this file because it contains the admin token in the form required for authentication. Delete it while the gateway is stopped to return those fields to file/environment control.

Security policy changes are applied immediately. A new admin token takes effect immediately and invalidates the old token. Deployment-mode and Azure changes are saved for the next process start because Docker discovery, credentials, and Azure SDK clients are constructed at startup. Changing deployment mode in the Console does not change Docker network mode, mounts, host port publication, or VM infrastructure; the launcher must also satisfy the selected topology.

### Configuration archive transfer

**Settings > Configuration files** exports a dated `caddyproxy_config_yyyymmdd.zip`. The archive has exactly four JSON files:

| File | Contents |
|---|---|
| `manifest.json` | Format version, export time, fixed file list, and explicit no-secrets/no-certificate-material flags. |
| `routes.json` | Listeners, backend pools, and routing rules. |
| `settings.json` | Console-managed deployment, Azure, security, and related settings with authentication secrets removed. |
| `certificate-policy.json` | Issuer, subjects, renewal, and DNS challenge policy with the Azure client secret removed. |

The archive never contains issued certificates, private keys, Caddy data, admin tokens, additional authentication-header values, Azure client secrets, the Azure instance identity, runtime logs, or audit logs. Import preserves secret values and resource ownership from the target instance.

Import accepts only the fixed file set and rejects duplicate, nested, unknown, symlink, oversized, malformed, or unknown-field entries. It validates routes and settings and renders a complete candidate Caddy configuration before staging anything. A successful import only replaces the editable in-memory draft: it does not write route, settings, or certificate files and does not reload Caddy. The status API reports `configurationImportPending`, and conflicting settings, security, and certificate mutations are blocked while that draft is pending.

Review the imported routes, settings, and certificate policy, then select **Apply pending changes**. After Caddy accepts the candidate, the gateway atomically replaces each imported file and confirms the applied revision. Returned persistence errors retain the draft for retry and trigger restoration of the previous files and Caddy configuration; a failed restoration is reported explicitly. This is not a crash-atomic transaction across multiple files: keep a backup before import and restore a consistent state set if the process crashes partway through persistence. Certificate issuance starts asynchronously after reload; Apply does not wait for it. Deployment mode and Azure settings still require process restart because their clients and topology are initialized at startup.

The authenticated API equivalents are `GET /api/settings/configuration` for export and `POST /api/settings/configuration` with an `application/zip` body for import. Uploads are limited to 8 MiB compressed and 4 MiB total uncompressed JSON, with a 1 MiB limit per entry.

## Request Security Baseline

The gateway enables a lightweight request security baseline on every explicit, discovered, and public management route. It uses native Caddy matchers and handlers; it is not an SQL injection or XSS rules engine. The **Security** page edits the same global policy exposed by the environment variables below.

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_SECURITY_ENABLED` | `true` | Enables the security baseline globally. |
| `GATEWAY_SECURITY_MAX_REQUEST_BODY_BYTES` | `10485760` | Maximum request body size in bytes. `0` disables the global body limit. |
| `GATEWAY_SECURITY_DENIED_METHODS` | `TRACE,CONNECT` | Comma-separated HTTP methods rejected with 405. |
| `GATEWAY_SECURITY_DENIED_PATH_PREFIXES` | `/.git,/.env` | Comma-separated path prefixes rejected with 403. |
| `GATEWAY_SECURITY_ALLOWED_CIDRS` | empty | Optional direct-client IP/CIDR allowlist. Requests outside it receive 403. |
| `GATEWAY_SECURITY_BLOCKED_CIDRS` | empty | Direct-client IP/CIDR blocklist rejected with 403. |

`remote_ip` evaluates the peer connected directly to Caddy, not an untrusted forwarded header. Confirm that any load balancer in front of the gateway preserves the source address before using CIDR policy. A blocked range takes precedence over an allowlist. Global and route-specific allowlists are cumulative, so a request must satisfy both when both are present.

Explicit routes can add restrictions or override the body limit through their persisted JSON or the route API:

```json
{
	"security": {
		"maxRequestBodyBytes": 52428800,
		"additionalDeniedMethods": ["M-SEARCH"],
		"additionalDeniedPathPrefixes": ["/private"],
		"allowedCidrs": ["10.0.0.0/8"],
		"blockedCidrs": ["10.0.0.5"]
	}
}
```

A positive route body limit replaces the global value. Omitted or `0` inherits it. Set `security.disabled` to `true` only for a route that must bypass the entire baseline. Route overrides are inactive while `GATEWAY_SECURITY_ENABLED=false`. The effective global policy is visible in the Console Platform view and in `/api/status`.

## Certificate Policy

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CERTIFICATE_ISSUER` | `letsencrypt` | Certificate issuer policy: `letsencrypt`, `zerossl`, or `custom`. The legacy `default` alias still maps to `letsencrypt`. |
| `GATEWAY_CERTIFICATE_EMAIL` | empty | ACME contact email. Recommended for production. |
| `GATEWAY_CERTIFICATE_STAGING` | `false` | Uses Let's Encrypt staging when issuer is `letsencrypt`. |
| `GATEWAY_CERTIFICATE_CA_DIRECTORY` | empty | Custom ACME CA directory URL. Required when issuer is `custom`. |
| `GATEWAY_CERTIFICATE_SUBJECTS` | empty | Comma-separated names to request explicitly, including `*.example.com`. |
| `GATEWAY_CERTIFICATE_RENEWAL_WINDOW_RATIO` | `0.3333333333333333` | Fraction of certificate lifetime remaining when the renewal window starts. Must be greater than `0` and less than `1`; `0.5` starts renewal earlier than the default. |
| `GATEWAY_CERTIFICATE_DNS_PROVIDER` | empty | DNS challenge provider. Currently `azure` is supported. |
| `GATEWAY_CERTIFICATE_AZURE_SUBSCRIPTION_ID` | empty | Subscription containing the authoritative Azure DNS zone. |
| `GATEWAY_CERTIFICATE_AZURE_RESOURCE_GROUP` | empty | Resource group containing the authoritative Azure DNS zone. |
| `GATEWAY_CERTIFICATE_AZURE_AUTHENTICATION` | `managedidentity` | `managedidentity` or `appregistration`. |
| `GATEWAY_CERTIFICATE_AZURE_TENANT_ID` | empty | Tenant ID required for App Registration authentication. |
| `GATEWAY_CERTIFICATE_AZURE_CLIENT_ID` | empty | Client ID required for App Registration authentication. |
| `GATEWAY_CERTIFICATE_AZURE_CLIENT_SECRET` | empty | Client secret required for App Registration authentication. Prefer Console entry to shell history. |

The Certificates page is backed by `GET/PUT /api/certificate` and `POST /api/certificate/refresh`. Changes are atomically saved to `GATEWAY_CERTIFICATE_FILE` and then reconciled immediately; the Console reports persistence and Caddy reload failures separately. Saved settings are restored after restart. Client secrets are persisted but never returned by the API. When Azure DNS reconciliation is enabled, blank certificate subscription, DNS zone resource group, and authentication fields are derived from the matching configured DNS zone. Explicit certificate values are preserved, and an NSG resource group is never used as a DNS zone resource group.

Caddy automatically renews managed certificates before expiry. Renewal requires the gateway and Caddy runtime to remain operational, `/data/caddy` to stay persistent, and the configured HTTP-01, TLS-ALPN-01, or DNS-01 challenge to remain usable. The Console's **Enable earlier renewal** action sets the renewal window ratio to `0.5` and reapplies the policy. The CA's ACME Renewal Information (ARI) and Caddy/CertMagic scheduling can still affect the actual renewal time. Reloading TLS only reapplies the current policy; it does not force renewal.

The right side of the Certificates page scans `GATEWAY_CADDY_DATA_DIR/certificates/**/*.crt`. Management is classified against Caddy's accepted `/config/`, not the editable or saved policy: **Managed by active policy**, **History: wildcard-covered**, **History: not referenced**, or **Management unknown**. This is subject matching, not proof that every stored certificate is served or that its issuer is still selected. Unknown, manual-loader, dynamic, and unsupported runtime configurations disable archiving. Historical certificates keep their validity details without presenting an estimated renewal window as an active renewal failure. The displayed window is a lifetime-ratio estimate from the selected policy, not Caddy's ARI schedule. **Refresh status** does not reload Caddy or request certificates. Private-key contents are never returned.

Recent issuance/renewal events are extracted from the last 1,000 buffered runtime log entries, newest first, capped at 50 events. They show the operation, result, domain, broad error category, and retry time when Caddy reported one. Raw ACME error text and arbitrary log fields are excluded to avoid exposing credentials. These are recent observations, not a persistent history or a promise of future execution: restarts and buffer eviction lose events, and absence of an event does not prove renewal health. Inspect restricted runtime logs for detailed DNS, permissions, connectivity, and CA errors.

### Archive Historical Certificates

**Archive certificate** calls `POST /api/certificate/archive` with the inventory's opaque `id`, `fingerprintSha256`, and `confirm: true`. It does not revoke certificates, force renewal, or permanently delete material. The server rechecks the current runtime under the configuration-commit lock and uses CertMagic's issuance and storage-cleaning locks. For affected concrete TLS hosts, a local handshake must return a different, hostname-matching, unexpired certificate; failure or a still-served fingerprint blocks the operation. This check does not validate public trust or revocation. Managed subjects, unknown configurations, pending configuration imports, symbolic links, changed files, and nonstandard/incomplete certificate directories are rejected. The UI button indicates eligibility for these server-side checks, not a guarantee that archiving will succeed.

Only a standard directory containing exactly its `.crt`, `.key`, and `.json` files is moved atomically to `GATEWAY_CADDY_DATA_DIR/certificate-archive/<random-id>/materials`. Its parent is private (`0700`), and a `0600` manifest records the original relative directory, fingerprint, and archive time. File identities and contents are checked before and after the move; a mismatch triggers restoration or an explicit manual-recovery error. Archive storage is not automatically purged and still contains private keys, so retain it in protected backups. This operation is for the single-instance, local-filesystem deployment: do not share storage with another Caddy instance or concurrently edit the Admin API/files outside this control plane. Custom listeners bound only to non-loopback addresses and wildcard route probes require operator review.

To restore, stop the gateway first, back up the complete data directory, inspect the archive manifest, verify the fingerprint, and move `materials` back to its recorded relative directory under the same Caddy data root. Never overwrite an existing directory or newer certificate; resolve that conflict manually. Preserve restrictive ownership and permissions, then restart and check TLS and logs. Do not use archive/restore as a renewal trigger. Whole-system power-loss durability and multi-instance archive coordination are not guaranteed.

Wildcard names require DNS-01. Add both `*.example.com` and `example.com` when the apex is needed, select Azure DNS, and use Let's Encrypt or a custom ACME issuer. Caddy's ZeroSSL issuer does not accept configurable DNS challenges. The Azure identity needs `DNS Zone Contributor` on the authoritative zone. When `*.example.com` is an explicit certificate subject, concrete HTTPS route hosts such as `a.example.com` are added to Caddy's `automatic_https.skip_certificates` list so they use the wildcard instead of triggering separate certificate orders. Explicitly listing both the wildcard and a concrete subject still requests management of both; this is not silently deduplicated. Old files may remain on disk after switching policy and are not evidence of new orders or failed renewal. Wildcards cover exactly one label: they do not cover `example.com` or `a.b.example.com`. Wildcard certificate subjects and wildcard route hosts remain independent; exact route hosts are evaluated before `*.example.com` routes.

## Docker Discovery Labels

The `vm` profile imports these labels from running containers.

| Label | Required | Example | Purpose |
|---|---:|---|---|
| `caddy.enable` | Yes | `true` | Enables route import. |
| `caddy.host` | Yes | `webui.example.com` | Public host name. |
| `caddy.port` | No | `8080` | Upstream container port. |
| `caddy.health_path` | No | `/healthz` | Upstream HTTP health-check path. |
| `caddy.websocket` | No | `true` | Legacy compatibility hint. Caddy proxies WebSocket upgrades automatically; the generated proxy configuration does not require this flag. |
| `exposure.mode` | No | `public` | One of `public`, `protected`, `internal`. |

Containers without `caddy.enable=true` are still shown in discovery. The gateway container itself is excluded. The UI can also bind a discovered container manually; manual bindings require an explicit container port and upstream protocol, are saved as explicit routes, and do not require labels.

Automatic routes select only addresses on a shared gateway/workload network, in deterministic network-name order. Compose service names are a fallback only on shared user-defined networks, not the default bridge. If the gateway cannot be identified by its container hostname or no reachable shared-network address exists, discovery skips the automatic route and returns a warning. Host-network upstreams require explicit routes.

## Docker Discovery Variables

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_DOCKER_ENABLED` | `true` | Enables local Docker discovery. The standalone Azure VM script explicitly sets it to `false`; explicit routes remain available. |
| `GATEWAY_DOCKER_SOCKET` | `/var/run/docker.sock` | Docker socket path inside the gateway container. The sample mounts the host socket read-only. |
| `GATEWAY_DOCKER_ENDPOINT` | empty | Optional HTTP endpoint for a Docker socket proxy, for example `http://docker-socket-proxy:2375`. |

Use `make compose-up-proxy` when you want Docker discovery through a restricted Docker socket proxy instead of mounting `/var/run/docker.sock` into the gateway container directly.

## Azure DNS And NSG

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_AZURE_ENABLED` | `false` | Enables Azure reconciliation through `DefaultAzureCredential`. On Azure, this should use Managed Identity. |
| `GATEWAY_AZURE_MANAGE_DNS` | `true` | Creates, updates, and cleans up gateway-managed Azure DNS A records for public/protected routes. |
| `GATEWAY_AZURE_MANAGE_NSG` | `true` | Creates, updates, or deletes the gateway-managed VM NSG rule for public listener ports. The rule retains 80/443 for default ingress and ACME. |
| `GATEWAY_AZURE_SUBSCRIPTION_ID` | empty | Azure subscription ID. `AZURE_SUBSCRIPTION_ID` is also accepted. |
| `GATEWAY_AZURE_RESOURCE_GROUP` | empty | Default resource group for DNS zones. |
| `GATEWAY_AZURE_DNS_ZONE` | empty | Legacy single Azure DNS zone name. |
| `GATEWAY_AZURE_DNS_ZONES` | empty | JSON array of `{name,resourceGroup}` entries. Hostnames use the longest matching zone suffix. |
| `GATEWAY_AZURE_NSG_RESOURCE_GROUP` | `GATEWAY_AZURE_RESOURCE_GROUP` | Resource group containing the Network Security Group. |
| `GATEWAY_AZURE_NSG_NAME` | empty | Network Security Group name for VM public-listener inbound rule reconciliation. |
| `GATEWAY_AZURE_NSG_PRIORITY` | `120` | Priority for the managed VM NSG allow rule. |
| `GATEWAY_AZURE_NSG_SOURCE_PREFIXES` | `*` | Comma-separated source CIDR prefixes for the managed VM NSG allow rule. |
| `GATEWAY_PUBLIC_IP_ADDRESS` | empty | Required VM public IPv4 address when DNS management has public routes. Egress IP discovery is intentionally not used. |

Required Managed Identity roles:

- `DNS Zone Contributor` on the DNS zone or containing scope.
- `Network Contributor` on the NSG or containing scope when NSG reconciliation is enabled.

The standalone VM deployment creates a system-assigned managed identity with `az vm create --assign-identity`; role assignment remains an explicit operator step. The gateway uses `DefaultAzureCredential`, so no managed-identity secret is configured in the Console. `Configured` validates that required IDs and names are present.

In **Settings > Azure Integration**, **Check permissions** queries `Microsoft.Authorization/permissions` for every configured DNS zone and the target NSG. It verifies the effective `read`, `write`, and `delete` actions used for DNS A records and NSG security rules without changing either resource. The check uses the identity selected by `DefaultAzureCredential` in the running process: normally Managed Identity on the VM, but Azure CLI or another developer credential can be selected locally. New or changed role assignments can take several minutes to propagate; retry the check after propagation. A successful check confirms effective ARM actions, while Reconcile remains the end-to-end operational test.

Reconcile upserts desired A records, lists gateway-managed A records for cleanup, waits for the NSG rule operation, and reports counts, warnings, or the Azure API error on the Platform page. It is not a general drift monitor for unrelated DNS records or NSG rules.

Cleanup behavior:

- DNS cleanup deletes only A records with both `managed-by=ai-docker-farm-gateway` and the current `gateway-instance-id` metadata. Deletes and updates require the read ETag; creation uses `If-None-Match: *`. A 412 is reported rather than retried unconditionally. Unchanged owned records are not rewritten.
- Deleting, disabling, or internalizing a route removes its managed DNS record on the next reconcile.
- Upstream health failures are reported in route status but do not remove DNS records; disable or delete a route to withdraw DNS without creating probe-driven DNS cache churn.
- The NSG rule is shared by this instance's public/protected routes, updates when their listener-port set changes, and is deleted only when none remain, unless `GATEWAY_MANAGEMENT_HOST` is set. Its name ends in the instance ID and its description must identify the same owner. Unchanged rules are not rewritten; priority conflicts are returned by Azure, not resolved by overwriting another rule. The NSG SDK does not provide the DNS-style conditional-write guarantee.

### Ownership And Upgrade

Azure-enabled gateways create a stable random identity in `GATEWAY_STATE_DIR/azure-instance-id`, with mode `0600`. Preserve this file when recovering the same logical gateway. Do not clone it into an independent gateway: before starting a clone, remove only its copied identity file so it generates a new owner. Never run independent writers with the same identity or shared state directory. Configuration ZIP transfer does not transfer ownership.

Unknown-owner and other-instance DNS records are never taken over. Legacy records with only the generic `managed-by` marker are retained; desired names that collide with them report an ownership error. For an intentional migration, stop the previous writer, back up DNS and local state, verify each record belongs to this gateway, then explicitly add this gateway's `gateway-instance-id` metadata through your Azure administration workflow. Records not present in the desired route set may be cleaned up after adoption, so review the complete set first.

The old fixed NSG rule `Allow-AIDockerFarm-Gateway-HTTPHTTPS` is retained. Use a free priority for the new instance-scoped rule, verify ingress, then remove the legacy rule only after confirming no other gateway depends on it. Reusing its priority without migration can cause an Azure conflict. Removing old access rules is an explicit operator action, not automatic cleanup.

## VM Deployment Notes

For a new standalone Azure VM, run `make azure-vm-deploy`, or invoke `deploy/vm/deploy.sh` from Cloud Shell with the `curl`/`wget` commands in [deployment.md](deployment.md). The script interactively selects the region, VNet, subnet, VM size, and disk; installs Docker; disables local Docker discovery; starts the gateway with host networking; binds management to `127.0.0.1:8080`; and prints its IPs, managed identity, admin token, SSH command, and management tunnel.

After the script completes, manually configure DNS A records, explicit routes, backend NSG/firewall access, certificate policy, and any Azure DNS role assignment. The script does not change those resources.

For an existing or co-located Docker VM:

1. Assign a managed identity when Azure DNS or NSG reconciliation is required.
2. Grant the identity the Azure roles above.
3. Keep SSH/private management access through Tailscale, WireGuard, Bastion, VPN, or an equivalent private path.
4. Start with `IMAGE=<published-image> DOCKER_NETWORKS=<network1,network2> ./start.sh start`.

When Azure NSG reconciliation is enabled, the gateway manages inbound access for 80/443 and every enabled public listener port. It never opens 8080. `start.sh` and the standalone VM deployment bind the management UI to `127.0.0.1:8080` on the host. Standard Container + Socket deployments publish only 80/443 unless the operator adds explicit host-port mappings.

## Runtime Probes

- `/livez` returns success while the Go control plane is running.
- `/readyz` returns success only while the required Caddy child process is ready.
- `/healthz` is a compatibility alias for `/readyz`.
