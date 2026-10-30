# Roadmap And Capability Gaps

[简体中文](roadmap.zh-CN.md)

This document tracks what is currently implemented and what still needs to be completed before the gateway behaves like a small Azure Application Gateway for Docker and Azure workloads.

## Implemented In The MVP

- Single container image with the Go control plane and embedded Caddy runtime.
- Management API and embedded Alpine.js UI.
- Listener, backend-pool, and routing-rule CRUD with versioned JSON persistence and legacy route migration.
- Caddy JSON config rendering and Admin API reloads.
- VM profile Docker discovery through the Docker socket.
- Docker label route hints for `caddy.enable`, `caddy.host`, `caddy.port`, `caddy.websocket`, and `exposure.mode`.
- Manual bind from a discovered Docker container to a persisted explicit route.
- Public, internal, and protected exposure modes at the Caddy routing layer.
- Azure DNS A record reconciliation through `DefaultAzureCredential`.
- Cleanup for stale gateway-managed Azure DNS A records.
- VM NSG reconciliation for public listener ports through `DefaultAzureCredential`, retaining 80/443 for ACME and default ingress.
- Cleanup for the gateway-managed VM NSG inbound rule when no public routes remain.
- Interactive standalone Azure VM provisioning with VNet/subnet selection, static public IP, restricted NSG, managed identity, Docker installation, and persistent gateway state.
- Authenticated management API through an admin token.
- Multi-token management API allowlist for small-team operation.
- Configurable protected-route policy with bearer token, `X-Admin-Token`, and optional custom header matching.
- Atomically persisted certificate UI/API controls with explicit subjects, Azure DNS-01 wildcard issuance, Managed Identity/App Registration authentication, secret masking, and refresh-triggered Caddy reloads.
- Route and upstream health checks with route-level error reporting in API/UI status.
- Audit log for route changes, manual Docker binds, reconcile runs, DNS changes, and NSG changes.
- NSG rule priority and source-prefix policy controls for the managed VM inbound rule.
- Docker socket proxy deployment option for VM profile discovery.
- E2E routing test script for Caddy plus a sample Docker service.
- Supervised Caddy lifecycle with `/livez` and `/readyz` orchestration probes.
- Serialized configuration commits, generation-checked status publication, last-known-good Docker discovery routes, and atomic route file replacement.
- Internal-route CIDR enforcement, deterministic path priority, homogeneous upstream transports, and gateway credential stripping.
- Multi-zone Azure DNS reconciliation with an explicit ingress public IP.
- Single-container lifecycle script for existing hosts and a Cloud Shell/local Azure CLI deployment script for standalone Azure VMs.

## Further Hardening

### JOSE v4 Remediation And Scoped VEX (2026-10-30)

