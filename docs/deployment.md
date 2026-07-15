# Production deployment

[简体中文](deployment.zh-CN.md)

The project provides two mutually exclusive production profiles. Both use the same image. Caddy terminates TLS and routes requests by Host and Path to different services and ports.

| Profile | Public ingress | Discovery | Best fit |
|---|---|---|---|
| VM | VM Standard static public IP | Docker labels and explicit routes | Gateway and services share one VM |
| ACI | Standard Public Load Balancer | Explicit routes | Gateway must be isolated from the backend VM |

Application Gateway is not part of the final data path. Standard Load Balancer only forwards TCP. Caddy owns TLS, certificates, and layer-seven routing, so adding domains does not add Load Balancer rules.

## Common preparation

Build and push an immutable image tag:

```sh
make test
make docker-build IMAGE=<registry>/caddy-reverse-proxy:<version>
make docker-push IMAGE=<registry>/caddy-reverse-proxy:<version>
```

Production requirements:

- Point every public hostname at the static ingress IP.
- Expose TCP 80 and 443 for HTTP redirects, ACME HTTP-01, and HTTPS.
- Persist all of `/data`, including routes, audit logs, and Caddy certificates.
- Replace `change-me` with a strong random management token.
- Never expose port 8080 or the Caddy Admin API at `127.0.0.1:2019` to the internet.

## VM profile

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
IMAGE=<registry>/caddy-reverse-proxy:<version> \
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

## ACI with Standard Load Balancer

### Topology

```text
DNS -> Standard Public Load Balancer
       TCP 80  -> VNet ACI:80
       TCP 443 -> VNet ACI:443
       HTTP readiness probe -> VNet ACI:8080/readyz

ACI/Caddy -> backend VM private-IP:different-ports
ACI egress -> NAT Gateway -> ACME, Azure APIs, and registry
```

The template creates:

- A Standard Load Balancer with a dedicated static ingress IP.
- TCP 80 and TCP 443 load-balancing rules.
- An HTTP `/readyz` probe.
- A dedicated VNet, delegated ACI subnet, NSG, NAT Gateway, and separate egress public IP.
- A private ACI container group exposing private ports 80, 443, and 8080.
- An Azure Files share mounted at `/data`.
- A UAMI for ACR/control-plane operations and a system identity for Caddy DNS-01.
- Optional `AcrPull` and `DNS Zone Contributor` for both identities on every configured zone.

### Prerequisites

- The default template creates a dedicated `10.42.0.0/24` VNet and `10.42.0.0/28` ACI subnet; override the prefixes if they overlap existing networks.
- Peer the dedicated VNet with the backend VNet when upstreams use private addresses. Restrict the backend NSG to the required ports from the ACI subnet.
- The deployment principal can create network, ACI, storage, and role assignment resources.
- Validate that the target subscription and region support ACI private-IP backends in Standard Load Balancer.

### Parameters and deployment

[![Deploy to Azure](https://aka.ms/deploytoazurebutton)](https://portal.azure.com/#create/Microsoft.Template/uri/https%3A%2F%2Fraw.githubusercontent.com%2Fpczhao1210%2Fcaddy-reverse-proxy%2Fmain%2Fdeploy%2Faci%2Fazuredeploy.json)

`image` and `adminToken` are the only required portal parameters. Publish the image first; `dnsZones` is optional, and supplying it creates the DNS role assignments required by both A-record reconciliation and wildcard certificate DNS-01. Certificate subjects and authentication settings are configured later in the Console.

For CLI deployment:

```sh
cp deploy/aci/main.example.bicepparam deploy/aci/main.bicepparam
export GATEWAY_ADMIN_TOKEN="$(openssl rand -base64 48)"
```

Replace the sample image name. Add optional ACR, VNet prefix, management host, or `dnsZones` overrides only when needed.

```sh
make aci-build
make aci-validate AZURE_RESOURCE_GROUP=<resource-group>
make aci-what-if AZURE_RESOURCE_GROUP=<resource-group>
make aci-deploy AZURE_RESOURCE_GROUP=<resource-group>
```

Deployment outputs include:

- `ingressPublicIPAddress`: target for all public DNS records.
- `natPublicIPAddress`: ACI egress address; never use it for ingress DNS.
- `containerPrivateIPAddress`: current ACI private address.
- Load Balancer, delegated subnet, both identity principal IDs, and storage identifiers.

The template writes the actual ACI private IP into the backend pool. Re-running `make aci-deploy` synchronizes the backend after an IaC-driven ACI replacement. Do not delete and recreate the container group outside the template. If that occurs, redeploy immediately and inspect backend health.

Port 8080 has no public Load Balancer rule and is reachable only from the VNet, VPN, or a jump host. `managementHost` is empty by default; only set it when its DNS record points to the ingress IP and public token-protected management is an accepted risk.

After reaching the Console, add both `*.example.com` and `example.com` as certificate subjects, select Azure DNS, and choose Managed Identity. The system identity output by the deployment needs `DNS Zone Contributor`; this is automatic for entries supplied through `dnsZones`. Exact Host routes take precedence over a `*.example.com` fallback route.

### ACI verification

```sh
az deployment group show -g <resource-group> -n <deployment-name> --query properties.outputs
az network lb show -g <resource-group> -n <name>-lb
az network lb show-backend-health -g <resource-group> -n <name>-lb
curl --resolve app.example.com:80:<ingress-ip> http://app.example.com/
curl --resolve app.example.com:443:<ingress-ip> https://app.example.com/
```

Certificate policy changes are saved in Azure Files and do not require an ACI redeployment.

## Migrating from Application Gateway

1. Lower existing DNS TTLs to 300 seconds and wait for the previous TTL to expire.
2. Add all routes to the new ingress without changing production DNS.
3. Use `curl --resolve` to verify HTTP, HTTPS, WebSocket, path routing, and large requests.
4. Point A records at the VM or Load Balancer ingress IP.
5. Monitor Caddy logs, `/readyz`, backend health, certificate issuance, and upstream errors.
6. Keep Application Gateway for at least one complete TTL window.
7. Remove the old listeners, certificates, and gateway only after traffic reaches zero.

Rollback by restoring the old Application Gateway A records. Avoid having both ingresses independently issue the same certificates while using inconsistent route state.

## Availability boundary

Both profiles currently have one writable Caddy instance:

- Standard Load Balancer provides stable ingress, but one ACI group is not highly available.
- Persistent `/data` prevents route and certificate loss but does not remove restart downtime.
- Multiple writable instances cannot safely share `routes.json`; active-active requires an external state store with concurrency control.
- HTTP/3 is disabled. Enabling it also requires a UDP 443 Load Balancer rule and ACI UDP port.