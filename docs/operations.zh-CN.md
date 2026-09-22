# 运维指南

[English](operations.md)

本文合并本地启动、独立/同机 VM 部署说明、Docker 发现标签和运行时配置，减少分散重复的文档。

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
| `make azure-vm-deploy` | 交互创建并启动独立 Azure VM 网关。 |
| `make test-e2e` | 使用 VM 示例栈测试 Caddy 路由。 |
| `make compose-down` | 停止 VM 示例栈。 |

默认构建与推送目标为已发布的 Docker Hub 仓库：

```sh
make docker-build
make docker-push
```

发布到其他仓库或不可变 tag 时覆盖 `IMAGE`。

## 依赖升级基线（2026-09-22）

| 组件 | 选定版本 |
|---|---|
| Go 构建/测试工具链 | 1.26.8 |
| Caddy / xcaddy / Azure DNS 插件 | 2.11.4 / 0.4.7 / 0.6.0 |
| 控制平面与 Caddy 的 CertMagic | 0.25.4 |
| Azure azcore / azidentity | 1.23.1 / 1.14.1 |
| `x/crypto` / `x/net` / `x/text` | 0.57.0 / 0.59.0 / 0.42.0 |
| 网关运行时 Alpine Linux / UI Alpine.js | 3.22.6 / 3.17.4 |
| Docker socket proxy / 示例 go-httpbin | v0.5.0 / 2.25.0 |

构建基础镜像与附属镜像均固定 tag 和 registry digest。控制平面的 `go.mod` 与 xcaddy 生成的模块依赖图相互独立，必须分别升级和扫描。Dockerfile 还固定了 Chi、compress、OpenTelemetry 和 gRPC 的安全修复版本，但没有锁定每个间接构建依赖。UI vendor 声明记录了 npm 来源及文件哈希。

Caddy 2.11 改变了 HTTPS 上游的默认 Host 行为。渲染器现在显式保留入口 Host 以维持兼容；用户显式设置的 Host 仍优先，TLS 验证保持启用。控制平面与 Caddy 的 CertMagic 版本保持一致，因为证书归档依赖其原生存储锁。后续升级应同步更新运行时版本断言，并重跑存储锁与真实 Caddy 测试。

本地验证通过：全量 Go race 与 vet，真实 Caddy HTTP/TLS/鉴权/Host/活动配置回归，Node 测试 6/6，以及 Alpine 3.17.4 浏览器 fixture 的登录、嵌套表单、证书筛选/归档确认、401/503 处理、桌面和 390px 布局。所有 Compose 文件渲染通过。隔离 socket proxy 使用模拟 Unix socket，允许 GET containers/info/networks，拒绝 POST create 和 GET secrets；示例 httpbin 健康与 GET 检查通过。未使用现有网关或真实 Docker socket。

安全结果是指定日期的快照，**不代表已通过生产安全发布门槛**：

