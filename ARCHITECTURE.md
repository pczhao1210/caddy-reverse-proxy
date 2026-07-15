# Architecture

[简体中文](ARCHITECTURE.zh-CN.md)

## Target shape

```text
Internet
  -> VM static public IP, or Standard Load Balancer for private ACI
  -> platform gateway container :80/:443
    -> embedded Caddy runtime
    -> route table
    -> upstream Docker containers or explicit upstreams

Operators
  -> management UI/API :8080 or management host route
```

The platform is intentionally packaged as one gateway image. The control plane owns route and certificate policy state and renders Caddy JSON configuration. Caddy is built with the Azure DNS provider and handles HTTP, HTTPS, HTTP-01/DNS-01 certificate management, websocket upgrades, and reverse proxy behavior.

## Runtime components

- Gateway process: starts the API server, reconcile loop, and Caddy runtime.
- Caddy runtime: required child process that listens on 80 and 443 and receives generated config over a localhost-only admin endpoint. Startup or unexpected runtime failure terminates the container for orchestrator restart.
- Route sources: Docker discovery in VM profile and explicit route configuration in ACI profile.
- Reconciler: merges route sources, renders desired gateway config, reloads Caddy, and records status.
- Health checker: probes configured upstream health paths during reconcile and records route-level readiness.
- Audit logger: appends route, bind, reconcile, DNS, and NSG change summaries to JSONL state.
- Management UI: static assets embedded in the Go binary and served by the API process.
- Runtime probes: `/livez` reports control-plane liveness; `/readyz` and compatibility endpoint `/healthz` require Caddy readiness.

## VM profile flow

```text
Docker Engine -> discovery -> route model -> Caddy config -> public HTTPS route
```

In VM profile, containers are discovered through labels. Workloads should not publish host ports directly; they should share a private Docker network with the gateway container.

## ACI profile flow

```text
Standard Load Balancer TCP 80/443 -> private VNet ACI
routes config/API -> route model -> Caddy config -> private VM upstreams
```

The Load Balancer does not terminate TLS or inspect hostnames. ACI has no local Docker Engine, so automatic discovery is out of scope unless a future remote agent or service registry is added. NAT Gateway provides ACI egress, while Azure Files persists all of `/data`. The UAMI serves ACR and control-plane Azure operations; the system identity is used by Caddy's Azure DNS-01 provider.

## State and persistence

The platform stores routes, audit data, and the Console certificate policy under `/data/platform`; the policy is created with mode `0600` on POSIX filesystems. Caddy certificate storage uses `/data/caddy`. All of `/data` must be persistent in production. `start.sh` bind-mounts `~/docker_files/caddy-reverse-proxy` for VM deployments; ACI uses Azure Files over CIFS.
