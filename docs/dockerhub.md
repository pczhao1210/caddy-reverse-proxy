# Caddy Reverse Proxy

Caddy reverse proxy with web UI, automatic HTTPS, Docker discovery and Azure DNS. AMD64/ARM64.

This is the **AI Docker Farm Edge Gateway** project image, not the official Caddy image. It bundles a Go control plane, a browser-based management console, and a managed Caddy runtime in one container. Use explicit upstream routes on any supported Docker host, with optional Docker discovery and Azure DNS/network integration.

[Source and English documentation](https://github.com/pczhao1210/caddy-reverse-proxy) | [Chinese documentation](https://github.com/pczhao1210/caddy-reverse-proxy/blob/main/README.zh-CN.md)

## Image and Platforms

```sh
docker pull pczhao1210/caddy-reverse-proxy:latest
```

`latest` supports **linux/amd64** and **linux/arm64**, including Ubuntu ARM64 hosts. Docker automatically selects the appropriate architecture; no `--platform` flag is needed for ordinary use. This is a Linux container, not a Windows container.

`latest` is mutable. For repeatable deployments, record the multi-platform index digest from `docker buildx imagetools inspect pczhao1210/caddy-reverse-proxy:latest` and pin that digest. Pulling a new image does not update a running container.

## Features

- Manage routes, upstreams, security settings, certificates, and recent logs through the web console.
- Stage route edits as drafts, then activate the batch with **Apply pending changes**.
- Automatic HTTPS and renewal through Caddy, including Azure DNS-01 wildcard certificates.
- Explicit routes to reachable private backends, plus optional label-based discovery for Docker workloads.
- Public, protected, and internal exposure modes, request body limits, denied methods/paths, and IP/CIDR rules.
- Optional Azure DNS and NSG reconciliation with instance ownership checks.
- Configuration ZIP export/import with review before apply; issued certificates, private keys, and secrets are not included.

## Quick Start: Explicit Routes

Install Docker Engine and make sure host TCP ports 80 and 443 are available. This example does not require a Docker socket or cloud credentials.

Create a private `gateway.env` file with these settings. Replace the placeholder with a long, randomly generated token before starting; it is the console/API credential, not a sample password to reuse.

```dotenv
GATEWAY_AUTH_REQUIRED=true
GATEWAY_ADMIN_TOKEN=REPLACE_WITH_A_LONG_RANDOM_TOKEN
GATEWAY_DOCKER_ENABLED=false
GATEWAY_AZURE_ENABLED=false
```

Start the container:

```sh
chmod 600 gateway.env
docker volume create caddy-reverse-proxy-data
docker run --detach \
  --name caddy-reverse-proxy \
  --restart unless-stopped \
  --security-opt no-new-privileges:true \
  --env-file gateway.env \
  --publish 80:80 \
  --publish 443:443 \
  --publish 127.0.0.1:8080:8080 \
  --volume caddy-reverse-proxy-data:/data \
  pczhao1210/caddy-reverse-proxy:latest
```

Open `http://127.0.0.1:8080` and sign in with your token. For a remote host, use an SSH tunnel from your workstation:

```sh
ssh -N -L 8080:127.0.0.1:8080 user@gateway-host
```

Then open the same local URL. Do not expose port 8080 directly to the Internet. Remote browser access should use a tunnel or a deliberately configured authenticated HTTPS management hostname.

Add a domain and an upstream in the console, review the draft, then select **Apply pending changes**. The upstream must be reachable from inside the gateway container: `127.0.0.1` refers to that container, not the Docker host. For container-name upstreams, attach the gateway and backend to the same user-defined Docker network. DNS and inbound firewall rules must direct the domain's traffic to the gateway.

## Ports and Persistent Data

| Container port | Purpose | Recommended exposure |
| --- | --- | --- |
| `80/tcp` | HTTP ingress, HTTPS redirects, HTTP-01 challenges | Public when needed |
| `443/tcp` | HTTPS ingress | Public when needed |
| `8080/tcp` | Management console, API, and health endpoints | Host loopback only |

Mount a persistent volume at **`/data`**:

- `/data/platform`: routes, settings, certificate policy, and control-plane state.
- `/data/caddy`: Caddy runtime storage, issued certificates, and private keys.

Keep this volume when recreating or upgrading the container. Protect backups as secret material. A configuration ZIP is not a backup of certificate storage or credentials. Do not share one writable state volume between independent gateway instances.

## HTTPS and Wildcard Certificates

For public HTTP-01 issuance, the domain must resolve to this gateway and the CA must be able to reach TCP port 80. Configure the issuer and account email in the console as needed.

Wildcard certificates require DNS-01. The bundled DNS provider is Azure DNS; configure the DNS zone permissions and supported identity or application credentials before requesting a wildcard. `*.example.com` covers one subdomain label, but does not cover `example.com` or `a.b.example.com`. Add the apex as a separate subject when required.

Caddy schedules renewal automatically while the gateway is running, storage persists, and the configured challenge remains usable. A displayed renewal estimate or a console refresh is not a forced renewal. Guarded certificate archival is not revocation or permanent deletion, and archived material still contains private keys.

## Optional Docker and Azure Integration

The quick start intentionally disables discovery and cloud reconciliation. Explicit routes still work.

For Docker discovery, prefer the repository's restricted socket-proxy deployment and a shared workload network. A Docker socket is a privileged interface; a read-only bind mount does not make all API calls read-only. Never expose a Docker socket proxy publicly.

For Azure integration, configure only the required identity permissions and DNS/NSG settings. Certificate DNS-01 credentials and optional DNS/NSG reconciliation are separate concerns. Some deployment and integration settings require a process restart; selecting them in the console does not provision host networks or mounts automatically.

See the [deployment guide](https://github.com/pczhao1210/caddy-reverse-proxy/blob/main/docs/deployment.md) and [production Compose example](https://github.com/pczhao1210/caddy-reverse-proxy/blob/main/deploy/vm/docker-compose.production.yml) before enabling these integrations.

## Health and Operations

```sh
curl --fail http://127.0.0.1:8080/livez
curl --fail http://127.0.0.1:8080/readyz
docker logs --tail 100 caddy-reverse-proxy
```

`/livez` checks process liveness; `/readyz` reports readiness. Management `/api/*` requests require authentication. Health endpoints alone do not prove that DNS, public certificate issuance, or every upstream works.

Before upgrading, back up persistent data, record the currently deployed image digest, pull the selected image, then recreate the container with the same volume, network attachments, ports, and environment. Test in staging first. Do not delete the data volume as part of an ordinary upgrade.

The repository launcher is an alternative to the manual command above; do not run both against the same container name or ports. In particular, `./start.sh restore` is destructive and removes persisted data.

## Security and Support

Keep management access private, use strong credentials, and review the documented dependency security exceptions before production use. Passing health checks or supporting both architectures is not a zero-vulnerability claim or a production performance guarantee.

- [Operations, certificate lifecycle, and dependency findings](https://github.com/pczhao1210/caddy-reverse-proxy/blob/main/docs/operations.md)
- [Security model](https://github.com/pczhao1210/caddy-reverse-proxy/blob/main/SECURITY.md)
- [Chinese deployment guide](https://github.com/pczhao1210/caddy-reverse-proxy/blob/main/docs/deployment.zh-CN.md)
- [Chinese operations guide](https://github.com/pczhao1210/caddy-reverse-proxy/blob/main/docs/operations.zh-CN.md)
- [Issue tracker](https://github.com/pczhao1210/caddy-reverse-proxy/issues)

When reporting a problem, include the image digest, host architecture, and sanitized logs. Do not attach admin tokens, Azure secrets, private keys, or unredacted state directories.
