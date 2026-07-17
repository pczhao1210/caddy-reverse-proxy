# AI Docker Farm Edge Gateway

[简体中文](README.zh-CN.md)

AI Docker Farm Edge Gateway is a self-hosted ingress platform for Docker and Azure workloads. It is designed as a single container image that can receive Internet traffic directly on ports 80 and 443, route requests through an embedded Caddy runtime, expose a management UI, and coordinate Docker discovery plus Azure DNS and network state.

Published image: `docker.io/pczhao1210/caddy-reverse-proxy:latest`  
Current digest: `sha256:0e75a5bbeccb3b9354516e757bb805803a501cf6cca03988028e03030aa94c52`

## MVP scope

- Single gateway container image.
- Embedded Caddy runtime managed through a localhost-only admin endpoint.
- Go control plane API with an embedded static management UI.
- VM deployment with explicit routes and optional label-based discovery for co-located Docker workloads.
- Interactive standalone Azure VM provisioning from Cloud Shell or a local Azure CLI environment.
- Automatic HTTPS through HTTP-01 or Azure DNS-01, including wildcard certificates.
- Managed Identity or App Registration authentication for Azure DNS-01.
- Built-in request security baseline for body-size limits, denied methods and paths, and IP/CIDR access policy.
- Console-managed security policy, login-token rotation, desired deployment mode, and Azure DNS/NSG settings.

## Deployment modes

### Co-located VM

The gateway runs on the same Ubuntu VM as the workloads. It joins the Docker network used by services, discovers containers through a restricted Docker socket or socket proxy, and forwards public traffic to internal container names and ports.

### Standalone Azure VM

The gateway runs on a dedicated Ubuntu VM with one Standard static public IP. It reaches backends through private VNet addresses or private DNS and uses explicit routes. No Load Balancer or NAT Gateway is required.

## Quick Start

Start the VM profile directly from the repository:

```sh
./start.sh start
```

The script pulls `pczhao1210/caddy-reverse-proxy:latest` before every startup and starts exactly one gateway container. It publishes 80/443 publicly, binds the Console to `127.0.0.1:8080`, and persists all state under `~/docker_files/caddy-reverse-proxy`. When `.env` does not contain a custom admin token, the script generates one, prints it once at startup, and stores it with the persisted state. `start.sh` requires Docker to be available; use the interactive local deployment mode below when Docker is not installed yet.

Open `http://127.0.0.1:8080`, sign in with that token, then configure routes, security, settings, and certificates in the Console. Security and token changes apply immediately. Deployment and Azure integration changes are staged for restart because they affect startup-time clients and topology; the launcher must still provide the selected Docker network, socket, and port mappings. To request `*.example.com`, open **Certificates**, add `*.example.com` and `example.com` as subjects, select Azure DNS, and provide the Azure authentication settings. The apex subject is separate because a wildcard does not cover `example.com` itself.

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

`build` and `push` use `pczhao1210/caddy-reverse-proxy:latest` by default. Set `IMAGE` or `PUSH_IMAGE` to select another repository; `start.sh` always replaces any supplied tag or digest with `latest`. `stop` preserves the data directory. `restore` removes only the managed container, the selected image, and the guarded project directory below `~/docker_files`; it does not modify `.env` or Git files. Run `./start.sh help` for port, image repository, network, and path overrides.

The interactive deployment script supports two modes: create a standalone Azure VM, or deploy only the gateway container on the current machine. Azure mode requires Azure Cloud Shell or local Bash 4+ with Azure CLI; local mode requires Bash 4+ and checks Docker. It can offer to install missing Docker on Debian/Ubuntu after confirmation. It can also explain and offer to repair a stopped service or missing access to the standard socket. Choose one block below; each downloads the script to a temporary file and runs it only after a successful download:

```bash
deploy_script=$(mktemp) && curl -fsSL -o "$deploy_script" https://raw.githubusercontent.com/pczhao1210/caddy-reverse-proxy/main/deploy/vm/deploy.sh &&
	bash "$deploy_script"
status=$?
rm -f "${deploy_script:-}"
test "$status" -eq 0
```

```bash
deploy_script=$(mktemp) && wget -qO "$deploy_script" https://raw.githubusercontent.com/pczhao1210/caddy-reverse-proxy/main/deploy/vm/deploy.sh &&
	bash "$deploy_script"
status=$?
rm -f "${deploy_script:-}"
test "$status" -eq 0
```

Select the first mode to create Azure infrastructure. It selects the subscription, region, VNet, subnet, VM size, and OS disk; creates a static public IP, NSG, NIC, managed identity, and Ubuntu VM; installs Docker; and starts the gateway. Select the second mode to deploy on the current host. It reuses `start.sh` from a checkout or installs the minimal launcher files under `~/caddy-reverse-proxy`, then starts only the container without changing Azure infrastructure or DNS.

From a local clone, use `make azure-vm-deploy` or `make container-deploy`. The deployment image defaults to `pczhao1210/caddy-reverse-proxy:latest`; set `IMAGE` to override it. Azure mode supports `ROLLBACK_ON_ERROR=false`; local mode supports the same `CONTAINER_NAME`, `DATA_DIR`, port, and `DOCKER_NETWORKS` overrides as `start.sh`. See [docs/deployment.md](docs/deployment.md) for permissions, persistence, and network constraints.

## Repository layout

```text
backend/             Go control plane, Caddy integration, embedded UI
config/              Example platform and route configuration
deploy/vm/           Standalone Azure VM script and Docker Compose deployments
docs/                Operations and security documentation
```

## Current implementation status

The implementation includes the management API, embedded Alpine.js UI, optional Docker label discovery, explicit and wildcard routes, Caddy config rendering, protected/internal/public exposure modes, a lightweight request security baseline, supervised Caddy lifecycle, liveness/readiness endpoints, atomic route and certificate persistence, Azure DNS-01 wildcard certificates, route health checks, audit logging, multi-zone Azure DNS reconciliation, and interactive standalone Azure VM provisioning.

See [docs/deployment.md](docs/deployment.md) for production deployment, [docs/operations.md](docs/operations.md) for runtime options, and [docs/roadmap.md](docs/roadmap.md) for remaining capability gaps.