- Post-review policy isolation: all three Trivy scans ignore ambient filtering/config/ignore rules while retaining transport/cache settings; only the verified VEX may filter findings. Passed 41 scan/publish regressions, including an unfixed unrelated vulnerability under hostile filters, and real same-image control/hostile-environment comparisons. This shell-only follow-up did not rebuild images or close the ARM64 runtime gate.
- Added checksum-pinned Smallstep source patches using go-jose v4.1.6, preserving all-audience validation, legacy JWT builder interfaces and XEdDSA key IDs. Invalid Ed25519 key pairs/short HMAC keys remain rejected. The default Caddy PKI/local ACME/Azure modules were not removed.
- Builds verify exact source/binary module graphs and emit target-specific hashes, source/patch provenance and OpenPGP-only VEX. The scan wrapper locks the image ID, verifies both executables, refuses extra Go binaries, and separately scans the patched upstream base versions to avoid local-replacement version blind spots. Both multi-architecture candidates must pass before target promotion; Trivy is now required.
- Verification passed: dependency race suites, full real-Caddy Go race/vet including isolated internal-CA issuance, 37 Node regressions, Compose/shell checks and final-image desktop/390px browser workflows. AMD64 raw findings are now two OpenPGP module records; scoped applicable findings and patched-base SBOM findings are zero in this snapshot.
- ARM64 binaries and their independent source-graph evidence cross-build successfully; full ARM64 runtime/image and real cloud/ACME gates remain open. No image was published or deployed. These local patches require maintenance until compatible, fixed upstream releases can replace them; details and removal criteria are in [operations](operations.md#local-jose-migration-and-evidence-bound-scanning).

### Dependency And Configuration Repairs (2026-10-30)

This records the earlier same-day baseline; the JOSE remediation above supersedes its residual-risk status.

- Latest-stable follow-up implemented in order: Go 1.27.3/Alpine 3.24.3 and HAProxy 3.4.7, Caddy's CEL 0.33.1/QUIC 0.64.1/OpenTelemetry 1.47.0/Prometheus family and other transitive updates, then Azure Network SDK v12.1.0. Reviewed and pinned the commit-based updates; runtime dependency metadata is checked rather than inferring versions from requested replacements.
- Added real HTTP/3 routing/authentication and metrics regressions, plus Azure SDK request-version/path/body, missing-resource and read-error checks. Production topology and ownership semantics are unchanged; real cloud API availability remains an unverified gate.
- Upgraded the control-plane and custom Caddy dependency graphs, shared Go build toolchain, runtime base and vendored Alpine.js. Added controlled socket-proxy/httpbin rebuilds; production requires a preloaded socket-proxy image rather than building or pulling implicitly. Exact versions and preparation steps are in [operations](operations.md#dependency-upgrade-and-configuration-fixes-2026-10-30).
- Fixed immediate security/token/certificate updates to persist only after Caddy accepts the candidate, preserve pending routes, and restore the previous runtime after load/persistence failure. Regression coverage includes cancellation, lost acknowledgments, rollback failures and cloud errors after commit; this does not guarantee crash atomicity.
- Canonicalized empty and `/` root paths across CRUD, bundle import and v1/v2/v3 persistence loading. Ambiguous legacy files are rejected without modifying them, rather than silently shadowing a route.
- Verification passed: full Go race/vet with real upgraded Caddy, 24 Node tests, Compose validation, native AMD64 gateway/companion builds, mock-socket and non-root httpbin smoke tests, and desktop/390px browser workflows. ARM64 control-plane/Caddy/httpbin cross-compilation and ELF checks passed. ARM64 runtime/full-image verification remains blocked by the host's `exec format error`; earlier ARM64 results below are historical, not validation of this upgrade.
- Local AMD64 gateway findings decreased from 62 to 5 package records; both rebuilt companions have zero findings in the same scanner database. Unpatched go-jose v3 and OpenPGP advisories remain a security release gate. No image was published or deployed, and no real cloud/ACME, production certificate or destructive E2E operations were performed.
- Next gate: resolve or explicitly assess the remaining advisories and validate complete ARM64 images in an approved native/emulated environment before considering a release.

### Multi-Architecture Release (2026-09-22)

- Implemented and published one `latest` index for `linux/amd64` and `linux/arm64`. Native cross-compilation, immutable candidate validation, shipping-image smoke checks, target-change detection and post-promotion digest verification are shared by `start.sh push` and `make docker-push`; local `build` remains single-platform.
- Verification passed: 15 publishing regression tests, full native Go race/vet, QEMU ARM64 Caddy/certificate tests covering HTTP/TLS/Host/auth/storage locks, both candidate containers' health/auth/ELF checks, and registry platform/digest verification. Ordinary pull selects AMD64 on the development host. Release index, per-platform sizes, setup and rollback details are in [operations](operations.md#multi-architecture-publishing).
- Remaining gates: native Ubuntu ARM64 deployment/performance and real cloud/ACME were not exercised. Dependency security exceptions remain, including newly recorded OpenTelemetry findings in unchanged AMD64 binaries. Publishing is not production deployment or a clean security attestation. Next smallest step: authorized ARM64 staging validation and separate dependency-risk remediation.

### Dependency Upgrade (2026-09-22)

- Local implementation complete: Go 1.26.8, Caddy 2.11.4, xcaddy 0.4.7, aligned CertMagic 0.25.4, Azure SDK and Go security dependencies, Alpine Linux 3.22.6, Alpine.js 3.17.4, socket proxy v0.5.0 and sample httpbin 2.25.0. Base/companion images are digest-pinned; Azure DNS 0.6.0 is retained. HTTPS upstream Host compatibility is explicitly preserved with regression coverage.
- Verification passed: final image build, full Go race/vet with the real Caddy binary, Node 6/6, browser login/forms/certificate archive fixtures and 401/503 checks at desktop/390px, Compose rendering and isolated read-only socket proxy/httpbin tests. Control-plane source scanning found no reachable vulnerabilities; the gateway OS image scan found none.
- Security release gate remains open: Caddy CEL/OpenPGP advisories, socket proxy OpenSSL findings, and the sample httpbin's older Go toolchain. See [the dependency baseline](operations.md#dependency-upgrade-baseline-2026-09-22) for versions, evidence and limitations. No deployment, real Azure/public ACME or production archive was attempted.
- Next smallest step: resolve or explicitly assess the upstream exceptions and rescan exact artifacts before authorized staging. A successful upgrade/build is not a claim of zero vulnerabilities.

### Certificate Lifecycle Remediation (2026-09-22)

- Implemented active-policy versus historical/unknown inventory, independent estimated validity windows, bounded recent issuance/renewal events, and confirmed reversible archive with runtime, TLS, path, file-identity, configuration-lock and CertMagic storage-lock checks.
- Local verification: full Go race tests and vet; renderer/API/security regression tests; real Caddy 2.10.2 active-config reading and manual-loader rejection; Node UI regressions (6/6); browser fixture tests for filtering, cancel/confirm archive, events, desktop and 390px layout. No public CA requests or production certificate removal were performed.
- Remaining environment gates: real ACME renewal, production storage/archive rehearsal, shared-storage/crash fault injection. Multi-instance archive coordination and whole-system power-loss durability are not guaranteed. Next smallest step: back up data and validate the new inventory against the deployed runtime in an authorized test environment before exercising archive; see the operations guide.

### Prioritized Audit Remediation (2026-09-22)

- Phases 1-4 implemented: HTTP/TLS separation, management API credential preservation, restart-safe desired/applied revisions, concurrent Apply confirmation, instance-owned Azure resources, shared-network Docker discovery, and non-authentication 503 handling.
- Phase 5 implemented: bounded concurrent health checks outside the commit lock, reverse-block audit tail reads, no-op Azure write suppression, shared atomic JSON persistence, and removal of two unused wrappers. Compatibility APIs are retained.
- Phase 6 local verification complete: full `go test -race ./...` and `go vet ./...` passed, including isolated Caddy 2.10.2 HTTP/TLS/auth tests using `CADDY_TEST_BIN`; Node authentication regressions passed (4/4). Browser checks passed for real login, injected 401/503 responses, pending drafts, and Apply through to actual Caddy forwarding; a 390px viewport had no horizontal overflow. JS/shell syntax, editor diagnostics, and `git diff --check` passed. SDK tests use mock transports, not cloud resources. Million-event audit tail benchmark: approximately 0.15 ms/op on the test host, not a production throughput claim.
- Optional/environment gates not run: real Azure integration, public ACME issuance, full Docker-stack E2E, crash injection during multi-file import, and production load tests. The existing E2E script tears down its sample stack and volumes, so it was not run against current workloads. Multi-file import remains non-atomic across process crashes.
- Next smallest step: back up state and rehearse the v3/ownership migration in an explicitly authorized test environment before production rollout; see the operations guide. No deployment or cloud resources were changed in this remediation.

- Entra ID/OIDC should replace token-based management auth for production multi-user governance.
- Health checks currently use simple HTTP status probes; future work can add per-route intervals, thresholds, and active/passive policy controls.
- The E2E test is a local Docker script and should be promoted into CI once the target runner can expose ports 80 and 8080.
- Active-active instances require an external route store with concurrency control; multiple writers cannot share `routes.json` safely.

## Routing Resource Model

The v2 resource model and the Routes UI use three reusable resources, wrapped in a v3 disk envelope:

- Listener: a frontend hostname, port, and HTTP/HTTPS protocol.
- Backend pool: a named set of IP addresses or DNS names.
- Routing rule: selects one listener and backend pool, then defines backend protocol/port, path, health path, exposure, and WebSocket behavior.

The store compiles these resources into the runtime route model consumed by reconciliation, health checks, Azure, and Caddy. Legacy v1/v2 files migrate atomically to a v3 envelope with desired/applied snapshots and revisions, taking existing contents as an applied baseline. Configuration ZIPs remain v2. The old route API remains as a compatibility adapter for Docker bind and existing clients.

Certificate policy is still managed globally by subject rather than as a separate per-listener binding. Docker-discovered service identities also remain runtime inputs rather than persisted first-class resources.

## Current UI Status Meanings

- Azure `Enabled: No` means the Azure reconcilers are available but disabled in config.
- Azure `Configured: No` means required settings such as subscription, resource group, DNS zone, or NSG name are missing.
- Docker `Active: No` in the local preview usually means the preview was started with `GATEWAY_DOCKER_ENABLED=false` or without a mounted Docker socket.
- Docker `Active: No` is expected on standalone gateway VMs, which use explicit private-backend routes and intentionally disable local discovery.

## Recommended Next Milestone

Close the dependency security release gates above before production rollout. Then promote Entra ID/OIDC management auth and CI-backed E2E coverage. The gateway now has the operational loop in place: deploy a container, bind a route, reconcile network state, obtain HTTPS, audit the change, and show health/error state in the UI.
