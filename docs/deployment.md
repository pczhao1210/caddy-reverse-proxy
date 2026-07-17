# Production deployment

[简体中文](deployment.zh-CN.md)

The project uses a VM deployment. Caddy terminates TLS and routes requests by Host and Path to different services and ports.

| Mode | Public ingress | Routes | Best fit |
|---|---|---|---|
| Standalone Azure VM | VM Standard static public IP | Explicit routes to private backends | Gateway is isolated from backend hosts |
| Existing/co-located VM | VM Standard static public IP | Explicit routes and optional Docker labels | Gateway and services share one Docker host |

No Azure Load Balancer or NAT Gateway is required. The VM public IP handles inbound and outbound traffic, while Caddy owns TLS, certificates, and layer-seven routing.

## Common preparation

The public image is ready to use:

```sh
docker pull pczhao1210/caddy-reverse-proxy:latest
```

The published digest is `sha256:0e75a5bbeccb3b9354516e757bb805803a501cf6cca03988028e03030aa94c52`. For an immutable deployment, use `pczhao1210/caddy-reverse-proxy@sha256:0e75a5bbeccb3b9354516e757bb805803a501cf6cca03988028e03030aa94c52`.

Production requirements:

- Point every public hostname at the static ingress IP.
- Expose TCP 80 and 443 for HTTP redirects, ACME HTTP-01, and HTTPS.
- Persist all of `/data`, including routes, audit logs, and Caddy certificates.
- Replace `change-me` with a strong random management token.
- Never expose port 8080 or the Caddy Admin API at `127.0.0.1:2019` to the internet.

## Standalone Azure VM

### Prerequisites

- Azure Cloud Shell, or Bash 4+ with Azure CLI installed locally. The script is validated with Azure CLI 2.88.0.
- An Azure identity that can create resource groups, networking resources, managed identities, disks, and VMs.
- `ssh-keygen` when using an existing or newly generated SSH key. Password authentication does not require it.
- To use an SSH public key stored in Azure, the operator identity needs `Microsoft.Compute/sshPublicKeys/read` permission. Interactive selection also needs permission to list the visible SSH public key resources; specify both the resource name and resource group when list permission is unavailable.
- To use a Key Vault secret instead, the operator identity needs secret `get` permission. `list` permission enables interactive secret selection but is optional when the secret name is known. With Key Vault RBAC, `Key Vault Secrets User` scoped to the target vault or secret is sufficient; the script does not change role assignments.
- `curl` or `wget` for remote invocation. A local clone needs neither.

The script uses the active Azure CLI login. If none exists locally, it starts device-code login. When an existing VNet is in another resource group, the identity needs subnet join permission there; creating a subnet also requires subnet write permission.

### Run the interactive script

Choose one block in Azure Cloud Shell or a local Bash terminal. The temporary-file sequence prevents Bash from running partial or failed downloads and preserves the command's failure status. The first prompt selects either standalone Azure VM creation or local container-only deployment; choose the Azure VM option for this section.

```bash
deploy_script=$(mktemp) && curl -fsSL -o "$deploy_script" https://raw.githubusercontent.com/pczhao1210/caddy-reverse-proxy/main/deploy/vm/deploy.sh &&
    bash "$deploy_script"
status=$?
rm -f "${deploy_script:-}"
test "$status" -eq 0
```

```bash
deploy_script=$(mktemp) && wget -qO "$deploy_script" https://raw.githubusercontent.com/pczhao1210/caddy-reverse-proxy/main/deploy/vm/deploy.sh &&
    bash "$deploy_script"
status=$?
rm -f "${deploy_script:-}"
test "$status" -eq 0
```

From a cloned repository:

```sh
make azure-vm-deploy
```

The script interactively selects:

- Azure subscription and region. The region prompt defaults to Japan East (`japaneast`); set `LOCATION` before invocation to use another default.
- Deployment resource group and VM name.
- An existing or new VNet and a non-delegated existing or new subnet.
- An available VM size. `Standard_B1ms` (1 vCPU, 2 GiB) is recommended for low traffic; `Standard_B1s` (1 vCPU, 1 GiB) is a minimum-cost option only for very low traffic.
- OS disk SKU and size. The default Ubuntu Marketplace image is about 30 GiB, so 30 and 32 GiB Standard SSD disks both map to the 32-GiB E4 billing tier. Smaller E1-E3 tiers require an OS image that fits within those sizes.
- VM administrator authentication. The recommended default is an existing SSH public key from Azure, Key Vault, or a local `.pub` file. The script can instead generate a new local Ed25519 key pair or enable password authentication.
- The source CIDR permitted to use SSH. The detected operator public IP `/32` is offered when available.

For a new key pair, the script asks for a private-key path, refuses to overwrite either the private or public file, and waits until after the deployment confirmation prompt but before creating any Azure resource before running `ssh-keygen`. If key generation fails, no Azure resource is created. `ssh-keygen` prompts directly for an optional passphrase. Only the `.pub` file is sent to Azure; the private key remains local, is retained if Azure deployment later rolls back, and is included with `-i` in the printed SSH commands.

