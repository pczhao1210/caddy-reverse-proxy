# Architecture

[简体中文](ARCHITECTURE.zh-CN.md)

## Target shape

```text
Internet
  -> platform gateway container :80/:443
    -> embedded Caddy runtime
    -> route table
    -> upstream Docker containers or explicit upstreams

Operators
  -> management UI/API :8080 or management host route
```

The platform is intentionally packaged as one gateway image. The control plane owns route state and renders Caddy JSON configuration. Caddy handles HTTP, HTTPS, automatic certificate management, websocket upgrades, and reverse proxy behavior.

## Runtime components

- Gateway process: starts the API server, reconcile loop, and Caddy runtime.
- Caddy runtime: listens on 80 and 443, receives generated config over a localhost-only admin endpoint.
- Route sources: Docker discovery in VM profile and explicit route configuration in ACI profile.
- Reconciler: merges route sources, renders desired gateway config, reloads Caddy, and records status.
- Health checker: probes configured upstream health paths during reconcile and records route-level readiness.
- Audit logger: appends route, bind, reconcile, DNS, and NSG change summaries to JSONL state.
- Management UI: static assets embedded in the Go binary and served by the API process.

## VM profile flow

```text
Docker Engine -> discovery -> route model -> Caddy config -> public HTTPS route
```

In VM profile, containers are discovered through labels. Workloads should not publish host ports directly; they should share a private Docker network with the gateway container.

## ACI profile flow

```text
routes config/API -> route model -> Caddy config -> public HTTPS route
```

ACI has no local Docker Engine. Automatic discovery is therefore out of scope for this profile unless a future remote agent or service registry is added.

## State and persistence

The platform stores operational state under `/data/platform`. Caddy certificate storage uses `/data/caddy`. These paths must be persistent in production. For VM deployments use Docker volumes. For ACI deployments use Azure Files or an equivalent persistent mount.
