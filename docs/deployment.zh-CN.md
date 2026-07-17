# 生产部署

[English](deployment.md)

本项目统一使用 VM 部署，由 Caddy 终止 TLS，并按 Host 和 Path 将请求转发到不同服务与端口。

| 方式 | 公网入口 | 路由来源 | 适用场景 |
|---|---|---|---|
| 独立 Azure VM | VM Standard 静态公网 IP | 指向私网后端的显式路由 | 网关与后端主机隔离 |
| 已有/同机 VM | VM Standard 静态公网 IP | 显式路由和可选 Docker 标签 | 网关与服务共用 Docker 主机 |

不需要 Azure Load Balancer 或 NAT Gateway。VM 公网 IP 同时承担入站和出站，TLS、证书和七层路由全部由 Caddy 处理。

## 通用准备

公开镜像可直接使用：

```sh
docker pull pczhao1210/caddy-reverse-proxy:latest
```

当前发布 digest 为 `sha256:0e75a5bbeccb3b9354516e757bb805803a501cf6cca03988028e03030aa94c52`。需要不可变部署时，使用 `pczhao1210/caddy-reverse-proxy@sha256:0e75a5bbeccb3b9354516e757bb805803a501cf6cca03988028e03030aa94c52`。

生产环境需要：

- 为每个公网域名设置指向入口静态公网 IP 的 A 记录。
- 公网开放 TCP 80 和 443，供 HTTP 跳转、ACME HTTP-01 和 HTTPS 使用。
- 持久化整个 `/data`，其中包含路由、审计日志和 Caddy 证书。
- 使用高强度随机管理员令牌，不能保留 `change-me`。
- 保持 8080 和 Caddy Admin API `127.0.0.1:2019` 不对互联网开放。

## 独立 Azure VM

### 前置条件

- Azure Cloud Shell，或本地 Bash 4+ 与 Azure CLI 环境；脚本已使用 Azure CLI 2.88.0 验证。
- 当前 Azure 身份可以创建资源组、网络资源、托管身份、磁盘和 VM。
- 使用已有或新生成的 SSH key 时需要 `ssh-keygen`；密码认证不需要。
- 使用 Azure 中存储的 SSH 公钥时，部署操作者身份需要 `Microsoft.Compute/sshPublicKeys/read` 权限。交互选择还需要列出可见 SSH 公钥资源的权限；没有列表权限时可同时指定资源名和资源组。
- 改用 Key Vault secret 时，部署操作者身份需要 secret `get` 权限；已知 secret 名时不强制要求 `list`，具有 `list` 权限则可交互选择。使用 Key Vault RBAC 时，在目标 vault 或 secret 范围授予 `Key Vault Secrets User` 即可；脚本不会修改角色分配。
- 远程运行使用 `curl` 或 `wget`；已克隆仓库时不需要二者。

脚本使用当前 Azure CLI 登录。本地没有有效登录时会启动设备代码登录。已有 VNet 位于其他资源组时，当前身份需要该子网的 join 权限；如需新建子网，还需要 subnet write 权限。

### 运行交互脚本

在 Azure Cloud Shell 或本地 Bash 终端任选下面一段。临时文件流程会避免 Bash 执行下载失败或不完整的内容，并保留命令的失败状态。第一个提示可选择创建独立 Azure VM 或同机仅部署容器；本节应选择 Azure VM 模式。

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

已克隆仓库时：

```sh
make azure-vm-deploy
```

脚本会交互选择：

- Azure 订阅和区域。区域提示默认使用 Japan East（`japaneast`）；调用前设置 `LOCATION` 可修改默认区域。
- 部署资源组与 VM 名称。
- 已有或新建 VNet，以及其中未委派的已有或新建子网。
- 当前区域可用的 VM 规格。低流量默认推荐 `Standard_B1ms`（1 vCPU、2 GiB）；`Standard_B1s`（1 vCPU、1 GiB）只适合作为极低流量的最低成本选项。
- 系统盘 SKU 和容量。默认 Ubuntu Marketplace 镜像约 30 GiB，因此 30 与 32 GiB Standard SSD 都映射到 32-GiB E4 计费档；使用 E1-E3 小档位需要能够装入相应容量的 OS 镜像。
- VM 管理员认证方式。推荐的默认方式是使用 Azure、Key Vault 或本地 `.pub` 文件中的已有 SSH 公钥；也可以新建本地 Ed25519 密钥对或启用密码认证。
- 允许 SSH 的来源 CIDR；能检测到操作者公网 IP 时默认建议其 `/32`。

