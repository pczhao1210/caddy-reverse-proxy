# Roadmap And Capability Gaps

[简体中文](roadmap.zh-CN.md)

This document tracks what is currently implemented and what still needs to be completed before the gateway behaves like a small Azure Application Gateway for Docker and Azure workloads.

## Implemented In The MVP

- Single container image with the Go control plane and embedded Caddy runtime.
- Management API and embedded Alpine.js UI.
- Explicit route CRUD with JSON file persistence.
- Caddy JSON config rendering and Admin API reloads.
- VM profile Docker discovery through the Docker socket.
- Docker label route hints for `caddy.enable`, `caddy.host`, `caddy.port`, `caddy.websocket`, and `exposure.mode`.
- Manual bind from a discovered Docker container to a persisted explicit route.
- Public, internal, and protected exposure modes at the Caddy routing layer.
- Azure DNS A record reconciliation through `DefaultAzureCredential`.
- Cleanup for stale gateway-managed Azure DNS A records.
- VM NSG inbound 80/443 rule reconciliation through `DefaultAzureCredential`.
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
- Serialized reconciliation, last-known-good Docker discovery routes, and atomic route file replacement.
- Internal-route CIDR enforcement, deterministic path priority, homogeneous upstream transports, and gateway credential stripping.
- Multi-zone Azure DNS reconciliation with an explicit ingress public IP.
- Single-container lifecycle script for existing hosts and a Cloud Shell/local Azure CLI deployment script for standalone Azure VMs.

## Further Hardening

- Entra ID/OIDC should replace token-based management auth for production multi-user governance.
- Health checks currently use simple HTTP status probes; future work can add per-route intervals, thresholds, and active/passive policy controls.
- The E2E test is a local Docker script and should be promoted into CI once the target runner can expose ports 80 and 8080.
- Active-active instances require an external route store with concurrency control; multiple writers cannot share `routes.json` safely.

## Current UI Status Meanings

- Azure `Enabled: No` means the Azure reconcilers are available but disabled in config.
- Azure `Configured: No` means required settings such as subscription, resource group, DNS zone, or NSG name are missing.
- Docker `Active: No` in the local preview usually means the preview was started with `GATEWAY_DOCKER_ENABLED=false` or without a mounted Docker socket.
- Docker `Active: No` is expected on standalone gateway VMs, which use explicit private-backend routes and intentionally disable local discovery.

## Recommended Next Milestone

Promote Entra ID/OIDC management auth and CI-backed E2E coverage next. The gateway now has the operational loop in place: deploy a container, bind a route, reconcile network state, obtain HTTPS, audit the change, and show health/error state in the UI.
