# 运维指南

[English](operations.md)

本文合并本地启动、VM/ACI 部署说明、Docker 发现标签和运行时配置，减少分散重复的文档。

## 环境文件

`.env.example` 是示例文件。`start.sh` 不强制要求本地 `.env`；只有需要部署专用环境变量覆盖时才创建：

```sh
cp .env.example .env
```

`.env` 已被 Git 忽略，可保存部署专用值。令牌为空或仍是 `change-me` 时，`start.sh` 会在持久化目录生成强随机令牌并覆盖示例值。`/config/platform.example.json` 和 `/data/platform/routes.json` 等路径都是容器内部路径。

布尔值将 `1`、`true`、`yes`、`y`、`on` 视为真。其他非空值都视为假。

## 本地快速运行

```sh
./start.sh start
```

该命令只启动一个容器，发布 80/443，将管理 UI 绑定到 `127.0.0.1:8080`，并把 `~/docker_files/caddy-reverse-proxy` bind mount 到 `/data`。使用脚本显示的令牌登录。`stop` 保留该目录；`restore` 经过 `~/docker_files` 路径保护后删除它。

在默认 bridge 网络下，Docker 发现会优先使用 inspect 得到的容器 IP 作为上游地址。这样网关可以代理同一 bridge 网络内的容器，而不依赖 Docker DNS 名称。

## Docker 网络可达性

Caddy 只能代理网关容器在网络层可以访问到的上游。如果网关只加入一个新的自定义 Docker network，而工作负载容器仍然只在 Docker 默认 `bridge` 网络里，那么通常无法通过容器 DNS 名称访问这些容器；直接访问容器 IP 也可能被 Docker 的 bridge 隔离规则阻断。

可选模式如下：

- 本地预览时让网关和工作负载保持在同一个网络，例如默认 bridge。
- 将需要被路由的工作负载额外加入网关的自定义 network。
- 当需要路由多个隔离网络中的工作负载时，让网关同时加入多个 Docker network。
- 将工作负载发布到宿主机端口，然后显式路由到宿主机可达地址，例如已配置时的 `http://host.docker.internal:<port>`，或典型 Linux Docker bridge 下的 `http://172.17.0.1:<port>`。
- 当明确需要代理宿主机本地服务时，有意识地使用 host 网络模式。

推荐的 VM 实践：网关以普通容器运行，只把网关的端口发布到宿主机，并让网关加入它需要服务的工作负载网络。工作负载端口保持在 Docker 网络内部私有。这样既保留容器网络隔离，又能让 Caddy 路由多个应用网络。

不要把 `network_mode: host` 当成“让网关加入多个 Docker network”的方式。Host 网络模式会让容器进入宿主机网络命名空间，Docker 不会把这种模式和普通的多 network attach 混用。只有当上游是宿主机本地服务，或部署明确需要宿主机网络命名空间行为时，才使用 host 模式。

### 混合网络示例

假设 Portainer 运行在宿主机网络，网关运行在 `proxy-net`，其他程序仍在默认 `bridge` 网络。网关必须同时具备到这三类上游的可达路径：

- Portainer：如果 Portainer 监听宿主机端口 `9443`，网关路由写成 `https://host.docker.internal:9443`。Linux 上需要给网关容器加 `--add-host=host.docker.internal:host-gateway`；不使用该名称时，也可以写典型 bridge 网关地址 `https://172.17.0.1:9443`。
- `proxy-net` 内的服务：让服务和网关都加入 `proxy-net`，上游可写成 `http://service-name:port`。
- 默认 `bridge` 内的服务：推荐把该服务额外加入 `proxy-net`，然后也用 `http://service-name:port`。如果不能改网络，只能使用 Docker inspect 得到的 bridge IP，例如 `http://172.17.0.5:8080`，或把服务发布到宿主机端口后从网关访问宿主机地址。

示例命令：

```sh
docker network create proxy-net

docker run -d --name gateway \
	--network proxy-net \
	--add-host=host.docker.internal:host-gateway \
	-p 80:80 -p 443:443 -p 127.0.0.1:8080:8080 \
	-v "$HOME/docker_files/caddy-reverse-proxy:/data" \
	-v /var/run/docker.sock:/var/run/docker.sock:ro \
	--env-file .env \
	pczhao1210/caddy-reverse-proxy:latest

docker network connect proxy-net app-on-bridge
```

对应显式路由示例：

```json
{
	"routes": [
		{
			"host": "portainer.example.com",
			"exposure": "protected",
			"enabled": true,
			"https": true,
			"source": "explicit",
			"upstreams": [{ "name": "portainer", "url": "https://host.docker.internal:9443" }]
		},
		{
			"host": "app.example.com",
			"exposure": "public",
			"enabled": true,
			"https": true,
			"source": "explicit",
			"upstreams": [{ "name": "app", "url": "http://app-on-bridge:8080" }]
		}
	]
}
```

