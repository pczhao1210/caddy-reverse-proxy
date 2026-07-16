# Security

[简体中文](SECURITY.zh-CN.md)

## Public edge model

The platform container is a public edge gateway. Only ports 80 and 443 should be exposed to the Internet by default. The management port 8080 should be private, bound to localhost, protected by VPN, or exposed through a dedicated authenticated management host.

## Management API

The implementation protects management API calls with an admin token or comma-separated token allowlist. Production deployments should move to Entra ID/OIDC. Until then, set `GATEWAY_ADMIN_TOKEN` or `GATEWAY_ADMIN_TOKENS` to long random values and avoid exposing port 8080 publicly.

Protected routes remove every enabled gateway credential header before proxying the request. If an upstream needs its own `Authorization` header, disable bearer-token gateway auth and use a dedicated gateway header.

## Docker socket

Do not expose `/var/run/docker.sock` directly to untrusted containers or networks. The VM profile can use `make compose-up-proxy` to route discovery through a Docker socket proxy with limited inspection permissions.

## Caddy admin endpoint

Caddy's admin endpoint is bound to `127.0.0.1:2019` inside the platform container. It must never be published as a host port.

## Azure identity

Prefer the VM system-assigned managed identity for both control-plane Azure operations and Azure DNS-01. App Registration is available for environments without managed identity; its client secret is stored only in `/data/platform/certificate.json` and is never returned by the API. The file is created with mode `0600` on POSIX filesystems. Protect the VM disk and all of `/data` as secret-bearing state. Never bake client secrets, service principal passwords, or local Azure tokens into the image.

## Network rules

The VM NSG should allow public TCP 80/443 and restrict TCP 22 to an operator CIDR or private management path. Port 8080 remains bound to VM loopback and must not have an inbound rule. Restrict backend ports to the gateway VM private IP or subnet. The deployment script does not change backend NSGs or host firewalls.
