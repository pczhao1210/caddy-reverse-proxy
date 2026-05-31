# Operations Guide

[简体中文](operations.zh-CN.md)

This guide consolidates local startup, VM/ACI deployment notes, Docker discovery labels, and runtime configuration.

## Environment File

`.env.example` is the sample file. Create a local `.env` before running the gateway:

```sh
cp .env.example .env
```

`.env` is ignored by Git. Keep local tokens and deployment-specific values there. The values are passed to the gateway container as environment variables; paths such as `/config/platform.example.json` and `/data/platform/routes.json` are paths inside the container.

Boolean values accept `1`, `true`, `yes`, `y`, or `on` as true. Any other non-empty value is treated as false.

## Quick Local Run

```sh
cp .env.example .env
make docker-build
make docker-run
```

`make docker-run` uses Docker's default bridge network and does not create a custom network. The management UI is available at `http://localhost:8080`; the default sample token is `change-me`.

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
	-p 80:80 -p 443:443 -p 8080:8080 \
	-v /var/run/docker.sock:/var/run/docker.sock \
	--env-file .env \
	caddy-reverse-proxy:latest

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
| `make docker-build` | Build `IMAGE`, default `caddy-reverse-proxy:latest`. |
| `make docker-push` | Check Docker daemon/login state and push `IMAGE`. |
| `make docker-run` | Run the image locally with `ENV_FILE`, default `.env`, on Docker bridge. |
| `make compose-up` | Start the VM sample stack. |
| `make compose-up-proxy` | Start the VM stack with Docker discovery through a socket proxy. |
| `make test-e2e` | Exercise Caddy routing with the sample VM stack. |
| `make compose-down` | Stop the VM sample stack. |

Override the image when pushing to a registry:

```sh
make docker-build IMAGE=registry.example.com/team/caddy-reverse-proxy:latest
make docker-push IMAGE=registry.example.com/team/caddy-reverse-proxy:latest
```