这个场景里，关键不是网关是否在 `host` 模式，而是每个 upstream 地址从网关容器内部是否能访问。建议优先把要代理的容器加入 `proxy-net`，把 host 上的服务通过 `host.docker.internal` 或宿主机 bridge 网关地址显式暴露给网关。

## Host 网络模式

Host 网络模式可以代理上游，但权衡不同：

- 网关可以不通过 `-p` 直接绑定宿主机 80/443。
- 可以通过 `127.0.0.1:<port>` 访问宿主机本地服务。
- Docker 自动发现仍然需要挂载 Docker socket 或使用 socket proxy。
- 发现到的容器 IP 通常仍可代理，但 host 网络会取消网关容器自身的网络隔离。
- 普通 Docker Engine 下主要适用于 Linux，不建议作为默认预览路径。

如需代理宿主机本地上游，建议创建显式路由，例如 `http://127.0.0.1:3000`。

## Make 目标

| Target | Purpose |
|---|---|
| `make test` | 在 Go 工具链容器中运行测试。 |
| `make docker-build` | 构建 `IMAGE`，默认 `pczhao1210/caddy-reverse-proxy:latest`。 |
| `make docker-push` | 检查 Docker daemon/登录状态后推送 `IMAGE`。 |
| `make docker-run` | 使用 `ENV_FILE` 在 Docker bridge 网络上本地运行镜像，默认 `.env`。 |
| `make compose-up` | 启动 VM 示例栈。 |
| `make compose-up-proxy` | 通过 Docker socket proxy 启动 VM 示例栈。 |
| `make compose-prod-up` | 启动生产 VM 栈并等待就绪。 |
| `make compose-prod-down` | 停止生产 VM 栈并保留数据卷。 |
| `make aci-build` | 编译 ACI + Standard Load Balancer Bicep 模板。 |
| `make aci-validate` | 使用参数在 Azure 中验证模板。 |
| `make aci-what-if` | 预览 Azure 部署变更。 |
| `make aci-deploy` | 部署 ACI + Standard Load Balancer 配置档。 |
| `make test-e2e` | 使用 VM 示例栈测试 Caddy 路由。 |
| `make compose-down` | 停止 VM 示例栈。 |

默认构建与推送目标为已发布的 Docker Hub 仓库：

```sh
make docker-build
make docker-push
```

发布到其他仓库或不可变 tag 时覆盖 `IMAGE`。

