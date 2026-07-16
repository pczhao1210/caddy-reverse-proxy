# Architecture

[简体中文](ARCHITECTURE.zh-CN.md)

## Target shape

```text
Internet
  -> VM static public IP
  -> platform gateway container :80/:443
    -> embedded Caddy runtime
    -> route table
    -> explicit private upstreams or co-located Docker containers

Operators
  -> management UI/API :8080 or management host route
```

The platform is intentionally packaged as one gateway image. The control plane owns route and certificate policy state and renders Caddy JSON configuration. Caddy is built with the Azure DNS provider and handles HTTP, HTTPS, HTTP-01/DNS-01 certificate management, websocket upgrades, and reverse proxy behavior.

## Runtime components

- Gateway process: starts the API server, reconcile loop, and Caddy runtime.
- Caddy runtime: required child process that listens on 80 and 443 and receives generated config over a localhost-only admin endpoint. Startup or unexpected runtime failure terminates the container for orchestrator restart.
- Route sources: persisted explicit routes, plus optional Docker discovery for co-located workloads.
- Reconciler: merges route sources, renders desired gateway config, reloads Caddy, and records status.
- Request security baseline: emits native Caddy matchers and handlers before each proxy for body-size limits, denied methods and paths, and direct-client IP/CIDR policy.
- Health checker: probes configured upstream health paths during reconcile and records route-level readiness.
- Audit logger: appends route, bind, reconcile, DNS, and NSG change summaries to JSONL state.
- Management UI: static assets embedded in the Go binary and served by the API process.
- Runtime probes: `/livez` reports control-plane liveness; `/readyz` and compatibility endpoint `/healthz` require Caddy readiness.

## VM flow

```text
explicit routes ---------------------> route model -> Caddy config -> public HTTPS route
local Docker Engine -> optional discovery ----^
```

On a standalone gateway VM, Docker discovery is disabled and Console/API routes point to private VNet IPs or DNS names. On a co-located Docker host, discovery can read labels through a restricted socket proxy. Workloads should not publish host ports directly; they should share a private Docker network with the gateway container.

## State and persistence

The platform stores routes, audit data, and the Console certificate policy under `/data/platform`; the policy is created with mode `0600` on POSIX filesystems. Caddy certificate storage uses `/data/caddy`. All of `/data` must be persistent in production. The Azure VM deployment bind-mounts `/var/lib/caddy-reverse-proxy`; `start.sh` uses `~/docker_files/caddy-reverse-proxy` on an existing host.