选择新建密钥对时，脚本会询问私钥路径，并拒绝覆盖已有的私钥或公钥文件。只有通过部署确认提示后才会运行 `ssh-keygen`，且此时尚未创建任何 Azure 资源；密钥生成失败不会留下 Azure 资源。passphrase 由 `ssh-keygen` 直接询问。只有 `.pub` 文件会发送到 Azure；私钥始终保留在本机，即使后续 Azure 部署回滚也不会删除。最终输出的 SSH 命令会自动包含对应的 `-i` 参数。

密码认证的安全性低于带 passphrase 的 SSH key。脚本只向 Azure CLI 传递 `--authentication-type password`，密码及确认值由 Azure CLI 在创建 VM 时隐藏输入；脚本不会读取、保存或打印密码。Azure CLI 要求密码长度为 12-123 个字符，并满足小写字母、大写字母、数字、特殊字符四类中的至少三类。

可通过 `VM_AUTHENTICATION_TYPE=ssh`、`generate` 或 `password` 预选认证模式；生成新密钥时可用 `SSH_PRIVATE_KEY_FILE` 修改默认私钥路径。

Portal 中的 **Use existing key stored in Azure** 指 Azure SSH Public Key 资源（`Microsoft.Compute/sshPublicKeys`），不是 Key Vault。脚本会列出所选订阅中的这些资源并供选择。要按唯一名称跳过选择，或者在没有列表权限时完整指定资源，可使用：

```sh
SSH_PUBLIC_KEY_SOURCE=azure \
SSH_PUBLIC_KEY_RESOURCE_NAME=Azure_Personal_Tokyo \
SSH_PUBLIC_KEY_RESOURCE_GROUP=my-resource-group \
bash deploy/vm/deploy.sh
```

当该名称在所选订阅的可见资源中唯一时，可以省略 `SSH_PUBLIC_KEY_RESOURCE_GROUP`。同时提供两个值时会直接读取目标资源。脚本只读取并校验其中的 OpenSSH 公钥，不会下载或处理对应私钥。

Key Vault secret 的值必须是且只能是一行 OpenSSH 公钥，例如 `ssh-ed25519 AAAA...`。Key Vault **Certificate** 中的 X.509/PFX 数据和 SSH 私钥均不能作为该输入。脚本把值读取到权限为 `0600` 的临时文件，使用 `ssh-keygen` 校验后传给 `az vm create`，退出时删除；不会打印公钥值。可直接指定 vault 与 secret，跳过列表选择：

```sh
SSH_PUBLIC_KEY_SOURCE=keyvault \
SSH_KEY_VAULT_NAME=Tokyo-KV \
SSH_KEY_VAULT_SECRET_NAME=gateway-ssh-public-key \
bash deploy/vm/deploy.sh
```

可选设置 `SSH_KEY_VAULT_SECRET_VERSION` 固定版本。没有 secret list 权限时，交互流程会要求输入已知 secret 名，然后继续尝试必需的 `get` 操作。

确认后，脚本按需创建资源组，并创建 NSG、Standard 静态 IPv4、NIC、系统分配托管身份、Ubuntu 24.04 VM 和系统盘。cloud-init 安装 Docker、拉取公开镜像、在 VM 内生成管理员令牌、把 `/data` 持久化到 `/var/lib/caddy-reverse-proxy`、启动网关，并通过 Azure Run Command 同时等待 `/readyz` 成功和 Docker 状态变为 `healthy`。

默认 Ubuntu 24.04 镜像在 Japan East 解析为 x64、Hyper-V Generation 2，无需另传架构或 generation 参数。创建资源前，脚本会读取镜像元数据，并验证所选 VM 规格支持对应架构和 generation。

Azure Run Command 会先等待 cloud-init 完成，再给网关最多 5 分钟进入就绪状态。超时时会输出容器状态和最近 100 行 Docker 日志，然后开始回滚。