## 核心运行时变量

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_PROFILE` | `vm` | 部署配置档。Docker 主机或 VM 使用 `vm`；Azure Container Instances 显式路由使用 `aci`。 |
| `GATEWAY_ADMIN_TOKEN` | `change-me` | 管理 API 和受保护路由的管理员令牌。真实部署前必须替换。 |
| `GATEWAY_ADMIN_TOKENS` | empty | 可选的逗号分隔多令牌 allowlist。 |
| `GATEWAY_AUTH_REQUIRED` | `true` | 为 `/api/*` 启用令牌认证。 |
| `GATEWAY_RECONCILE_SECONDS` | `30` | 周期性协调间隔，单位秒。路由变更和手动 Apply 也会触发协调。 |
| `GATEWAY_CONFIG_FILE` | `/config/platform.example.json` | 容器内 JSON 平台配置文件。环境变量会覆盖它。 |
| `GATEWAY_ROUTES_FILE` | `/data/platform/routes.json` | UI 创建路由和 Docker bind 的可写路由存储。 |
| `GATEWAY_STATE_DIR` | `/data/platform` | 平台状态目录。 |
| `GATEWAY_CADDY_DATA_DIR` | `/data/caddy` | Caddy 证书和运行时数据。生产环境应持久化。 |
| `GATEWAY_CERTIFICATE_FILE` | `/data/platform/certificate.json` | Console 管理的证书设置，原子保存；POSIX 文件系统上创建权限为 `0600`。 |
| `GATEWAY_INTERNAL_SOURCE_RANGES` | RFC1918、回环、IPv6 私网/链路本地 | 允许访问 `internal` 路由的逗号分隔 IP/CIDR。 |

## 监听地址与管理访问

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CONTROL_LISTEN` | `:8080` | 容器内管理 API/UI 监听地址。 |
| `GATEWAY_MANAGEMENT_HOST` | empty | 可选公网管理 UI 主机名，通过 Caddy 在 80/443 暴露。它会成为受保护路由，并参与 Azure DNS/NSG 协调。 |
| `GATEWAY_HTTP_LISTEN` | `:80` | 容器内 Caddy HTTP 监听地址。 |
| `GATEWAY_HTTPS_LISTEN` | `:443` | 容器内 Caddy HTTPS 监听地址。 |
| `GATEWAY_CADDY_ADMIN_ENDPOINT` | `http://127.0.0.1:2019` | 本地 Caddy Admin API 地址。应保持只监听回环地址。 |

默认建议：保持 `GATEWAY_MANAGEMENT_HOST` 为空，通过 SSH 隧道、VPN、Bastion、Tailscale 或 WireGuard 访问 UI。

## 证书策略

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CERTIFICATE_ISSUER` | `letsencrypt` | 证书签发器策略：`letsencrypt`、`zerossl` 或 `custom`。历史兼容值 `default` 仍会映射到 `letsencrypt`。 |
| `GATEWAY_CERTIFICATE_EMAIL` | empty | ACME 联系邮箱。生产环境建议配置。 |
| `GATEWAY_CERTIFICATE_STAGING` | `false` | issuer 为 `letsencrypt` 时使用 Let's Encrypt staging。 |
| `GATEWAY_CERTIFICATE_CA_DIRECTORY` | empty | 自定义 ACME CA directory URL。issuer 为 `custom` 时必填。 |
| `GATEWAY_CERTIFICATE_SUBJECTS` | empty | 显式申请的逗号分隔域名，包括 `*.example.com`。 |
| `GATEWAY_CERTIFICATE_DNS_PROVIDER` | empty | DNS challenge 提供商，当前支持 `azure`。 |
| `GATEWAY_CERTIFICATE_AZURE_SUBSCRIPTION_ID` | empty | 权威 Azure DNS Zone 所在订阅。 |
| `GATEWAY_CERTIFICATE_AZURE_RESOURCE_GROUP` | empty | 权威 Azure DNS Zone 所在资源组。 |
| `GATEWAY_CERTIFICATE_AZURE_AUTHENTICATION` | `managedidentity` | `managedidentity` 或 `appregistration`。 |
| `GATEWAY_CERTIFICATE_AZURE_TENANT_ID` | empty | App Registration 认证所需租户 ID。 |
| `GATEWAY_CERTIFICATE_AZURE_CLIENT_ID` | empty | App Registration 认证所需客户端 ID。 |
| `GATEWAY_CERTIFICATE_AZURE_CLIENT_SECRET` | empty | App Registration 认证所需密钥，建议在 Console 输入以避免 shell history。 |

Network 页面提供证书控制，后端接口为 `GET/PUT /api/certificate` 和 `POST /api/certificate/refresh`。变更会原子保存到 `GATEWAY_CERTIFICATE_FILE`、立即应用，并在重启后恢复。客户端密钥会持久化，但 API 永不回传。

通配符域名必须使用 DNS-01。需要根域名时同时添加 `*.example.com` 与 `example.com`，选择 Azure DNS，并使用 Let's Encrypt 或自定义 ACME；Caddy 的 ZeroSSL issuer 不接受可配置 DNS challenge。Azure 身份需要权威 Zone 上的 `DNS Zone Contributor`。通配符证书域名与通配符路由 Host 相互独立；精确 Host 路由会优先于 `*.example.com`。

## Docker 发现标签

`vm` 配置档会从正在运行的容器读取以下标签。

| Label | Required | Example | Purpose |
|---|---:|---|---|
| `caddy.enable` | Yes | `true` | 启用路由导入。 |
| `caddy.host` | Yes | `webui.example.com` | 公网主机名。 |
| `caddy.port` | No | `8080` | 上游容器端口。 |
| `caddy.health_path` | No | `/healthz` | 上游 HTTP 健康检查路径。 |
| `caddy.websocket` | No | `true` | 标记 WebSocket/SSE 友好的工作负载。 |
| `exposure.mode` | No | `public` | `public`、`protected`、`internal` 三者之一。 |

没有 `caddy.enable=true` 的容器仍会显示在发现列表中。UI 也可以手动绑定已发现容器；手动绑定会保存为显式路由，不要求容器携带标签。

## Docker 发现变量

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_DOCKER_ENABLED` | `vm` 中默认 `true`，`aci` 中默认关闭 | 启用本地 Docker 发现。 |
| `GATEWAY_DOCKER_SOCKET` | `/var/run/docker.sock` | 网关容器内 Docker socket 路径。示例会只读挂载宿主机 socket。 |
| `GATEWAY_DOCKER_ENDPOINT` | empty | 可选 Docker socket proxy HTTP 地址，例如 `http://docker-socket-proxy:2375`。 |

如果希望通过受限 Docker socket proxy 发现容器，而不是直接挂载 `/var/run/docker.sock`，使用 `make compose-up-proxy`。

## Azure DNS 与 NSG

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_AZURE_ENABLED` | `false` | 通过 `DefaultAzureCredential` 启用 Azure 协调。在 Azure 上应使用托管身份。 |
| `GATEWAY_AZURE_MANAGE_DNS` | `true` | 为公开/受保护路由创建、更新、清理由网关管理的 Azure DNS A 记录。 |
| `GATEWAY_AZURE_MANAGE_NSG` | `true` | 在 `vm` 配置档中创建或删除由网关管理的 80/443 VM NSG 入站规则。ACI 中会忽略 VM 风格 NSG 管理。 |
| `GATEWAY_AZURE_SUBSCRIPTION_ID` | empty | Azure 订阅 ID。也接受 `AZURE_SUBSCRIPTION_ID`。 |
| `GATEWAY_AZURE_RESOURCE_GROUP` | empty | DNS Zone 和 NSG 所在资源组。 |
| `GATEWAY_AZURE_DNS_ZONE` | empty | 兼容旧配置的单个 Azure DNS Zone。 |
| `GATEWAY_AZURE_DNS_ZONES` | empty | `{name,resourceGroup}` JSON 数组；Host 使用最长匹配 Zone 后缀。 |
| `GATEWAY_AZURE_NSG_NAME` | empty | `vm` 配置档中用于 80/443 入站规则协调的 NSG 名称。 |
| `GATEWAY_AZURE_NSG_PRIORITY` | `120` | 托管 VM NSG allow 规则优先级。 |
| `GATEWAY_AZURE_NSG_SOURCE_PREFIXES` | `*` | 托管 VM NSG allow 规则来源 CIDR 前缀，多个值用逗号分隔。 |
| `GATEWAY_PUBLIC_IP_ADDRESS` | empty | DNS 管理存在公网路由时必填的入口 IPv4：VM 公网 IP 或 LB 入站 IP。不会再自动使用公网出口 IP。 |

托管身份所需角色：

- 在 DNS Zone 或其上级作用域授予 `DNS Zone Contributor`。
- 启用 NSG 协调时，在 NSG 或其上级作用域授予 `Network Contributor`。

清理行为：

- DNS 清理只会删除带有 `managed-by=ai-docker-farm-gateway` 元数据的 A 记录。
- 删除、禁用路由或将其改为 `internal` 后，下一次协调会移除对应托管 DNS 记录。
- upstream 健康失败只记录在路由状态中，不会删除 DNS；需要撤回 DNS 时应禁用或删除路由，避免探针造成 DNS 缓存抖动。
- NSG 规则由所有 `public/protected` 路由共享。只有不存在任何 `public/protected` 路由时才会删除，除非设置了 `GATEWAY_MANAGEMENT_HOST`。

## VM 部署说明

1. 为 VM 分配托管身份。
2. 授予上方 Azure 角色。
3. 通过 Tailscale、WireGuard、Bastion、VPN 或等价私有路径保持管理访问私有。
4. 使用 `IMAGE=<已发布镜像> DOCKER_NETWORKS=<network1,network2> ./start.sh start` 启动。

网关只管理 80 和 443 的入站 NSG 访问，不会打开 8080。`start.sh` 会把管理 UI 绑定到宿主机 `127.0.0.1:8080`。

## ACI 部署说明

ACI 模式是显式路由网关配置档。它不会从另一台 VM 自动发现 Docker 容器。

要求：

- 网关镜像已发布到 ACI 可访问的仓库。
- 容器组同时具有 system-assigned 与 user-assigned identity。
- 如果网关更新 Azure DNS，需要 DNS 权限。
- 生产环境前为 `/data/caddy` 和 `/data/platform` 提供持久化存储。
- 容器组到每个上游都必须网络可达。

支持的生产路径是 Standard Public Load Balancer 后的 VNet 私网 ACI。Bicep 模板创建专用 VNet、TCP 80/443 规则、`/readyz` 探针、NAT Gateway 出站、Azure Files 持久化，以及使用实际 ACI 私网 IP 的 backend。UAMI 用于 ACR 和控制面 Azure 操作，Caddy DNS-01 使用 system identity；传入 `dnsZones` 会给两者都授予 `DNS Zone Contributor`。上游位于其他私有 VNet 时，需要为专用 VNet 配置 peering。ACI 模式仍不启用 VM 风格的运行时 NSG 管理。详见 [deployment.zh-CN.md](deployment.zh-CN.md)。

## 运行时探针

- `/livez` 在 Go 控制面运行时返回成功。
- `/readyz` 仅在必需的 Caddy 子进程就绪时返回成功。
- `/healthz` 是 `/readyz` 的兼容别名。