Password authentication is less secure than a passphrase-protected SSH key. The script passes only `--authentication-type password`; Azure CLI securely prompts for the password and its confirmation while creating the VM, so the script never reads, stores, or prints it. Azure CLI passwords must contain 12-123 characters and meet at least three of the lowercase, uppercase, digit, and special-character categories.

Set `VM_AUTHENTICATION_TYPE=ssh`, `generate`, or `password` to preselect a mode. For generated keys, `SSH_PRIVATE_KEY_FILE` changes the default private-key path.

The portal option **Use existing key stored in Azure** refers to an Azure SSH Public Key resource (`Microsoft.Compute/sshPublicKeys`), not Key Vault. The script lists these resources from the selected subscription and lets you select one. To resolve a uniquely named resource without the selection prompt, or to identify it fully without list permission:

```sh
SSH_PUBLIC_KEY_SOURCE=azure \
SSH_PUBLIC_KEY_RESOURCE_NAME=Azure_Personal_Tokyo \
SSH_PUBLIC_KEY_RESOURCE_GROUP=my-resource-group \
bash deploy/vm/deploy.sh
```

`SSH_PUBLIC_KEY_RESOURCE_GROUP` may be omitted when the name is unique among the visible resources in the selected subscription. Supplying both values reads that resource directly. The script validates the retrieved OpenSSH public key and never downloads or handles its private key.

The Key Vault secret value must be exactly one OpenSSH public key line, such as `ssh-ed25519 AAAA...`. A Key Vault **Certificate** containing X.509/PFX data and an SSH private key are not valid inputs. The value is read into a mode-`0600` temporary file, validated with `ssh-keygen`, passed to `az vm create`, and deleted when the script exits. The value is never printed. To skip vault and secret listing, specify them directly:

```sh
SSH_PUBLIC_KEY_SOURCE=keyvault \
SSH_KEY_VAULT_NAME=Tokyo-KV \
SSH_KEY_VAULT_SECRET_NAME=gateway-ssh-public-key \
bash deploy/vm/deploy.sh
```

Set `SSH_KEY_VAULT_SECRET_VERSION` to pin an optional version. If secret list permission is unavailable, the interactive flow asks for a known secret name and still attempts the required `get` operation.

After confirmation, it creates the resource group when needed, NSG, Standard static IPv4 address, NIC, system-assigned managed identity, Ubuntu 24.04 VM, and OS disk. Cloud-init installs Docker, pulls the published image, generates the admin token on the VM, persists `/data` under `/var/lib/caddy-reverse-proxy`, starts the gateway, and waits for both `/readyz` and Docker's `healthy` status through Azure Run Command.

The default Ubuntu 24.04 image resolves to x64, Hyper-V Generation 2 in Japan East. No separate architecture or generation argument is needed: before creating resources, the script resolves the image metadata and verifies that the selected VM size supports that architecture and generation.

Azure Run Command first waits for cloud-init to finish, then allows up to five minutes for gateway readiness. On timeout it prints the container state and the last 100 Docker log lines before rollback begins.

The default container image is pinned by digest so repeated deployments do not silently move to a different build. Set `IMAGE=<repository:tag-or-digest>` when invoking the script to use another image.

`Standard_B1s` has half the memory and half the baseline CPU performance of `Standard_B1ms`. It can run this prebuilt gateway at very low traffic, but leaves little headroom for Ubuntu, Docker, certificate operations, traffic spikes, or route growth. Monitor available memory, OOM events, CPU credits, and response latency; use `Standard_B1ms` when the gateway is internet-facing or expected to grow.

After a confirmed deployment starts, any error triggers rollback by default. The script removes and then checks the VM, OS disk, NIC, public IP, NSG, and any VNet or subnet created by that run. It retains the resource group and all pre-existing networks. Set `ROLLBACK_ON_ERROR=false` before invocation only when failed resources must remain available for diagnosis; they can continue to incur Azure charges.

The `/data` bind mount is durable across container replacement and VM restart, but it is stored on the VM OS disk. The VM is created with its OS disk delete option set to `Delete`; back up `/var/lib/caddy-reverse-proxy` or snapshot the disk before deleting the VM.

The initial NSG exposes TCP 80/443 to the internet and TCP 22 only to the selected source. Port 8080 remains bound to VM loopback. The standalone container uses host networking so configured listener ports can bind directly; a custom public port still requires managed NSG reconciliation or a manual NSG rule. The script does not create a Load Balancer or NAT Gateway.

Selecting an existing subnet does not alter its route table, subnet NSG, Azure Firewall or NVA path, or DNS settings. Those controls continue to apply in addition to the NIC NSG created by the script. Confirm that the selected subnet can reach Ubuntu package repositories, the container registry, and each private backend.

### Topology

```text
DNS -> VM Standard static public IP -> Caddy :80/:443/:custom-listener
                                      -> private-IP-or-DNS:port in the VNet

Operator -> SSH tunnel -> VM 127.0.0.1:8080
```

### Manual post-deployment steps