默认容器镜像为 `pczhao1210/caddy-reverse-proxy:latest`，因此每次部署都会解析当前已发布版本。在 Azure 模式中调用脚本时，可设置 `IMAGE=<仓库:tag或digest>` 改用其他镜像。

`Standard_B1s` 的内存和 CPU 基线性能均只有 `Standard_B1ms` 的一半。极低流量时可以运行当前预构建网关，但给 Ubuntu、Docker、证书操作、流量突发和路由增长留下的余量很小。应监控可用内存、OOM、CPU credits 和响应延迟；网关面向公网或预期增长时使用 `Standard_B1ms`。

部署确认并开始后，任何错误默认触发回滚。脚本会删除并复查本次创建的 VM、系统盘、NIC、公网 IP、NSG，以及本次创建的 VNet 或子网；资源组和所有原有网络均会保留。只有需要保留失败资源进行诊断时才应在调用前设置 `ROLLBACK_ON_ERROR=false`，这些资源可能继续产生 Azure 费用。

`/data` bind mount 可跨容器重建和 VM 重启持久化，但实际位于 VM 系统盘。VM 的系统盘删除选项为 `Delete`；删除 VM 前应备份 `/var/lib/caddy-reverse-proxy` 或创建磁盘快照。

初始 NSG 向互联网开放 TCP 80/443，TCP 22 只允许所选来源；8080 仍只绑定 VM 回环地址。独立容器使用 host network，因此已配置的 Listener 端口可以直接绑定；公网自定义端口仍需托管 NSG 协调或手工 NSG 规则。脚本不会创建 Load Balancer 或 NAT Gateway。

选择已有子网不会修改其路由表、子网 NSG、Azure Firewall/NVA 路径或 DNS 设置。这些控制会与脚本创建的 NIC NSG 同时生效。应确认所选子网能够访问 Ubuntu 软件源、容器镜像仓库以及每个私有后端。

### 拓扑

```text
DNS -> VM Standard 静态公网 IP -> Caddy :80/:443/:自定义Listener
                                  -> VNet 中的私网 IP 或 DNS:端口

运维人员 -> SSH 隧道 -> VM 127.0.0.1:8080
```

### 部署后手工步骤

1. 把每个公网域名的 A 记录指向脚本输出的公网 IP。
2. 使用脚本输出的 SSH 和隧道命令，打开 `http://127.0.0.1:8080`，配置显式应用路由与证书。
3. 在后端 NSG 和主机防火墙中，允许网关 VM 私网 IP 或所在子网访问所需后端端口。
4. 使用 Azure DNS-01 时，在权威 DNS Zone 上给脚本输出的 VM 身份授予 `DNS Zone Contributor`。DNS 记录和该角色分配刻意保留为手工操作。

独立 VM 不访问远端 Docker socket。后端应使用私网 IP 或私有 DNS 配置，不要为了自动发现而远程暴露 Docker Engine。

## 已有或同机 VM

同一个交互脚本支持在现有 Linux 主机上只部署容器。该模式需要 Bash 4+，并会先检查 Docker CLI、daemon 和 socket 权限。Docker 未安装时，脚本只在带 `apt-get` 的 Debian/Ubuntu 主机上提供安装发行版 `docker.io` 软件包的选项；服务未运行时可经确认启用并启动。其他发行版需要先按其官方方式安装 Docker Engine。该模式不会创建或修改 Azure 资源、NSG、公网 IP 或 DNS 记录。运行上方任一下载命令，然后选择 **Deploy only the gateway container on this machine**。已克隆仓库时可直接运行：

```sh
make container-deploy
```

在仓库内运行时，该模式直接委托给现有 `start.sh`；否则默认把 `start.sh`、`.env.example` 和 `config/platform.example.json` 下载到 `~/caddy-reverse-proxy`，再启动容器。下载使用临时 staging 目录，并拒绝覆盖已有的残缺或自定义 launcher 目录。可通过 `LOCAL_INSTALL_DIR` 修改 launcher 位置；`IMAGE`、`CONTAINER_NAME`、`DATA_DIR`、`HTTP_PORT`、`HTTPS_PORT`、`MANAGEMENT_PORT` 和 `DOCKER_NETWORKS` 会原样传给 `start.sh`。launcher 会把镜像仓库归一化为 `latest` tag，并在每次启动前拉取。`DATA_DIR` 必须位于 `~/docker_files` 下；`/data` 会持久化到该目录，整个过程不修改基础设施。

