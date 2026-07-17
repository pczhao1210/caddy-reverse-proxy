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
- Caddy runtime: required child process that listens on the default 80/443 endpoints plus configured listener ports and receives generated config over a localhost-only admin endpoint. Startup or unexpected runtime failure terminates the container for orchestrator restart.
- Route sources: persisted listeners, backend pools, and routing rules, plus optional Docker discovery for co-located workloads.
- Reconciler: merges route sources, renders desired gateway config, reloads Caddy, and records status.
- Request security baseline: emits native Caddy matchers and handlers before each proxy for body-size limits, denied methods and paths, and direct-client IP/CIDR policy.
- Settings store: atomically persists Console-managed security, authentication, desired deployment, and Azure settings. Security and token updates propagate to the running API/Reconciler/Renderer; deployment and Azure client changes activate on restart.
- Health checker: probes configured upstream health paths during reconcile and records route-level readiness.
- Audit logger: appends route, bind, reconcile, DNS, and NSG change summaries to JSONL state.
- Management UI: static assets embedded in the Go binary and served by the API process.
- Runtime probes: `/livez` reports control-plane liveness; `/readyz` and compatibility endpoint `/healthz` require Caddy readiness.

## VM flow

```text
listeners + backend pools + routing rules -> route compiler -> runtime route model -> Caddy config
legacy Route API / Docker bind ------------^             ^
local Docker Engine -> optional discovery ----------------+
```

On a standalone gateway VM, Docker discovery is disabled and Console/API routes point to private VNet IPs or DNS names. On a co-located Docker host, discovery can read labels through a restricted socket proxy. Workloads should not publish host ports directly; they should share a private Docker network with the gateway container.

## State and persistence

The platform stores versioned routing resources, audit data, the Console certificate policy, and Console-managed settings under `/data/platform`. `settings.json` contains the admin token; it and other sensitive state are created with mode `0600` on POSIX filesystems. Legacy route files are atomically migrated to listeners, backend pools, and routing rules. Caddy certificate storage uses `/data/caddy`. All of `/data` must be persistent in production. The Azure VM deployment bind-mounts `/var/lib/caddy-reverse-proxy`; `start.sh` uses `~/docker_files/caddy-reverse-proxy` on an existing host.
