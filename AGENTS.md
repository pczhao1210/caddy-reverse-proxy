# Repository Agent Guide

## Scope and Sources

This file applies to the entire repository. Keep changes focused on the requested task and preserve existing worktree changes. Do not commit, publish images, or deploy unless explicitly requested.

- Start with [README.md](README.md) for setup and [ARCHITECTURE.md](ARCHITECTURE.md) for component boundaries.
- Use [docs/operations.md](docs/operations.md) for publishing, certificate operations, dependency risks, and verification limits; see [SECURITY.md](SECURITY.md) for security constraints.
- Treat [backend/go.mod](backend/go.mod) and [backend/Dockerfile](backend/Dockerfile) as the current dependency baseline, not historical roadmap entries. Do not copy release digests or machine-specific paths into these instructions.

## Code Ownership

The shipped image runs a Go control plane, an embedded Alpine.js UI, and a managed Caddy subprocess. There is no separate frontend build pipeline.

- [backend/cmd/server](backend/cmd/server): startup and dependency wiring.
- [backend/internal/api](backend/internal/api): management API and configuration import/export; [backend/internal/model](backend/internal/model): shared models.
- [backend/internal/routes](backend/internal/routes) and [backend/internal/config](backend/internal/config): resource validation, compatibility, and persistence; [backend/internal/reconcile](backend/internal/reconcile): desired/applied coordination.
- [backend/internal/caddy](backend/internal/caddy): Caddy configuration rendering and runtime management; [backend/internal/certificate](backend/internal/certificate): certificate inventory, policy classification, and guarded archival.
- [backend/internal/docker](backend/internal/docker) and [backend/internal/azure](backend/internal/azure): discovery and cloud reconciliation.
- [backend/internal/auth](backend/internal/auth), [backend/internal/health](backend/internal/health), [backend/internal/logs](backend/internal/logs), and [backend/internal/audit](backend/internal/audit): cross-cutting services.
- [backend/internal/ui/static](backend/internal/ui/static): embedded HTML, JavaScript, CSS, and vendored Alpine.js; [deploy/vm](deploy/vm): deployment profiles; [scripts](scripts): integration and publishing helpers.

## Change Workflow

- Inspect the owning implementation and nearby tests before editing. Prefer existing helpers and test files over new abstractions or dependencies.
- Format touched Go files with `gofmt`. Preserve the existing Go, POSIX shell, and plain JavaScript conventions; do not introduce a frontend framework or package toolchain for incidental UI changes.
- Add focused regression coverage for changed behavior. Run the smallest relevant check first, then expand verification for shared contracts or user-facing workflows.
- Keep English and Chinese document counterparts aligned when changing documented behavior. Link to detailed documentation instead of duplicating it here.
- After a meaningful milestone, update the active plan with completed work, validation evidence, unrun checks, and the next smallest step. Use [docs/roadmap.md](docs/roadmap.md) and [docs/roadmap.zh-CN.md](docs/roadmap.zh-CN.md) when the change affects tracked roadmap work, not for every small edit.
- Report skipped tests and unresolved gates explicitly. Do not present historical test results as verification of the current change.

## Validation

Run commands from the repository root unless a block changes directory. Use the Go toolchain declared by the module/build files; an older host Go installation is not sufficient when automatic toolchain selection is disabled.

Go checks:

```sh
cd backend
go test ./...
go vet ./...
go test -race ./...
```

Race tests require a supported native toolchain with CGO support. For focused changes, first select the relevant package, such as `go test ./internal/caddy -run TestName`. The root `make test` runs ordinary Go tests in the pinned Go container; it does not replace race, vet, or real-Caddy checks.

Real Caddy tests in [backend/internal/caddy/config_test.go](backend/internal/caddy/config_test.go) skip when `CADDY_TEST_BIN` is unset. For renderer, routing, TLS, authentication forwarding, or Caddy dependency changes, set it to an absolute path to the matching custom Caddy binary, including the Azure DNS module and compatible CertMagic version:

