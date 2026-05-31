# AI Docker Farm Edge Gateway

[简体中文](README.zh-CN.md)

AI Docker Farm Edge Gateway is a self-hosted ingress platform for Docker and Azure workloads. It is designed as a single container image that can receive Internet traffic directly on ports 80 and 443, route requests through an embedded Caddy runtime, expose a management UI, and coordinate Docker discovery plus Azure DNS and network state.

## MVP scope

- Single gateway container image.
- Embedded Caddy runtime managed through a localhost-only admin endpoint.
- Go control plane API with an embedded static management UI.
- VM profile for co-located Docker workloads and label-based discovery.
- ACI profile for explicit route configuration.
- Public DNS and automatic HTTPS as the primary certificate path.
- Managed identity only for Azure DNS and NSG integration.

## Deployment profiles

### VM profile

The gateway runs on the same Ubuntu VM as the workloads. It joins the Docker network used by services, discovers containers through a restricted Docker socket or socket proxy, and forwards public traffic to internal container names and ports.

### ACI profile

The gateway runs in Azure Container Instances. ACI does not provide local Docker discovery, so routes must come from explicit configuration or the management API. Upstreams must be reachable from the ACI container group through public networking, VNet injection, Private Link, or private DNS.

## Quick Start

Create a local env file from the sample. `.env` is ignored by Git.

```sh
cp .env.example .env
```

Build the single image:

```sh
make docker-build
```

Run it with Docker discovery enabled:

```sh
make docker-run
```

`make docker-run` uses Docker's default bridge network and does not create a custom network. With the default bridge network, Docker discovery uses inspected container IP addresses for upstreams when they are available.

If you put the gateway on a new custom Docker network, it cannot automatically proxy containers that remain only on the default `bridge` network unless there is a reachable path between them. Attach workloads to the gateway network, attach the gateway to multiple networks, or route through a host-published port.

Best practice for a VM deployment is a normal gateway container attached to the workload networks it must serve, with only the gateway's 80/443/management ports published on the host. `network_mode: host` is a separate mode, not a way to attach the gateway to multiple Docker networks.

Configuration values live in `.env` by default. Edit that file before running `make docker-run` or `make compose-up`; see [docs/operations.md](docs/operations.md) for every option and its meaning.

Open the management UI at `http://localhost:8080` and sign in with the token `change-me`.

Host network mode can proxy traffic too, especially to host-local upstreams like `http://127.0.0.1:3000`, but it is Linux-oriented, removes container network isolation for the gateway, and is better treated as an explicit deployment choice instead of the default preview mode.

## Repository layout

```text
backend/             Go control plane, Caddy integration, embedded UI
config/              Example platform and route configuration
deploy/vm/           Docker Compose deployment for the co-located VM profile
deploy/aci/          ACI deployment starter template
docs/                Operations and security documentation
```

## Current implementation status

This repository starts with a focused MVP: the management API, embedded Alpine.js UI, Docker label discovery, manual bind, explicit routes, Caddy config rendering, protected/internal/public exposure modes, Caddy process management, runtime certificate issuer policy controls, route health checks, audit logging, Docker socket proxy deployment, and Azure DNS/NSG reconciliation through `DefaultAzureCredential` when enabled.

See [docs/operations.md](docs/operations.md) for operations details and [docs/roadmap.md](docs/roadmap.md) for the current capability gaps and recommended next milestone order.
