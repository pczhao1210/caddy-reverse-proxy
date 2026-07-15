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

Prefer managed identity for both control-plane Azure operations and Azure DNS-01. ACI intentionally separates the control-plane UAMI from Caddy's system identity. App Registration is available for environments without managed identity; its client secret is stored only in `/data/platform/certificate.json` and is never returned by the API. The file is created with mode `0600` on POSIX filesystems. ACI Azure Files uses CIFS, so POSIX mode bits are not the security boundary there; restrict storage-account access and network reachability, and protect all of `/data` as secret-bearing state. Never bake client secrets, service principal passwords, or local Azure tokens into the image.

## Network rules

The VM profile should manage only the minimum ingress rules required for 80 and 443. In ACI profile, Standard Load Balancer owns ingress and NAT Gateway owns egress; never publish the NAT egress address in public DNS. Port 8080 has no public Load Balancer rule. Restrict backend VM ports to the ACI subnet and allow the `AzureLoadBalancer` service tag to probe ACI port 8080.
