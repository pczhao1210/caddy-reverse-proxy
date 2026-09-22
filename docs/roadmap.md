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
