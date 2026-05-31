# AI Docker Farm 边缘网关

[English](README.md)

AI Docker Farm 边缘网关是一个面向 Docker 与 Azure 工作负载的自托管入口平台。它被设计为单个容器镜像，可直接在 80 和 443 端口接收互联网流量，通过内嵌的 Caddy 运行时转发请求，提供管理 UI，并协调 Docker 发现以及 Azure DNS 与网络状态。

## MVP 范围

- 单一网关容器镜像。
- 通过仅监听本地回环的管理端点管理内嵌 Caddy 运行时。
- 使用 Go 构建控制平面 API，并内嵌静态管理 UI。
- `vm` 配置档用于与工作负载同机部署并基于标签发现容器。
- `aci` 配置档用于显式路由配置。
- 以公共 DNS 和自动 HTTPS 作为主要证书路径。
- Azure 集成仅使用托管身份处理 DNS 和 NSG。

## 部署配置档

### VM 配置档

网关与工作负载运行在同一台 Ubuntu VM 上。它加入服务所在的 Docker 网络，通过受限 Docker socket 或 socket 代理发现容器，并将公网流量转发到内部容器名和端口。

### ACI 配置档

网关运行在 Azure Container Instances 中。ACI 不提供本地 Docker 自动发现，因此路由必须来自显式配置或管理 API。上游服务必须能从 ACI 容器组访问，可通过公网网络、VNet 注入、Private Link 或私有 DNS 实现。

## Quick Start

先从示例文件复制出本地环境文件。`.env` 已被 Git 忽略。

```sh
cp .env.example .env
```

构建单镜像：

```sh
make docker-build
```

在启用 Docker 发现的情况下运行：

```sh
make docker-run
```

`make docker-run` 使用 Docker 默认 bridge 网络，不会创建自定义网络。在默认 bridge 网络下，Docker 发现会优先使用 inspect 得到的容器 IP 作为上游地址。

如果把网关放进新的自定义 Docker network，它不能自动代理仍然只在默认 `bridge` 网络里的容器，除非两边之间存在可达路径。可以把工作负载加入网关 network、让网关加入多个 network，或通过宿主机发布端口进行路由。

VM 部署的最佳实践是让网关作为普通容器运行，加入它需要服务的工作负载网络，并只把网关自己的 80/443/管理端口发布到宿主机。`network_mode: host` 是另一种独立模式，不是“让网关加入多个 Docker network”的方式。

默认配置写在 `.env` 中。运行 `make docker-run` 或 `make compose-up` 前请先编辑该文件；所有选项及含义见 [docs/operations.zh-CN.md](docs/operations.zh-CN.md)。

在 `http://localhost:8080` 打开管理 UI，并使用令牌 `change-me` 登录。

Host 网络模式也可以代理流量，尤其适合 `http://127.0.0.1:3000` 这类宿主机本地上游；但它更偏 Linux 场景，会取消网关容器自身的网络隔离，因此更适合作为显式部署选择，而不是默认预览模式。

## 仓库结构

```text
backend/             Go 控制平面、Caddy 集成、内嵌 UI
config/              平台与路由配置示例
deploy/vm/           同机 VM 配置档的 Docker Compose 部署
deploy/aci/          ACI 部署起始模板
docs/                运维与安全文档
```

## 当前实现状态

当前仓库提供了聚焦的 MVP：管理 API、内嵌 Alpine.js UI、Docker 标签发现、手动绑定、显式路由、Caddy 配置渲染、`protected/internal/public` 暴露模式、Caddy 进程管理、运行时证书签发器策略控制、路由健康检查、审计日志、Docker socket proxy 部署，以及在启用后通过 `DefaultAzureCredential` 实现 Azure DNS/NSG 协调。

运维细节见 [docs/operations.zh-CN.md](docs/operations.zh-CN.md)，当前能力缺口和建议的下一阶段优先级见 [docs/roadmap.zh-CN.md](docs/roadmap.zh-CN.md)。