Docker daemon 已运行但当前用户无权访问 `/var/run/docker.sock` 时，脚本会说明 `docker` 组具有等同 root 的主机权限，并在确认后把当前用户加入该组，再尝试通过临时组会话继续本次部署。如果系统没有 `sg` 或临时组会话失败，脚本会停止并要求注销、重新登录后再次运行。原终端中的后续 Docker 命令同样可能要在重新登录后才能使用。

默认同机 launcher 会启用 Docker 发现并挂载 `/var/run/docker.sock`。该 socket 的访问权限应视为主机级权限；不接受直接 socket 访问时，应改用 socket-proxy Compose 部署。

### 拓扑

```text
DNS -> VM Standard 静态公网 IP -> Caddy :80/:443
                                  -> Docker 私有网络中的 service:port
```

1. 给 VM 分配 Standard SKU 静态公网 IP，只在 NSG 中开放 TCP 80/443。
2. 如果工作负载使用自定义 Docker network，在启动前创建：

```sh
docker network create gateway-workloads
```

3. 启动单个网关容器。脚本会生成初始管理员令牌，并将整个 `/data` 持久化到 `~/docker_files/caddy-reverse-proxy`：

```sh
DOCKER_NETWORKS=gateway-workloads \
./start.sh start
```

通过下方 SSH 隧道打开 Console，再配置路由、ACME 邮箱、证书域名和 DNS challenge。只有必须在 Console 可访问前生效的基础设施集成才需要写入 `.env`。

可选的运行时 Azure A 记录协调需要在 `.env` 中配置 VM 入口 IP 与 Zone：

```dotenv
GATEWAY_AZURE_ENABLED=true
GATEWAY_AZURE_MANAGE_DNS=true
GATEWAY_AZURE_DNS_ZONES=[{"name":"example.com","resourceGroup":"dns-rg"},{"name":"example.net","resourceGroup":"dns-rg"}]
```

给 VM 托管身份在每个 Zone 上授予 `DNS Zone Contributor`。只有启用运行时 NSG 协调时才需要 `Network Contributor`。

4. 通过 SSH 隧道访问管理 UI：

```sh
ssh -L 8080:127.0.0.1:8080 <vm>
```

浏览器访问 `http://127.0.0.1:8080`。不要把宿主机 8080 绑定到 `0.0.0.0`。

### VM 验证

```sh
docker inspect caddy-reverse-proxy --format '{{.State.Status}} {{.State.Health.Status}}'
curl -fsS http://127.0.0.1:8080/livez
curl -fsS http://127.0.0.1:8080/readyz
curl --resolve app.example.com:443:<VM 公网 IP> https://app.example.com/
```

## 从 Application Gateway 迁移

1. 将现有 DNS TTL提前降低到 300 秒，并等待旧 TTL 过期。
2. 在新入口添加全部路由，但暂不改正式 DNS。
3. 使用 `curl --resolve` 分别验证 HTTP、HTTPS、WebSocket、Path路由和大请求。
4. 将 A 记录切换到 VM 静态公网 IP。
5. 观察 Caddy 日志、`/readyz`、证书签发和后端错误。
6. 保留旧 Application Gateway至少一个完整 TTL窗口。
7. 确认没有流量后再删除旧 listener、证书和 Application Gateway。

回滚时只需把 DNS A 记录改回旧 Application Gateway IP。不要同时让两个入口独立签发同一批证书并共用不一致的路由状态。

## 可用性边界

VM 部署当前只有一个可写 Caddy 实例：

- `/data` 持久化可以避免重建后丢失证书和路由，但不能消除重启窗口。
- 不支持让多个可写实例同时共享 `routes.json`。双活前需要把路由状态迁移到具备并发控制的外部存储。
- 暂不启用 HTTP/3；部署脚本只开放 TCP 80 和 443。