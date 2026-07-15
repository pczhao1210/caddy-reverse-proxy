# AI Docker Farm Edge Gateway

[简体中文](README.zh-CN.md)

[![Deploy to Azure](https://aka.ms/deploytoazurebutton)](https://portal.azure.com/#create/Microsoft.Template/uri/https%3A%2F%2Fraw.githubusercontent.com%2Fpczhao1210%2Fcaddy-reverse-proxy%2Fmain%2Fdeploy%2Faci%2Fazuredeploy.json)

AI Docker Farm Edge Gateway is a self-hosted ingress platform for Docker and Azure workloads. It is designed as a single container image that can receive Internet traffic directly on ports 80 and 443, route requests through an embedded Caddy runtime, expose a management UI, and coordinate Docker discovery plus Azure DNS and network state.

## MVP scope

- Single gateway container image.
- Embedded Caddy runtime managed through a localhost-only admin endpoint.
- Go control plane API with an embedded static management UI.
- VM profile for co-located Docker workloads and label-based discovery.
- ACI profile for explicit route configuration.
- Automatic HTTPS through HTTP-01 or Azure DNS-01, including wildcard certificates.
- Managed Identity or App Registration authentication for Azure DNS-01.

## Deployment profiles

### VM profile

The gateway runs on the same Ubuntu VM as the workloads. It joins the Docker network used by services, discovers containers through a restricted Docker socket or socket proxy, and forwards public traffic to internal container names and ports.

### ACI profile

The gateway runs as a private VNet-injected Azure Container Instance behind Standard Public Load Balancer. The Load Balancer forwards TCP 80/443 without terminating TLS, so Caddy retains certificate and multi-domain routing ownership. ACI has no local Docker discovery; routes are explicit and upstreams use private VNet addresses or private DNS.

## Quick Start

Start the VM profile directly from the repository:

```sh
./start.sh start
```

The script builds the image when it is missing and starts exactly one gateway container. It publishes 80/443 publicly, binds the Console to `127.0.0.1:8080`, and persists all state under `~/docker_files/caddy-reverse-proxy`. When `.env` does not contain a custom admin token, the script generates one, prints it once at startup, and stores it with the persisted state.

Open `http://127.0.0.1:8080`, sign in with that token, then configure routes and certificates in the Console. To request `*.example.com`, open **Network → Certificates**, add `*.example.com` and `example.com` as subjects, select Azure DNS, and provide the Azure authentication settings. The apex subject is separate because a wildcard does not cover `example.com` itself.

Attach the gateway to existing workload networks when container-name routing is needed:

```sh
DOCKER_NETWORKS=frontend,internal ./start.sh start
```

Other lifecycle commands accept either form, such as `build` or `--build`:

```sh
./start.sh build
./start.sh push
./start.sh stop
./start.sh restore
```

`push` uses the username reported by the current Docker Hub login; set `PUSH_IMAGE=<organization>/caddy-reverse-proxy:tag` to override it. `stop` preserves the data directory. `restore` removes only the managed container, the selected image, and the guarded project directory below `~/docker_files`; it does not modify `.env` or Git files. Run `./start.sh help` for port, image, network, and path overrides.

For ACI, use the Deploy to Azure button above. The template creates the VNet, ACI, Standard Load Balancer, NAT Gateway, Azure Files persistence, and managed identities. Only a published image and the admin token are required. Supplying `dnsZones` during deployment also grants both the control-plane UAMI and Caddy's system identity `DNS Zone Contributor`; other certificate settings can be entered later in the Console.

## Repository layout

```text
backend/             Go control plane, Caddy integration, embedded UI
config/              Example platform and route configuration
deploy/vm/           Docker Compose deployment for the co-located VM profile
deploy/aci/          Private ACI + Standard Load Balancer Bicep deployment
docs/                Operations and security documentation
```

## Current implementation status

The implementation includes the management API, embedded Alpine.js UI, Docker label discovery, explicit and wildcard routes, Caddy config rendering, protected/internal/public exposure modes, supervised Caddy lifecycle, liveness/readiness endpoints, atomic route and certificate persistence, Azure DNS-01 wildcard certificates, route health checks, audit logging, multi-zone Azure DNS reconciliation, and private ACI + Standard Load Balancer infrastructure.

See [docs/deployment.md](docs/deployment.md) for production deployment, [docs/operations.md](docs/operations.md) for runtime options, and [docs/roadmap.md](docs/roadmap.md) for remaining capability gaps.
