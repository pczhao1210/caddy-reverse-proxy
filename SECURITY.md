# Security

[简体中文](SECURITY.zh-CN.md)

## Public edge model

The platform container is a public edge gateway. Only ports 80 and 443 should be exposed to the Internet by default. The management port 8080 should be private, bound to localhost, protected by VPN, or exposed through a dedicated authenticated management host.

## Management API

The implementation protects management API calls with an admin token or comma-separated token allowlist. Production deployments should move to Entra ID/OIDC. Until then, set `GATEWAY_ADMIN_TOKEN` or `GATEWAY_ADMIN_TOKENS` to long random values and avoid exposing port 8080 publicly.

## Docker socket

Do not expose `/var/run/docker.sock` directly to untrusted containers or networks. The VM profile can use `make compose-up-proxy` to route discovery through a Docker socket proxy with limited inspection permissions.

## Caddy admin endpoint

Caddy's admin endpoint is bound to `127.0.0.1:2019` inside the platform container. It must never be published as a host port.

## Azure identity

The platform is designed for `DefaultAzureCredential` and managed identity. Do not store client secrets, service principal passwords, or local Azure tokens in the container image.

## Network rules

The VM profile should manage only the minimum ingress rules required for 80 and 443. ACI networking differs from VM NSG behavior; network changes should be capability-aware and should not claim to manage resources that are not attached to the container group.
