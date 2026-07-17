# Security

[简体中文](SECURITY.zh-CN.md)

## Public edge model

The platform container is a public edge gateway. Only ports 80 and 443 should be exposed to the Internet by default. Expose a custom listener port only when a route requires it, and constrain its NSG source prefixes where possible. The management port 8080 should be private, bound to localhost, protected by VPN, or exposed through a dedicated authenticated management host.

## Management API

The implementation protects management API calls with an admin token or comma-separated token allowlist. Production deployments should move to Entra ID/OIDC. Until then, set `GATEWAY_ADMIN_TOKEN` or `GATEWAY_ADMIN_TOKENS` to long random values and avoid exposing port 8080 publicly.

The Console **Settings** page can rotate the admin token. Rotation takes effect immediately and invalidates the old token. Console-managed settings are stored in `/data/platform/settings.json` with mode `0600`; the file contains the token in the form required for authentication. Protect the persistent disk, backups, and all of `/data` as secret-bearing state.

Protected routes remove every enabled gateway credential header before proxying the request. If an upstream needs its own `Authorization` header, disable bearer-token gateway auth and use a dedicated gateway header.

The Console **Security** page controls request-body limits, denied methods and paths, direct-client CIDR policy, internal route ranges, and which token headers protected routes accept. Keep at least one protected-route token policy enabled. `remote_ip` is the peer connected directly to Caddy; do not treat forwarded headers as trusted client identity without a separately configured trusted-proxy design.

## Docker socket

Do not expose `/var/run/docker.sock` directly to untrusted containers or networks. The VM profile can use `make compose-up-proxy` to route discovery through a Docker socket proxy with limited inspection permissions.

## Caddy admin endpoint

Caddy's admin endpoint is bound to `127.0.0.1:2019` in the container network namespace, or VM loopback when the standalone Azure deployment uses host networking. It must never be publicly reachable.

## Azure identity

Prefer the VM system-assigned managed identity for both control-plane Azure operations and Azure DNS-01. The standalone VM script creates that identity, but role assignment remains manual. Grant `DNS Zone Contributor` on managed zones and `Network Contributor` on the target NSG when NSG reconciliation is enabled. App Registration is available for DNS-01 in environments without managed identity; its client secret is stored only in `/data/platform/certificate.json` and is never returned by the API. The file is created with mode `0600` on POSIX filesystems. Never bake client secrets, service principal passwords, or local Azure tokens into the image.

## Network rules

The VM NSG should allow public TCP 80/443 and only the custom listener ports that are intentionally public; restrict TCP 22 to an operator CIDR or private management path. Port 8080 remains bound to VM loopback and must not have an inbound rule. Restrict backend ports to the gateway VM private IP or subnet. The deployment script does not change backend NSGs or host firewalls.