## Core Runtime Variables

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_PROFILE` | `vm` | Deployment profile. Use `vm` for a Docker host or VM; use `aci` for Azure Container Instances with explicit routes. |
| `GATEWAY_ADMIN_TOKEN` | `change-me` | Admin token for the management API and protected routes. Replace this before real deployment. |
| `GATEWAY_ADMIN_TOKENS` | empty | Optional comma-separated token allowlist for multiple operators. |
| `GATEWAY_AUTH_REQUIRED` | `true` | Enables token authentication for `/api/*`. |
| `GATEWAY_RECONCILE_SECONDS` | `30` | Periodic reconcile interval in seconds. Route changes and manual Apply also reconcile. |
| `GATEWAY_CONFIG_FILE` | `/config/platform.example.json` | JSON platform config inside the container. Environment variables override it. |
| `GATEWAY_ROUTES_FILE` | `/data/platform/routes.json` | Writable route store for UI-created routes and Docker binds. |
| `GATEWAY_STATE_DIR` | `/data/platform` | Platform state directory. |
| `GATEWAY_CADDY_DATA_DIR` | `/data/caddy` | Caddy certificate/runtime data. Persist this in production. |

## Listeners And Management Access

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CONTROL_LISTEN` | `:8080` | Management API/UI listener inside the container. |
| `GATEWAY_MANAGEMENT_HOST` | empty | Optional public hostname for the management UI through Caddy on 80/443. It becomes a protected route and participates in Azure DNS/NSG reconciliation. |
| `GATEWAY_HTTP_LISTEN` | `:80` | Caddy HTTP listener inside the container. |
| `GATEWAY_HTTPS_LISTEN` | `:443` | Caddy HTTPS listener inside the container. |
| `GATEWAY_CADDY_ADMIN_ENDPOINT` | `http://127.0.0.1:2019` | Local Caddy Admin API endpoint. Keep it loopback-only. |

Default recommendation: leave `GATEWAY_MANAGEMENT_HOST` empty and access the UI through SSH tunnel, VPN, Bastion, Tailscale, or WireGuard.

## Certificate Policy

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CERTIFICATE_ISSUER` | `default` | Certificate issuer policy: `default`, `letsencrypt`, `zerossl`, or `custom`. |
| `GATEWAY_CERTIFICATE_EMAIL` | empty | ACME contact email. Recommended for production. |
| `GATEWAY_CERTIFICATE_STAGING` | `false` | Uses Let's Encrypt staging when issuer is `letsencrypt`. |
| `GATEWAY_CERTIFICATE_CA_DIRECTORY` | empty | Custom ACME CA directory URL. Required when issuer is `custom`. |

The Network page includes certificate controls backed by `GET/PUT /api/certificate` and `POST /api/certificate/refresh`. UI/API changes apply to the running gateway and trigger a Caddy reload; they do not write back to `.env`.

## Docker Discovery Labels

The `vm` profile imports these labels from running containers.

| Label | Required | Example | Purpose |
|---|---:|---|---|
| `caddy.enable` | Yes | `true` | Enables route import. |
| `caddy.host` | Yes | `webui.example.com` | Public host name. |
| `caddy.port` | No | `8080` | Upstream container port. |
| `caddy.health_path` | No | `/healthz` | Upstream HTTP health-check path. |
| `caddy.websocket` | No | `true` | Marks websocket/SSE-friendly workloads. |
| `exposure.mode` | No | `public` | One of `public`, `protected`, `internal`. |

Containers without `caddy.enable=true` are still shown in discovery. The UI can also bind a discovered container manually; manual bindings are saved as explicit routes and do not require labels.

## Docker Discovery Variables

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_DOCKER_ENABLED` | `true` in `vm`, disabled by default in `aci` | Enables local Docker discovery. |
| `GATEWAY_DOCKER_SOCKET` | `/var/run/docker.sock` | Docker socket path inside the gateway container. The sample mounts the host socket read-only. |
| `GATEWAY_DOCKER_ENDPOINT` | empty | Optional HTTP endpoint for a Docker socket proxy, for example `http://docker-socket-proxy:2375`. |

Use `make compose-up-proxy` when you want Docker discovery through a restricted Docker socket proxy instead of mounting `/var/run/docker.sock` into the gateway container directly.

## Azure DNS And NSG

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_AZURE_ENABLED` | `false` | Enables Azure reconciliation through `DefaultAzureCredential`. On Azure, this should use Managed Identity. |
| `GATEWAY_AZURE_MANAGE_DNS` | `true` | Creates, updates, and cleans up gateway-managed Azure DNS A records for public/protected routes. |
| `GATEWAY_AZURE_MANAGE_NSG` | `true` | Creates or deletes the gateway-managed VM NSG inbound rule for 80/443 in `vm` profile. Ignored for VM-style NSG management in `aci`. |
| `GATEWAY_AZURE_SUBSCRIPTION_ID` | empty | Azure subscription ID. `AZURE_SUBSCRIPTION_ID` is also accepted. |
| `GATEWAY_AZURE_RESOURCE_GROUP` | empty | Resource group containing the DNS zone and NSG. |
| `GATEWAY_AZURE_DNS_ZONE` | empty | Azure DNS zone name, for example `example.com`. |
| `GATEWAY_AZURE_NSG_NAME` | empty | Network Security Group name for VM profile 80/443 inbound rule reconciliation. |
| `GATEWAY_AZURE_NSG_PRIORITY` | `120` | Priority for the managed VM NSG allow rule. |
| `GATEWAY_AZURE_NSG_SOURCE_PREFIXES` | `*` | Comma-separated source CIDR prefixes for the managed VM NSG allow rule. |
| `GATEWAY_PUBLIC_IP_ADDRESS` | empty | Optional static public IP for DNS A records. If empty, the gateway discovers its public egress IP. |

Required Managed Identity roles:

- `DNS Zone Contributor` on the DNS zone or containing scope.
- `Network Contributor` on the NSG or containing scope when NSG reconciliation is enabled.

Cleanup behavior:

- DNS cleanup deletes only A records marked with `managed-by=ai-docker-farm-gateway` metadata.
- Deleting, disabling, or internalizing a route removes its managed DNS record on the next reconcile.
- The NSG rule is shared by all public/protected routes and is deleted only when no public/protected route remains, unless `GATEWAY_MANAGEMENT_HOST` is set.

## VM Deployment Notes

1. Assign a managed identity to the VM.
2. Grant the identity the Azure roles above.
3. Keep SSH/private management access through Tailscale, WireGuard, Bastion, VPN, or an equivalent private path.
4. Start with `make compose-up` or `make compose-up-proxy`.

The gateway only manages inbound NSG access for 80 and 443. It does not open 8080. The sample compose file binds the management UI to `127.0.0.1:8080` on the host.

## ACI Deployment Notes

ACI mode is an explicit-route gateway profile. It does not discover Docker containers from another VM.

Requirements:

- A published gateway image in a registry reachable by ACI.
- Managed identity assigned to the container group.
- DNS permissions if the gateway updates Azure DNS.
- Persistent storage for `/data/caddy` and `/data/platform` before production use.
- Network reachability from the container group to every upstream.

Important limits:

- ACI public IPs can change when the container group is recreated.
- Prefer CNAME records to the ACI DNS label, or let the gateway reconcile A records on startup.
- Private upstreams require VNet injection, Private Link, private DNS, or another explicit connectivity path.
- VM-style NSG rule management is disabled in ACI mode.
