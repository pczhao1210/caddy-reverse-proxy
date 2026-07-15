# AI Docker Farm 边缘网关

[English](README.md)

[![Deploy to Azure](https://aka.ms/deploytoazurebutton)](https://portal.azure.com/#create/Microsoft.Template/uri/https%3A%2F%2Fraw.githubusercontent.com%2Fpczhao1210%2Fcaddy-reverse-proxy%2Fmain%2Fdeploy%2Faci%2Fazuredeploy.json)

AI Docker Farm 边缘网关是一个面向 Docker 与 Azure 工作负载的自托管入口平台。它被设计为单个容器镜像，可直接在 80 和 443 端口接收互联网流量，通过内嵌的 Caddy 运行时转发请求，提供管理 UI，并协调 Docker 发现以及 Azure DNS 与网络状态。

## MVP 范围

- 单一网关容器镜像。
- 通过仅监听本地回环的管理端点管理内嵌 Caddy 运行时。
- 使用 Go 构建控制平面 API，并内嵌静态管理 UI。
- `vm` 配置档用于与工作负载同机部署并基于标签发现容器。
- `aci` 配置档用于显式路由配置。
- 支持 HTTP-01 与 Azure DNS-01 自动 HTTPS，包括通配符证书。
- Azure DNS-01 支持托管身份与 App Registration 两种认证方式。

## 部署配置档

### VM 配置档

网关与工作负载运行在同一台 Ubuntu VM 上。它加入服务所在的 Docker 网络，通过受限 Docker socket 或 socket 代理发现容器，并将公网流量转发到内部容器名和端口。

### ACI 配置档

网关以注入 VNet 的私网 Azure Container Instance 运行，并由 Standard Public Load Balancer 提供公网入口。Load Balancer 只透传 TCP 80/443，不终止 TLS，因此证书和多域名路由仍由 Caddy 管理。ACI 不提供本地 Docker 自动发现，路由必须显式配置，上游使用 VNet 私网地址或私有 DNS。

## Quick Start

在仓库根目录直接启动 VM 配置档：

```sh
./start.sh start
```

脚本会在镜像不存在时自动构建，并且只启动一个网关容器。80/443 对外发布，Console 仅绑定 `127.0.0.1:8080`，全部状态持久化到 `~/docker_files/caddy-reverse-proxy`。如果 `.env` 没有自定义管理员令牌，脚本会自动生成，在启动时显示一次，并随数据目录持久化。

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

`push` 会使用当前 Docker Hub 登录信息中的用户名；如需覆盖，可设置 `PUSH_IMAGE=<组织名>/caddy-reverse-proxy:tag`。`stop` 保留数据目录。`restore` 只删除受管容器、选定镜像及 `~/docker_files` 下经过路径保护的项目目录，不会修改 `.env` 或 Git 文件。端口、镜像、network 与路径覆盖方式见 `./start.sh help`。

ACI 可直接使用上方 Deploy to Azure 按钮。模板会创建 VNet、ACI、Standard Load Balancer、NAT Gateway、Azure Files 持久化和托管身份；只需填写已发布镜像与管理员令牌。部署时提供 `dnsZones`，模板还会同时给控制面 UAMI 与 Caddy 的 system identity 授予 `DNS Zone Contributor`；其他证书设置可在 Console 启动后填写。

## 仓库结构

```text
backend/             Go 控制平面、Caddy 集成、内嵌 UI
config/              平台与路由配置示例
deploy/vm/           同机 VM 配置档的 Docker Compose 部署
deploy/aci/          私网 ACI + Standard Load Balancer Bicep 部署
docs/                运维与安全文档
```

## 当前实现状态

当前实现包括管理 API、内嵌 Alpine.js UI、Docker 标签发现、显式与通配符路由、Caddy 配置渲染、`protected/internal/public` 暴露模式、Caddy 生命周期监督、存活/就绪端点、路由与证书原子持久化、Azure DNS-01 通配符证书、路由健康检查、审计日志、多 Azure DNS Zone 协调，以及私网 ACI + Standard Load Balancer 基础设施。

生产部署见 [docs/deployment.zh-CN.md](docs/deployment.zh-CN.md)，运行参数见 [docs/operations.zh-CN.md](docs/operations.zh-CN.md)，剩余能力缺口见 [docs/roadmap.zh-CN.md](docs/roadmap.zh-CN.md)。
