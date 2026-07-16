# AI Docker Farm 边缘网关

[English](README.md)

AI Docker Farm 边缘网关是一个面向 Docker 与 Azure 工作负载的自托管入口平台。它被设计为单个容器镜像，可直接在 80 和 443 端口接收互联网流量，通过内嵌的 Caddy 运行时转发请求，提供管理 UI，并协调 Docker 发现以及 Azure DNS 与网络状态。

已发布镜像：`docker.io/pczhao1210/caddy-reverse-proxy:latest`  
当前 digest：`sha256:0e75a5bbeccb3b9354516e757bb805803a501cf6cca03988028e03030aa94c52`

## MVP 范围

- 单一网关容器镜像。
- 通过仅监听本地回环的管理端点管理内嵌 Caddy 运行时。
- 使用 Go 构建控制平面 API，并内嵌静态管理 UI。
- VM 部署支持显式路由，并可在与 Docker 工作负载同机时启用标签发现。
- 可从 Cloud Shell 或本地 Azure CLI 环境交互创建独立 Azure VM。
- 支持 HTTP-01 与 Azure DNS-01 自动 HTTPS，包括通配符证书。
- Azure DNS-01 支持托管身份与 App Registration 两种认证方式。
- 内置请求体上限、危险方法/路径拒绝和 IP/CIDR 访问策略等请求安全基线。

## 部署方式

### 同机 VM

网关与工作负载运行在同一台 Ubuntu VM 上。它加入服务所在的 Docker 网络，通过受限 Docker socket 或 socket 代理发现容器，并将公网流量转发到内部容器名和端口。

### 独立 Azure VM

网关运行在独立 Ubuntu VM 上，使用一个 Standard 静态公网 IP。它通过 VNet 私网地址或私有 DNS 访问后端，并使用显式路由；不需要 Load Balancer 或 NAT Gateway。

## Quick Start

在仓库根目录直接启动 VM 配置档：

```sh
./start.sh start
```

脚本会在镜像不存在时拉取 `pczhao1210/caddy-reverse-proxy:latest`，并且只启动一个网关容器。80/443 对外发布，Console 仅绑定 `127.0.0.1:8080`，全部状态持久化到 `~/docker_files/caddy-reverse-proxy`。如果 `.env` 没有自定义管理员令牌，脚本会自动生成，在启动时显示一次，并随数据目录持久化。

打开 `http://127.0.0.1:8080`，使用该令牌登录，然后在 Console 中配置路由和证书。申请 `*.example.com` 时，在 **网络 → 证书** 中同时添加 `*.example.com` 与 `example.com`，选择 Azure DNS 并填写 Azure 认证设置。通配符不覆盖根域名，因此根域名需要单独列出。

需要通过容器名访问已有工作负载时，将网关加入对应 Docker network：

```sh
DOCKER_NETWORKS=frontend,internal ./start.sh start
```

其他生命周期命令支持 `build` 与 `--build` 两种形式：

```sh
./start.sh build
./start.sh push
./start.sh stop
./start.sh restore
```

`build` 与 `push` 默认使用 `pczhao1210/caddy-reverse-proxy:latest`；如需发布其他仓库或 tag，可覆盖 `IMAGE` 或 `PUSH_IMAGE`。`stop` 保留数据目录。`restore` 只删除受管容器、选定镜像及 `~/docker_files` 下经过路径保护的项目目录，不会修改 `.env` 或 Git 文件。端口、镜像、network 与路径覆盖方式见 `./start.sh help`。

交互部署脚本支持两种模式：创建独立 Azure VM，或只在当前机器部署网关容器。Azure 模式需要 Azure Cloud Shell，或本地 Bash 4+ 与 Azure CLI；同机模式需要 Bash 4+ 和可访问的 Docker Engine。任选下面一段执行；两种方式都会先下载到临时文件，仅在下载成功后运行：

```bash
deploy_script=$(mktemp) &&
	curl -fsSL https://raw.githubusercontent.com/pczhao1210/caddy-reverse-proxy/main/deploy/vm/deploy.sh -o "$deploy_script" &&
	bash "$deploy_script"
status=$?
rm -f "${deploy_script:-}"
test "$status" -eq 0
```

```bash
deploy_script=$(mktemp) &&
	wget -qO "$deploy_script" https://raw.githubusercontent.com/pczhao1210/caddy-reverse-proxy/main/deploy/vm/deploy.sh &&
	bash "$deploy_script"
status=$?
rm -f "${deploy_script:-}"
test "$status" -eq 0
```

选择第一种模式会创建 Azure 基础设施：交互选择订阅、区域、VNet、子网、VM 规格和系统盘，创建静态公网 IP、NSG、NIC、托管身份及 Ubuntu VM，安装 Docker 并启动网关。当前机器已有 Docker 时选择第二种模式；脚本会复用仓库中的 `start.sh`，或把最小 launcher 文件安装到 `~/caddy-reverse-proxy`，只启动容器，不修改 Azure 基础设施或 DNS。

已克隆仓库时可分别运行 `make azure-vm-deploy` 或 `make container-deploy`。默认镜像固定到 digest，可通过 `IMAGE` 覆盖；Azure 模式支持 `ROLLBACK_ON_ERROR=false`，同机模式支持与 `start.sh` 相同的 `CONTAINER_NAME`、`DATA_DIR`、端口和 `DOCKER_NETWORKS` 覆盖。权限、持久化与网络约束见 [docs/deployment.zh-CN.md](docs/deployment.zh-CN.md)。

## 仓库结构

```text
backend/             Go 控制平面、Caddy 集成、内嵌 UI
config/              平台与路由配置示例
deploy/vm/           独立 Azure VM 脚本与 Docker Compose 部署
docs/                运维与安全文档
```

## 当前实现状态

当前实现包括管理 API、内嵌 Alpine.js UI、可选 Docker 标签发现、显式与通配符路由、Caddy 配置渲染、`protected/internal/public` 暴露模式、轻量请求安全基线、Caddy 生命周期监督、存活/就绪端点、路由与证书原子持久化、Azure DNS-01 通配符证书、路由健康检查、审计日志、多 Azure DNS Zone 协调，以及交互式独立 Azure VM 部署。

生产部署见 [docs/deployment.zh-CN.md](docs/deployment.zh-CN.md)，运行参数见 [docs/operations.zh-CN.md](docs/operations.zh-CN.md)，剩余能力缺口见 [docs/roadmap.zh-CN.md](docs/roadmap.zh-CN.md)。