- `govulncheck` 1.8.0 对 `./cmd/server` 未发现可达或已导入包漏洞，仍有仅模块级的 OpenPGP 告警。Trivy 0.74.0 对最终网关镜像的 OS 包扫描为零项（Alpine 3.22.6，共 18 个包）。
- 最终 Caddy 二进制仍报告 CEL 0.28.1 的 [GO-2026-6094](https://pkg.go.dev/vuln/GO-2026-6094)，涉及 `NativeTypes`/`ParseStructTag`。修复版 CEL 0.30.0 与 Caddy 2.11.4 的 interpreter API 编译不兼容。另有不再维护的 OpenPGP 包 [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932)，公告没有列出修复版本。二进制扫描不等于证明当前网关配置可触达漏洞；兼容修复与可达性评估仍待完成。
- 固定的 socket proxy 镜像仍含 OpenSSL `libcrypto3`/`libssl3` 3.5.7-r0：共 10 个不同 CVE，其中 1 个高危，分布于 20 条包级记录。Alpine 列出的修复版本为 3.5.8-r0。需要等待上游镜像更新，或明确批准维护重建镜像，不应直接替换为可变 nightly tag。
- 示例 httpbin 镜像没有 OS 扫描发现，但其 Go 1.26.5 二进制有 8 项标准库高危 CVE。该结果基于版本，并非调用链分析；相关修复从 Go 1.26.6 开始，需要上游重建。该示例不属于生产 Compose，不应对公网暴露。

下一最小步骤：解决或明确评估这些上游例外，重扫准确镜像后再进行授权的预发布演练。本轮未部署，未进行真实 Azure 集成、公网 ACME 续期、生产证书归档或会销毁示例栈的 E2E。演练前备份网关状态并保留旧镜像，在预发布环境验证 HTTPS Host、证书清单和续期后再安排生产升级。

## 核心运行时变量

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_PROFILE` | `vm` | 部署配置档；当前只支持 `vm`。 |
| `GATEWAY_DEPLOYMENT_MODE` | `container-socket` | UI/运行时部署标识：`container-socket` 或 `azure-vm`。启动脚本会显式设置。 |
| `GATEWAY_ADMIN_TOKEN` | `change-me` | 管理 API 和受保护路由的管理员令牌。真实部署前必须替换。 |
| `GATEWAY_ADMIN_TOKENS` | empty | 可选的逗号分隔多令牌 allowlist。 |
| `GATEWAY_AUTH_REQUIRED` | `true` | 为 `/api/*` 启用令牌认证。 |
| `GATEWAY_RECONCILE_SECONDS` | `30` | 周期性协调间隔，单位秒。存在草稿时后台仍协调已应用快照；只有手动 Apply 才激活草稿。 |
| `GATEWAY_CONFIG_FILE` | `/config/platform.example.json` | 容器内 JSON 平台配置文件。环境变量会覆盖它。 |
| `GATEWAY_ROUTES_FILE` | `/data/platform/routes.json` | v3 磁盘封装，保存 desired/applied 两份 v2 资源及版本号；旧路由与 Docker bind 仍使用兼容适配层。 |
| `GATEWAY_STATE_DIR` | `/data/platform` | 平台状态目录。 |
| `GATEWAY_CADDY_DATA_DIR` | `/data/caddy` | Caddy 证书和运行时数据。生产环境应持久化。 |
| `GATEWAY_CERTIFICATE_FILE` | `/data/platform/certificate.json` | Console 管理的证书设置，原子保存；POSIX 文件系统上创建权限为 `0600`。 |
| `GATEWAY_INTERNAL_SOURCE_RANGES` | RFC1918、回环、IPv6 私网/链路本地 | 允许访问 `internal` 路由的逗号分隔 IP/CIDR。 |

## 监听地址与管理访问

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CONTROL_LISTEN` | `:8080` | 容器内管理 API/UI 监听地址。 |
| `GATEWAY_MANAGEMENT_HOST` | empty | 可选公网管理 UI 主机名，通过 Caddy 在 80/443 暴露。登录静态资源公开，`/api/*` 由管理 API 鉴权；参与 Azure DNS/NSG 协调。 |
| `GATEWAY_HTTP_LISTEN` | `:80` | 容器内 Caddy HTTP 监听地址。 |
| `GATEWAY_HTTPS_LISTEN` | `:443` | 容器内 Caddy HTTPS 监听地址。 |
| `GATEWAY_CADDY_ADMIN_ENDPOINT` | `http://127.0.0.1:2019` | 本地 Caddy Admin API 地址。应保持只监听回环地址。 |

默认建议：保持 `GATEWAY_MANAGEMENT_HOST` 为空，通过 SSH 隧道、VPN、Bastion、Tailscale 或 WireGuard 访问 UI。

启用管理域名要求 `auth.required=true` 且至少配置一个管理员令牌，否则渲染拒绝。Caddy 只在内部生成的回环管理代理上保留凭据；普通 protected 业务路由仍在转发前移除网关凭据。Console 遇到普通 HTTP 503 会显示错误并保留令牌；401 才要求重新登录。

HTTP 与 HTTPS 使用独立 Caddy server，包括自定义端口；同一重叠端点的协议冲突会被拒绝。HTTPS 路由从默认 HTTP 入口重定向到实际 TLS 端口，且优先匹配显式 HTTP 路由。

## 路由 UI 字段语义

Console 使用 Listener、Backend Pool 和 Routing Rule 组合一条路由：

- 后端池通常每行只填写一个地址，不带协议和端口。支持 `10.0.0.5` 这样的私网 IP、网关可达的公网 IP，以及 `ex.example.com` 这样的 DNS 名称。DNS 名称由网关运行环境解析，每个地址都必须从该环境网络可达。HTTPS 后端的证书必须能校验所使用的地址或主机名。
- 路由规则中的后端协议和端口由池内所有目标共用。API 和迁移层为旧路由保留单目标端口覆盖能力，但新建 Console 配置应使用路由规则的共享端口。
- 后端地址决定 Caddy 连接到哪里，默认不会改变传入 HTTP `Host`。按前端域名路由的应用应把**后端 Host Header**留空；代理到要求自身域名的外部虚拟主机时填写 `ex.example.com`。
- 路由路径前缀留空表示匹配该 Listener 的所有路径。填写 `/api` 时匹配 `/api` 和 `/api/...`；Caddy 转发到上游时会保留此前缀。
- 健康检查路径可以留空。留空时使用 `health.defaultPath`，默认是 `/`；HTTP 2xx 和 3xx 视为健康。如果服务的 `/` 返回 404，应填写真实的就绪路径，例如 `/healthz`。探针失败只标记状态，不会停止代理或撤销 DNS。
- `public` 允许任何能访问 Listener 的客户端，但仍受全局安全基线约束；`protected` 还要求提供已启用的网关令牌 Header；`internal` 只允许直接客户端 IP 位于 `gateway.internalSourceRanges`，且不参与托管公网 DNS/NSG 协调。
- Caddy 的 HTTP 反向代理会自动处理 HTTP 或 HTTPS 上的 WebSocket Upgrade，不需要单独的路由规则 WebSocket 开关。

Listener、Backend Pool、Routing Rule、旧 Route API 和 Docker bind 的修改都会先持久化为草稿，点击**应用待处理更改**才激活。`/api/status` 根据 desired/applied 版本差异返回 `routingChangesPending`。手动 `POST /api/reconcile` 只确认 Caddy 实际接受的快照；期间保存的新草稿仍待应用。启动、周期协调、证书、安全策略和令牌刷新均使用已应用快照；存在草稿时，该快照的发现与健康检查仍继续运行。

v3 路由文件以一次原子替换保存 desired/applied 两份快照。普通草稿重启后仍保留但不会生效。升级前请备份状态：旧 v1/v2 文件无法识别历史未应用草稿，迁移时只能把当前文件内容作为已应用基线。旧程序不能读取 v3；降级前须恢复兼容备份。配置 ZIP 中的资源格式仍为 v2。

健康探测最多 8 个并发请求，每轮有 10 秒整体预算，同时保留配置的单请求超时。结果顺序不变，任一上游失败即标记该路由不健康。新配置加载不等待旧一轮探测或云操作完成；新一轮取消旧检查，仅当前代次可发布状态，Azure 操作另行串行执行。

## 运行日志

**日志**页面合并展示 `GET /api/logs` 返回的近期网关/Caddy 运行日志与 `GET /api/audit` 返回的持久化配置审计事件，支持按级别、来源筛选，并可搜索消息或结构化字段。运行日志使用有界内存环形缓冲区：最多 1000 条、序列化数据总量最多 8 MiB、每个输入行最多 64 KiB；达到任一上限时自动淘汰最旧记录，网关进程重启后全部清空。因此它适合近期排障，而不是长期留存。审计读取从文件末尾按 64 KiB 块逆向查找足够的有效记录，保持时间顺序并跳过损坏行；单行超过 1 MiB 会报错。历史审计不会自动轮转或删除，应纳入宿主机留存策略。API 只返回结构化消息，不提供任意文件读取能力。

来源筛选默认选择**路由活动**。该视图会显示 Caddy HTTP 访问事件，合并路由、Listener、Backend Pool、Routing Rule、配置导入及协调相关的持久化审计事件，并显示路由协调的 WARN/ERROR 运行日志。每条访问事件摘要会直接展示客户端 IP、请求方法与目标、命中的 Listener 和 Routing Rule、Backend Pool 与实际选中的 upstream、响应状态及耗时；展开**详情**可查看完整结构化请求字段，认证类 Header 默认由 Caddy 脱敏。正常完成时会保留可持久化的 `audit: reconcile.complete`，隐藏表示同一操作的运行时 `reconcile complete`，避免常见的重复记录。选择**全部来源**或某个精确来源仍可查看其他 Caddy 与控制平面日志。筛选只改变 Console 展示，不会删除记录；启用审计持久化后，路由与协调审计事件可跨重启保留，而 HTTP 访问事件及其他 `gateway/*`、`caddy/*` 运行日志会在重启时清空。

## Console 管理的设置

**安全**页面编辑全局请求基线、内部路由来源范围和受保护路由令牌 Header 策略，并在保存后立即重新加载 Caddy。**设置**页面编辑期望 Deployment、Azure 集成参数和管理员登录令牌。

Console 管理的设置会原子保存到 `/data/platform/settings.json`，POSIX 文件系统权限为 `0600`。对于 Console 管理的字段，它在 JSON 配置和环境变量之后加载。该文件以认证所需形式包含管理员令牌，必须作为敏感状态保护。如需让这些字段重新由文件/环境变量控制，应先停止网关再删除该文件。

安全策略会立即应用；新管理员令牌也立即生效并使旧令牌失效。Deployment 和 Azure 改动保存为下次进程启动配置，因为 Docker 发现、凭据和 Azure SDK 客户端都在启动时构造。Console 切换 Deployment 不会修改 Docker 网络模式、挂载、宿主机端口发布或 VM 基础设施；启动方式也必须满足所选拓扑。

### 配置压缩包迁移

**设置 > 配置文件**会导出按日期命名的 `caddyproxy_config_yyyymmdd.zip`。压缩包固定且仅包含四个 JSON 文件：

| 文件 | 内容 |
|---|---|
| `manifest.json` | 格式版本、导出时间、固定文件清单，以及明确的“不含秘密/证书材料”标记。 |
| `routes.json` | Listener、Backend Pool 和 Routing Rule。 |
| `settings.json` | Console 管理的 Deployment、Azure、安全及相关设置，已移除认证秘密。 |
| `certificate-policy.json` | 签发者、域名、续期与 DNS challenge 策略，已移除 Azure 客户端密钥。 |

压缩包绝不包含已签发证书、私钥、Caddy 数据、管理员令牌、附加认证 Header 值、Azure 客户端密钥、Azure 实例身份、运行日志或审计日志。导入保留目标实例现有秘密值与资源所有权。

导入只接受上述固定文件集；重复、嵌套、未知、符号链接、超限、JSON 格式错误或含未知字段的条目都会被拒绝。系统会先校验路由与设置，并完整渲染候选 Caddy 配置，再暂存任何内容。成功导入只替换可编辑的内存草稿，不写入路由、设置或证书文件，也不重载 Caddy。状态 API 会返回 `configurationImportPending`；草稿等待应用期间，系统会阻止有冲突的设置、安全和证书修改。

审阅导入的路由、设置与证书策略后，点击**应用待处理更改**。Caddy 接受候选配置后，网关逐文件原子替换并确认应用版本。返回的持久化错误会保留草稿供重试，并触发旧文件与旧 Caddy 配置恢复；恢复失败会明确报错。这不是跨文件的崩溃原子事务：导入前保留备份，进程在落盘中途崩溃后应恢复一组一致状态。证书申请在重载后异步开始，Apply 不等待签发。Deployment 和 Azure 设置仍需重启进程，相关客户端与拓扑才生效。

对应的认证 API 为：`GET /api/settings/configuration` 导出；以 `application/zip` 请求体调用 `POST /api/settings/configuration` 导入。压缩包上传上限为 8 MiB，解压后 JSON 总计上限为 4 MiB，单文件上限为 1 MiB。

## 请求安全基线

网关默认对所有显式路由、自动发现路由和公网管理路由启用轻量请求安全基线。它使用 Caddy 原生匹配器和处理器，不是 SQL 注入或 XSS 规则引擎。**安全**页面编辑的就是下列环境变量对应的全局策略。

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_SECURITY_ENABLED` | `true` | 全局启用请求安全基线。 |
| `GATEWAY_SECURITY_MAX_REQUEST_BODY_BYTES` | `10485760` | 请求体字节上限；`0` 关闭全局请求体限制。 |
| `GATEWAY_SECURITY_DENIED_METHODS` | `TRACE,CONNECT` | 以 405 拒绝的逗号分隔 HTTP 方法。 |
| `GATEWAY_SECURITY_DENIED_PATH_PREFIXES` | `/.git,/.env` | 以 403 拒绝的逗号分隔路径前缀。 |
| `GATEWAY_SECURITY_ALLOWED_CIDRS` | empty | 可选的直连客户端 IP/CIDR allowlist，范围外请求返回 403。 |
| `GATEWAY_SECURITY_BLOCKED_CIDRS` | empty | 以 403 拒绝的直连客户端 IP/CIDR blocklist。 |

`remote_ip` 判断直接连接 Caddy 的对端地址，不读取不可信的转发请求头。在网关前还有负载均衡器时，启用 CIDR 策略前必须确认它会保留源地址。blocklist 优先于 allowlist；全局与路由级 allowlist 会累积，因此同时配置时请求必须满足两者。

显式路由可以通过持久化 JSON 或路由 API 添加限制并覆盖请求体上限：

```json
{
	"security": {
		"maxRequestBodyBytes": 52428800,
		"additionalDeniedMethods": ["M-SEARCH"],
		"additionalDeniedPathPrefixes": ["/private"],
		"allowedCidrs": ["10.0.0.0/8"],
		"blockedCidrs": ["10.0.0.5"]
	}
}
```

路由级请求体上限为正数时替换全局值，省略或设为 `0` 时继承。只有确实需要绕过整套基线的路由才应将 `security.disabled` 设为 `true`。`GATEWAY_SECURITY_ENABLED=false` 时所有路由覆盖均不生效。Console 的“平台”页面和 `/api/status` 都会显示当前全局策略。

## 证书策略

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_CERTIFICATE_ISSUER` | `letsencrypt` | 证书签发器策略：`letsencrypt`、`zerossl` 或 `custom`。历史兼容值 `default` 仍会映射到 `letsencrypt`。 |
| `GATEWAY_CERTIFICATE_EMAIL` | empty | ACME 联系邮箱。生产环境建议配置。 |
| `GATEWAY_CERTIFICATE_STAGING` | `false` | issuer 为 `letsencrypt` 时使用 Let's Encrypt staging。 |
| `GATEWAY_CERTIFICATE_CA_DIRECTORY` | empty | 自定义 ACME CA directory URL。issuer 为 `custom` 时必填。 |
| `GATEWAY_CERTIFICATE_SUBJECTS` | empty | 显式申请的逗号分隔域名，包括 `*.example.com`。 |
| `GATEWAY_CERTIFICATE_RENEWAL_WINDOW_RATIO` | `0.3333333333333333` | 剩余多少比例的证书有效期时进入续期窗口。必须大于 `0` 且小于 `1`；`0.5` 会早于默认值开始续期。 |
| `GATEWAY_CERTIFICATE_DNS_PROVIDER` | empty | DNS challenge 提供商，当前支持 `azure`。 |
| `GATEWAY_CERTIFICATE_AZURE_SUBSCRIPTION_ID` | empty | 权威 Azure DNS Zone 所在订阅。 |
| `GATEWAY_CERTIFICATE_AZURE_RESOURCE_GROUP` | empty | 权威 Azure DNS Zone 所在资源组。 |
| `GATEWAY_CERTIFICATE_AZURE_AUTHENTICATION` | `managedidentity` | `managedidentity` 或 `appregistration`。 |
| `GATEWAY_CERTIFICATE_AZURE_TENANT_ID` | empty | App Registration 认证所需租户 ID。 |
| `GATEWAY_CERTIFICATE_AZURE_CLIENT_ID` | empty | App Registration 认证所需客户端 ID。 |
| `GATEWAY_CERTIFICATE_AZURE_CLIENT_SECRET` | empty | App Registration 认证所需密钥，建议在 Console 输入以避免 shell history。 |

“证书”页面的后端接口为 `GET/PUT /api/certificate` 和 `POST /api/certificate/refresh`。变更会先原子保存到 `GATEWAY_CERTIFICATE_FILE`，再立即执行协调；Console 会分别报告持久化结果与 Caddy 重新加载失败。已保存设置会在重启后恢复。客户端密钥会持久化，但 API 永不回传。启用 Azure DNS 协调后，证书中留空的订阅、DNS Zone 资源组和认证方式会根据匹配的已配置 DNS Zone 自动补全；显式填写的证书值不会被覆盖，NSG 资源组也绝不会被当作 DNS Zone 资源组。

Caddy 会在托管证书到期前自动续期。续期要求网关和 Caddy 运行时保持工作、`/data/caddy` 持久化，并且配置的 HTTP-01、TLS-ALPN-01 或 DNS-01 验证仍可使用。Console 的“启用提前续期”会把续期窗口比例设为 `0.5` 并重新应用策略；CA 的 ACME Renewal Information（ARI）及 Caddy/CertMagic 调度仍可能影响实际续期时间。“重新加载 TLS 配置”只应用当前策略，不会强制续期。

“证书”页面右侧扫描 `GATEWAY_CADDY_DATA_DIR/certificates/**/*.crt`，并根据 Caddy 已接受的 `/config/`，而非可编辑或已保存的策略，分类为“当前策略管理”“历史：通配符已覆盖”“历史：当前未引用”或“管理状态未知”。分类依据是域名匹配，不代表每份磁盘证书正在对外服务，也不证明仍使用其签发者。未知、手动加载、动态管理及不支持的运行配置会禁止归档。历史证书保留独立有效期详情，不再把进入预计续期窗口作为当前续期失败告警。窗口按所选策略的有效期比例估算，并非 Caddy 的 ARI 调度时间。“刷新状态”不重载 Caddy、不请求签发；私钥内容始终不会返回。

近期签发/续期事件从最近 1,000 条运行日志缓冲记录中提取，最新在前，最多 50 条，显示操作、结果、域名、宽泛的错误分类，以及 Caddy 明确报告的重试时间。为避免泄露凭据，不展示原始 ACME 错误及任意日志字段。这是近期观测，不是持久历史或未来执行承诺：重启和缓冲区淘汰会丢失事件，没有事件也不能证明续期正常。DNS、权限、连接与 CA 错误的详细原因应在受限运行日志中检查。

### 归档历史证书

“归档证书”调用 `POST /api/certificate/archive`，提交清单中的不透明 `id`、`fingerprintSha256` 与 `confirm: true`，不会吊销、强制续期或永久删除材料。服务端在配置提交锁内复核当前运行配置，并使用 CertMagic 的签发锁及存储清理锁。对受影响的具体 TLS 域名，本地握手必须返回另一份域名匹配且未过期的证书；无法握手或仍返回目标指纹会阻止归档。该检查不验证公网信任链或吊销状态。当前管理域名、未知配置、待应用的配置导入、符号链接、文件变化、非标准或不完整目录均会拒绝。按钮表示可进入服务端检查，并不保证归档成功。

仅允许将恰好包含同名 `.crt`、`.key`、`.json` 三件套的标准目录，原子移动到 `GATEWAY_CADDY_DATA_DIR/certificate-archive/<随机ID>/materials`。父目录权限为 `0700`，`0600` 清单记录原相对目录、指纹和归档时间。移动前后均核对文件身份及内容；不一致时尝试恢复，否则明确报告需要人工恢复。归档不会自动清空，仍含私钥，必须纳入受保护的备份。此功能面向单实例、本地文件系统部署：禁止与其他 Caddy 实例共享存储，也不要同时绕过控制面修改 Admin API 或文件。仅绑定非回环地址的自定义监听器，以及通配符路由的握手检查，需要人工审查。

恢复时先停止网关，备份完整数据目录，检查归档清单和指纹，再将 `materials` 移回同一 Caddy 数据根目录下记录的原相对目录。绝不覆盖已有目录或更新的证书，冲突须人工处理。保留严格的属主与权限，然后重启并检查 TLS 和日志。不要用归档/恢复触发续期；不保证整机断电持久性或多实例归档协调。

通配符域名必须使用 DNS-01。需要根域名时同时添加 `*.example.com` 与 `example.com`，选择 Azure DNS，并使用 Let's Encrypt 或自定义 ACME；Caddy 的 ZeroSSL issuer 不接受可配置 DNS challenge。Azure 身份需要权威 Zone 上的 `DNS Zone Contributor`。显式配置 `*.example.com` 后，`a.example.com` 这类具体 HTTPS 路由 Host 会写入 Caddy 的 `automatic_https.skip_certificates`，直接使用通配符证书，不再触发单独申请。如果显式域名列表同时包含通配符和具体域名，仍会管理两者，不会擅自去重。策略切换后旧文件可能保留，不能据此认定仍在重复申请或续期失败。通配符只覆盖一级标签，不覆盖 `example.com` 或 `a.b.example.com`。通配符证书域名与通配符路由 Host 仍相互独立；精确 Host 路由会优先于 `*.example.com`。

## Docker 发现标签

`vm` 配置档会从正在运行的容器读取以下标签。

| Label | Required | Example | Purpose |
|---|---:|---|---|
| `caddy.enable` | Yes | `true` | 启用路由导入。 |
| `caddy.host` | Yes | `webui.example.com` | 公网主机名。 |
| `caddy.port` | No | `8080` | 上游容器端口。 |
| `caddy.health_path` | No | `/healthz` | 上游 HTTP 健康检查路径。 |
| `caddy.websocket` | No | `true` | 兼容旧配置的提示字段。Caddy 会自动代理 WebSocket Upgrade，生成的代理配置不依赖该标志。 |
| `exposure.mode` | No | `public` | `public`、`protected`、`internal` 三者之一。 |

没有 `caddy.enable=true` 的容器仍会显示在发现列表中，但网关容器自身会被排除。UI 也可以手动绑定已发现容器；手动绑定必须明确选择容器端口与上游协议，会保存为显式路由，且不要求容器携带标签。

自动路由只从网关与工作负载的共享网络选择地址，按网络名称确定性排序。Compose 服务名只作为共享自定义网络的回退，不用于默认 bridge。无法通过容器 hostname 识别网关或没有可达共享网络地址时，会跳过自动路由并返回警告。host-network 上游须使用显式路由。

## Docker 发现变量

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_DOCKER_ENABLED` | `true` | 启用本地 Docker 发现。独立 Azure VM 脚本会显式设为 `false`；显式路由不受影响。 |
| `GATEWAY_DOCKER_SOCKET` | `/var/run/docker.sock` | 网关容器内 Docker socket 路径。示例会只读挂载宿主机 socket。 |
| `GATEWAY_DOCKER_ENDPOINT` | empty | 可选 Docker socket proxy HTTP 地址，例如 `http://docker-socket-proxy:2375`。 |

如果希望通过受限 Docker socket proxy 发现容器，而不是直接挂载 `/var/run/docker.sock`，使用 `make compose-up-proxy`。

## Azure DNS 与 NSG

| Variable | Default | Meaning |
|---|---|---|
| `GATEWAY_AZURE_ENABLED` | `false` | 通过 `DefaultAzureCredential` 启用 Azure 协调。在 Azure 上应使用托管身份。 |
| `GATEWAY_AZURE_MANAGE_DNS` | `true` | 为公开/受保护路由创建、更新、清理由网关管理的 Azure DNS A 记录。 |
| `GATEWAY_AZURE_MANAGE_NSG` | `true` | 为公网 Listener 端口创建、更新或删除由网关管理的 VM NSG 规则；默认入口和 ACME 会继续保留 80/443。 |
| `GATEWAY_AZURE_SUBSCRIPTION_ID` | empty | Azure 订阅 ID。也接受 `AZURE_SUBSCRIPTION_ID`。 |
| `GATEWAY_AZURE_RESOURCE_GROUP` | empty | DNS Zone 的默认资源组。 |
| `GATEWAY_AZURE_DNS_ZONE` | empty | 兼容旧配置的单个 Azure DNS Zone。 |
| `GATEWAY_AZURE_DNS_ZONES` | empty | `{name,resourceGroup}` JSON 数组；Host 使用最长匹配 Zone 后缀。 |
| `GATEWAY_AZURE_NSG_RESOURCE_GROUP` | `GATEWAY_AZURE_RESOURCE_GROUP` | Network Security Group 所在资源组。 |
| `GATEWAY_AZURE_NSG_NAME` | empty | `vm` 配置档中用于公网 Listener 入站规则协调的 NSG 名称。 |
| `GATEWAY_AZURE_NSG_PRIORITY` | `120` | 托管 VM NSG allow 规则优先级。 |
| `GATEWAY_AZURE_NSG_SOURCE_PREFIXES` | `*` | 托管 VM NSG allow 规则来源 CIDR 前缀，多个值用逗号分隔。 |
| `GATEWAY_PUBLIC_IP_ADDRESS` | empty | DNS 管理存在公网路由时必填的 VM 公网 IPv4。不会再自动使用公网出口 IP。 |

托管身份所需角色：

- 在 DNS Zone 或其上级作用域授予 `DNS Zone Contributor`。
- 启用 NSG 协调时，在 NSG 或其上级作用域授予 `Network Contributor`。

独立 VM 部署通过 `az vm create --assign-identity` 创建系统分配托管身份；角色授权仍由运维人员显式完成。网关使用 `DefaultAzureCredential`，因此 Console 不需要配置托管身份密钥。“配置完整”只校验必需 ID 和名称是否存在。

在 **设置 > Azure 集成** 中，**检查权限**会针对每个已配置 DNS Zone 和目标 NSG 查询 `Microsoft.Authorization/permissions`。它只读验证 DNS A 记录及 NSG Security Rule 协调所需的有效 `read`、`write`、`delete` 操作，不会修改资源。检查使用当前运行进程中由 `DefaultAzureCredential` 选中的身份：在 VM 上通常是托管身份，本地运行时可能是 Azure CLI 或其他开发者凭据。新增或修改的角色分配可能需要数分钟传播；传播完成后再重试检查。检查成功表示 ARM 有效操作满足要求，Reconcile 仍是端到端运行验证。

Reconcile 会写入期望 A 记录、列出网关托管 A 记录用于清理、等待 NSG 规则操作完成，并在平台页报告数量、警告或 Azure API 错误。它不是针对无关 DNS 记录或 NSG 规则的通用漂移监控器。

清理行为：

- DNS 清理只删除同时具有 `managed-by=ai-docker-farm-gateway` 和当前 `gateway-instance-id` 的 A 记录。更新、删除携带读取时的 ETag；新建使用 `If-None-Match: *`，412 会报错而不会降级为无条件覆盖。本实例内容未变的记录不重复写入。
- 删除、禁用路由或将其改为 `internal` 后，下一次协调会移除对应托管 DNS 记录。
- upstream 健康失败只记录在路由状态中，不会删除 DNS；需要撤回 DNS 时应禁用或删除路由，避免探针造成 DNS 缓存抖动。
- NSG 规则由本实例的 `public/protected` 路由共享；名称带实例 ID 后缀，描述必须标识同一归属。端口集合变化时更新，内容未变不重复写入；无公网路由且未设管理域名时删除。优先级冲突由 Azure 返回，不覆盖其他规则。NSG SDK 不提供 DNS 式条件写入保证。

### 所有权与升级

启用 Azure 的网关在 `GATEWAY_STATE_DIR/azure-instance-id` 创建稳定随机身份，文件权限为 `0600`。恢复同一逻辑网关时须保留；克隆独立网关时，在启动克隆前仅删除克隆的身份文件以生成新归属。不要让独立实例使用相同身份或共享状态目录。配置 ZIP 不转移资源所有权。

未知归属、其他实例的 DNS 记录不会被自动接管。只有通用 `managed-by` 标记的旧记录会保留；期望域名与其冲突时返回所有权错误。如确需迁移，先停止旧写入者、备份 DNS 和本地状态，确认每条记录属于此网关，再通过 Azure 管理流程显式添加本实例的 `gateway-instance-id` 元数据。接管后不在期望路由集合中的记录可能被清理，迁移前须审阅完整集合。

旧固定 NSG 规则 `Allow-AIDockerFarm-Gateway-HTTPHTTPS` 会保留。先为新实例规则选择空闲优先级，验证入站连通性，再确认无其他网关依赖后手工删除旧规则。直接复用旧规则优先级可能产生 Azure 冲突。移除旧访问规则是显式运维操作，不会自动进行。

## VM 部署说明

新建独立 Azure VM 时，运行 `make azure-vm-deploy`，或使用 [deployment.zh-CN.md](deployment.zh-CN.md) 中的 `curl`/`wget` 命令从 Cloud Shell 调用 `deploy/vm/deploy.sh`。脚本会交互选择区域、VNet、子网、VM 规格和磁盘，安装 Docker，关闭本地 Docker 发现，使用 host network 启动网关，将管理端绑定到 `127.0.0.1:8080`，并输出 IP、托管身份、管理员令牌、SSH 命令与管理隧道。

脚本完成后，手工配置 DNS A 记录、显式路由、后端 NSG/防火墙访问、证书策略及 Azure DNS 角色分配。脚本不会修改这些资源。

已有或同机 Docker VM：

1. 需要 Azure DNS 或 NSG 协调时，为 VM 分配托管身份。
2. 授予上方 Azure 角色。
3. 通过 Tailscale、WireGuard、Bastion、VPN 或等价私有路径保持管理访问私有。
4. 使用 `IMAGE=<已发布镜像> DOCKER_NETWORKS=<network1,network2> ./start.sh start` 启动。

启用 Azure NSG 协调后，网关会管理 80/443 及所有已启用公网 Listener 端口的入站访问，但绝不会开放 8080。`start.sh` 和独立 VM 部署都会把管理 UI 绑定到宿主机 `127.0.0.1:8080`。标准“容器 + Docker Socket”部署只发布 80/443，除非运维人员显式增加宿主机端口映射。

## 运行时探针

- `/livez` 在 Go 控制面运行时返回成功。
- `/readyz` 仅在必需的 Caddy 子进程就绪时返回成功。
- `/healthz` 是 `/readyz` 的兼容别名。