1. Point each public DNS A record to the public IP printed by the script.
2. Use the printed SSH command and tunnel, open `http://127.0.0.1:8080`, and configure explicit application routes and certificates.
3. Allow the gateway VM private IP or subnet to reach each backend port in backend NSGs and host firewalls.
4. For Azure DNS-01, grant the printed VM identity `DNS Zone Contributor` on the authoritative DNS zone. DNS record creation and this role assignment are intentionally manual.

The standalone VM has no access to a remote Docker socket. Configure backends by private IP or private DNS; do not expose Docker Engine remotely for discovery.

## Existing or co-located VM

The same interactive script supports a container-only mode for an existing Linux host. It requires Bash 4+ and first checks the Docker CLI, daemon, and socket permissions. When Docker is missing, it offers to install the distribution `docker.io` package only on Debian/Ubuntu hosts with `apt-get`; it can also offer to enable and start a stopped service. Install Docker Engine using the distribution's supported method first on other systems. This mode does not create or modify Azure resources, NSGs, public IPs, or DNS records. Run either download block above and select **Deploy only the gateway container on this machine**. From a cloned repository, use:

```sh
make container-deploy
```

Inside a checkout, this mode delegates to the existing `start.sh`. Otherwise it downloads `start.sh`, `.env.example`, and `config/platform.example.json` into `~/caddy-reverse-proxy` by default, then starts the container. The download uses a temporary staging directory and refuses to overwrite an existing incomplete or customized launcher directory. Override the launcher location with `LOCAL_INSTALL_DIR`; `IMAGE`, `CONTAINER_NAME`, `DATA_DIR`, `HTTP_PORT`, `HTTPS_PORT`, `MANAGEMENT_PORT`, and `DOCKER_NETWORKS` are passed through to `start.sh`. `DATA_DIR` must remain below `~/docker_files`; the mode preserves `/data` there and makes no infrastructure changes.

If the Docker daemon is running but the current user cannot access `/var/run/docker.sock`, the script explains that the `docker` group grants root-equivalent host access. After confirmation, it adds the user to that group and tries to continue this deployment in a temporary group session. If `sg` is unavailable or that session fails, the script stops and asks the user to sign out, sign back in, and rerun it. Later Docker commands in the original terminal may require the same sign-in refresh.

The default co-located launcher enables Docker discovery and mounts `/var/run/docker.sock`. Treat access to that socket as host-level privilege. Use the socket-proxy Compose deployment when direct socket access is not acceptable.

### Topology

```text
DNS -> VM Standard static public IP -> Caddy :80/:443
                                      -> service:port on private Docker networks
```

1. Assign a Standard static public IP to the VM and allow only TCP 80/443 through its NSG.
2. If workloads use custom Docker networks, create them before startup:

```sh
docker network create gateway-workloads
```

3. Start the single gateway container. The script generates the initial admin token and persists all of `/data` under `~/docker_files/caddy-reverse-proxy`:

```sh
DOCKER_NETWORKS=gateway-workloads \
./start.sh start
```

Open the Console through the SSH tunnel below and configure routes, ACME email, certificate subjects, and DNS challenge settings there. Use `.env` only for infrastructure integrations that must be present before Console access.

For optional runtime Azure A-record reconciliation, configure `.env` with the VM ingress IP and zones:

```dotenv
GATEWAY_AZURE_ENABLED=true
GATEWAY_AZURE_MANAGE_DNS=true
GATEWAY_AZURE_DNS_ZONES=[{"name":"example.com","resourceGroup":"dns-rg"},{"name":"example.net","resourceGroup":"dns-rg"}]
```

Grant the VM managed identity `DNS Zone Contributor` on every zone. `Network Contributor` is only required when runtime NSG reconciliation is enabled.

4. Reach the UI through an SSH tunnel:

```sh
ssh -L 8080:127.0.0.1:8080 <vm>
```

Open `http://127.0.0.1:8080`. Do not bind host port 8080 to `0.0.0.0`.

### VM verification

```sh
docker inspect caddy-reverse-proxy --format '{{.State.Status}} {{.State.Health.Status}}'
curl -fsS http://127.0.0.1:8080/livez
curl -fsS http://127.0.0.1:8080/readyz
curl --resolve app.example.com:443:<VM-public-IP> https://app.example.com/
```

## Migrating from Application Gateway

1. Lower existing DNS TTLs to 300 seconds and wait for the previous TTL to expire.
2. Add all routes to the new ingress without changing production DNS.
3. Use `curl --resolve` to verify HTTP, HTTPS, WebSocket, path routing, and large requests.
4. Point A records at the VM static public IP.
5. Monitor Caddy logs, `/readyz`, certificate issuance, and upstream errors.
6. Keep Application Gateway for at least one complete TTL window.
7. Remove the old listeners, certificates, and gateway only after traffic reaches zero.

Rollback by restoring the old Application Gateway A records. Avoid having both ingresses independently issue the same certificates while using inconsistent route state.

## Availability boundary

The VM deployment currently has one writable Caddy instance:

- Persistent `/data` prevents route and certificate loss but does not remove restart downtime.
- Multiple writable instances cannot safely share `routes.json`; active-active requires an external state store with concurrency control.
- HTTP/3 is disabled; the deployment script opens only TCP 80 and 443.