```sh
cd backend
CADDY_TEST_BIN=/absolute/path/to/caddy go test -race ./...
```

JavaScript and shell checks, from the repository root:

```sh
node --test backend/internal/ui/app.test.cjs
node --test scripts/publish-multiarch.test.cjs
sh -n start.sh scripts/publish-multiarch.sh
git diff --check
```

Node tests use the built-in runner, without an npm install. Publishing tests use fake Docker/curl commands and require real `jq`; they do not publish images. For UI changes, also check the relevant browser workflow and narrow-screen layout. Documentation-only changes need link/command consistency and diff checks, not an image rebuild.

## Behavioral Invariants

- Keep desired and applied routing state distinct. The route store uses a v3 envelope containing v2 resource snapshots; configuration-bundle resources remain v2 and the bundle manifest has its own version. Preserve migration, rollback, and persistence behavior; these version numbers are not interchangeable.
- Preserve separate HTTP/TLS listeners, redirect ports, and HTTPS upstream Host behavior. Explicit user Host settings must continue to override the compatibility default.
- Keep `/api/*` authenticated even when static UI assets are public. Strip gateway credentials from ordinary protected upstream requests, but preserve them for the management loopback proxy. UI HTTP 401 clears authentication; HTTP 503 must not sign the user out.
- Caddy/CertMagic owns issuance and renewal. Inventory renewal estimates and policy refreshes are not forced renewal. Persist `/data/caddy`; control-plane state is persisted under `/data/platform`.
- Certificate archival is not revocation, permanent deletion, or renewal. Preserve eligibility checks, fingerprints, serving/replacement TLS checks, CertMagic storage locks, and atomic moves; archived private keys still require protection.
- Azure reconciliation must respect the stable instance owner. Do not take over unknown or foreign resources, discard DNS ETag checks, or lose instance-scoped NSG naming. Configuration exports must not clone the instance identity.
- The control plane and xcaddy build have separate Go dependency graphs. Changing `go.mod` alone does not update Caddy. Preserve pinned dependencies/base images, check both binaries when upgrading, and keep CertMagic storage-lock compatibility. Document outstanding scanner findings instead of claiming a clean release from passing tests alone.

## Operational Safety

- `./start.sh build` and `make docker-build` are local single-platform builds. `./start.sh push` and `make docker-push` build, verify, and publish both `linux/amd64` and `linux/arm64` through [scripts/publish-multiarch.sh](scripts/publish-multiarch.sh). The former forces `latest`; Make preserves an explicit `IMAGE=repository:tag`.
- Never overwrite the multi-platform `latest` with plain `docker push` of a single-platform local image. Preserve candidate validation and digest-based promotion. Serialize publishers to one tag: the target-change check is not registry compare-and-swap.
- Prepare privileged binfmt support or builders only with explicit authorization. Build on a development machine, not a resource-constrained gateway VM. Do not silently change the default builder.
- Remote candidate tags can share manifests with `latest`. If authorized to clean one up, delete only the named tag through the registry's tag-management API, not its shared manifest digest, then verify `latest` remains intact.
- Do not run `make test-e2e` or [scripts/e2e-caddy-routing.sh](scripts/e2e-caddy-routing.sh) against an existing stack: they tear down the example Compose stack and delete its volumes. Use an explicitly approved isolated environment.
- `./start.sh start` can replace the managed container; `restore` deletes persistent data. Deployment targets can change host services or create cloud resources. Obtain explicit approval before these operations, real cloud/ACME tests, or production certificate actions.
- Use temporary state, loopback-only ports, and disabled Docker/cloud integration for routine smoke tests. Clean up only resources created for that test; do not mount production state or a real Docker socket unnecessarily.
- Never print or commit credentials, tokens, private keys, or secret-bearing configuration bundles. Have the user enter secrets directly in the appropriate terminal, not in chat or tool arguments.