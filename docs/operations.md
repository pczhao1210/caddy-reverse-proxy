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
| `make docker-push` | Check Docker daemon/login state and push `IMAGE`. |
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

## Core Runtime Variables

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_PROFILE` | `vm` | Deployment profile. `vm` is the only supported value. |
| `GATEWAY_ADMIN_TOKEN` | `change-me` | Admin token for the management API and protected routes. Replace this before real deployment. |
| `GATEWAY_ADMIN_TOKENS` | empty | Optional comma-separated token allowlist for multiple operators. |
| `GATEWAY_AUTH_REQUIRED` | `true` | Enables token authentication for `/api/*`. |
| `GATEWAY_RECONCILE_SECONDS` | `30` | Periodic reconcile interval in seconds. Route changes and manual Apply also reconcile. |
| `GATEWAY_CONFIG_FILE` | `/config/platform.example.json` | JSON platform config inside the container. Environment variables override it. |
| `GATEWAY_ROUTES_FILE` | `/data/platform/routes.json` | Writable route store for UI-created routes and Docker binds. |
| `GATEWAY_STATE_DIR` | `/data/platform` | Platform state directory. |
| `GATEWAY_CADDY_DATA_DIR` | `/data/caddy` | Caddy certificate/runtime data. Persist this in production. |
| `GATEWAY_CERTIFICATE_FILE` | `/data/platform/certificate.json` | Console-managed certificate settings. Stored atomically and created with mode `0600` on POSIX filesystems. |
| `GATEWAY_INTERNAL_SOURCE_RANGES` | RFC1918, loopback, IPv6 private/link-local | Comma-separated IP/CIDR ranges permitted to use `internal` routes. |

## Listeners And Management Access

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CONTROL_LISTEN` | `:8080` | Management API/UI listener inside the container. |
| `GATEWAY_MANAGEMENT_HOST` | empty | Optional public hostname for the management UI through Caddy on 80/443. It becomes a protected route and participates in Azure DNS/NSG reconciliation. |
| `GATEWAY_HTTP_LISTEN` | `:80` | Caddy HTTP listener inside the container. |
| `GATEWAY_HTTPS_LISTEN` | `:443` | Caddy HTTPS listener inside the container. |
| `GATEWAY_CADDY_ADMIN_ENDPOINT` | `http://127.0.0.1:2019` | Local Caddy Admin API endpoint. Keep it loopback-only. |

Default recommendation: leave `GATEWAY_MANAGEMENT_HOST` empty and access the UI through SSH tunnel, VPN, Bastion, Tailscale, or WireGuard.

## Request Security Baseline

The gateway enables a lightweight request security baseline on every explicit, discovered, and public management route. It uses native Caddy matchers and handlers; it is not an SQL injection or XSS rules engine.

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

A positive route body limit replaces the global value. Omitted or `0` inherits it. Set `security.disabled` to `true` only for a route that must bypass the entire baseline. Route overrides are inactive while `GATEWAY_SECURITY_ENABLED=false`. The effective global policy is visible in the Console Network view and in `/api/status`.

## Certificate Policy

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CERTIFICATE_ISSUER` | `letsencrypt` | Certificate issuer policy: `letsencrypt`, `zerossl`, or `custom`. The legacy `default` alias still maps to `letsencrypt`. |
| `GATEWAY_CERTIFICATE_EMAIL` | empty | ACME contact email. Recommended for production. |
| `GATEWAY_CERTIFICATE_STAGING` | `false` | Uses Let's Encrypt staging when issuer is `letsencrypt`. |
| `GATEWAY_CERTIFICATE_CA_DIRECTORY` | empty | Custom ACME CA directory URL. Required when issuer is `custom`. |
| `GATEWAY_CERTIFICATE_SUBJECTS` | empty | Comma-separated names to request explicitly, including `*.example.com`. |
| `GATEWAY_CERTIFICATE_DNS_PROVIDER` | empty | DNS challenge provider. Currently `azure` is supported. |
| `GATEWAY_CERTIFICATE_AZURE_SUBSCRIPTION_ID` | empty | Subscription containing the authoritative Azure DNS zone. |
| `GATEWAY_CERTIFICATE_AZURE_RESOURCE_GROUP` | empty | Resource group containing the authoritative Azure DNS zone. |
| `GATEWAY_CERTIFICATE_AZURE_AUTHENTICATION` | `managedidentity` | `managedidentity` or `appregistration`. |
| `GATEWAY_CERTIFICATE_AZURE_TENANT_ID` | empty | Tenant ID required for App Registration authentication. |
| `GATEWAY_CERTIFICATE_AZURE_CLIENT_ID` | empty | Client ID required for App Registration authentication. |
| `GATEWAY_CERTIFICATE_AZURE_CLIENT_SECRET` | empty | Client secret required for App Registration authentication. Prefer Console entry to shell history. |

The Network page includes certificate controls backed by `GET/PUT /api/certificate` and `POST /api/certificate/refresh`. Changes are atomically saved to `GATEWAY_CERTIFICATE_FILE` and then reconciled immediately; the Console reports persistence and Caddy reload failures separately. Saved settings are restored after restart. Client secrets are persisted but never returned by the API. The refresh endpoint reloads the current TLS configuration through reconciliation; it does not force ACME certificate renewal.

Wildcard names require DNS-01. Add both `*.example.com` and `example.com` when the apex is needed, select Azure DNS, and use Let's Encrypt or a custom ACME issuer. Caddy's ZeroSSL issuer does not accept configurable DNS challenges. The Azure identity needs `DNS Zone Contributor` on the authoritative zone. Wildcard certificate subjects and wildcard route hosts are independent; exact route hosts are evaluated before `*.example.com` routes.

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

Containers without `caddy.enable=true` are still shown in discovery. The gateway container itself is excluded. The UI can also bind a discovered container manually; manual bindings require an explicit container port and upstream protocol, are saved as explicit routes, and do not require labels.

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
| `GATEWAY_AZURE_MANAGE_NSG` | `true` | Creates or deletes the gateway-managed VM NSG inbound rule for 80/443. |
| `GATEWAY_AZURE_SUBSCRIPTION_ID` | empty | Azure subscription ID. `AZURE_SUBSCRIPTION_ID` is also accepted. |
| `GATEWAY_AZURE_RESOURCE_GROUP` | empty | Resource group containing the DNS zone and NSG. |
| `GATEWAY_AZURE_DNS_ZONE` | empty | Legacy single Azure DNS zone name. |
| `GATEWAY_AZURE_DNS_ZONES` | empty | JSON array of `{name,resourceGroup}` entries. Hostnames use the longest matching zone suffix. |
| `GATEWAY_AZURE_NSG_NAME` | empty | Network Security Group name for VM profile 80/443 inbound rule reconciliation. |
| `GATEWAY_AZURE_NSG_PRIORITY` | `120` | Priority for the managed VM NSG allow rule. |
| `GATEWAY_AZURE_NSG_SOURCE_PREFIXES` | `*` | Comma-separated source CIDR prefixes for the managed VM NSG allow rule. |
| `GATEWAY_PUBLIC_IP_ADDRESS` | empty | Required VM public IPv4 address when DNS management has public routes. Egress IP discovery is intentionally not used. |

Required Managed Identity roles:

- `DNS Zone Contributor` on the DNS zone or containing scope.
- `Network Contributor` on the NSG or containing scope when NSG reconciliation is enabled.

Cleanup behavior:

- DNS cleanup deletes only A records marked with `managed-by=ai-docker-farm-gateway` metadata.
- Deleting, disabling, or internalizing a route removes its managed DNS record on the next reconcile.
- Upstream health failures are reported in route status but do not remove DNS records; disable or delete a route to withdraw DNS without creating probe-driven DNS cache churn.
- The NSG rule is shared by all public/protected routes and is deleted only when no public/protected route remains, unless `GATEWAY_MANAGEMENT_HOST` is set.

## VM Deployment Notes

For a new standalone Azure VM, run `make azure-vm-deploy`, or invoke `deploy/vm/deploy.sh` from Cloud Shell with the `curl`/`wget` commands in [deployment.md](deployment.md). The script interactively selects the region, VNet, subnet, VM size, and disk; installs Docker; disables local Docker discovery; starts the gateway; and prints its IPs, managed identity, admin token, SSH command, and management tunnel.

After the script completes, manually configure DNS A records, explicit routes, backend NSG/firewall access, certificate policy, and any Azure DNS role assignment. The script does not change those resources.

For an existing or co-located Docker VM:

1. Assign a managed identity when Azure DNS or NSG reconciliation is required.
2. Grant the identity the Azure roles above.
3. Keep SSH/private management access through Tailscale, WireGuard, Bastion, VPN, or an equivalent private path.
4. Start with `IMAGE=<published-image> DOCKER_NETWORKS=<network1,network2> ./start.sh start`.

The gateway only manages inbound NSG access for 80 and 443. It does not open 8080. `start.sh` binds the management UI to `127.0.0.1:8080` on the host.

## Runtime Probes

- `/livez` returns success while the Go control plane is running.
- `/readyz` returns success only while the required Caddy child process is ready.
- `/healthz` is a compatibility alias for `/readyz`.
