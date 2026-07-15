# 架构

[English](ARCHITECTURE.md)

## 目标形态

```text
Internet
  -> VM 静态公网 IP，或私网 ACI 前的 Standard Load Balancer
  -> platform gateway container :80/:443
    -> embedded Caddy runtime
    -> route table
    -> upstream Docker containers or explicit upstreams

Operators
  -> management UI/API :8080 or management host route
```

该平台被有意打包为单一网关镜像。控制平面拥有路由与证书策略状态并渲染 Caddy JSON 配置。Caddy 内置 Azure DNS provider，负责 HTTP、HTTPS、HTTP-01/DNS-01 证书管理、WebSocket 升级和反向代理行为。

## 运行时组件

- Gateway 进程：启动 API 服务、协调循环和 Caddy 运行时。
- Caddy 运行时：必需子进程，监听 80 和 443，通过仅本地可访问的管理端点接收配置。启动失败或运行中异常退出会终止容器，由编排器重启。
- 路由来源：`vm` 配置档中来自 Docker 发现，`aci` 配置档中来自显式路由配置。
- Reconciler：合并路由来源，渲染期望的网关配置，重载 Caddy，并记录状态。
- Health checker：在协调期间探测配置的上游健康路径，并记录路由级就绪状态。
- Audit logger：将路由、bind、协调、DNS 和 NSG 变更摘要追加写入 JSONL 状态文件。
- 管理 UI：静态资源被内嵌进 Go 二进制并由 API 进程提供服务。
- 运行时探针：`/livez` 表示控制面存活；`/readyz` 和兼容端点 `/healthz` 要求 Caddy 已就绪。

## VM 配置档流程

```text
Docker Engine -> discovery -> route model -> Caddy config -> public HTTPS route
```

在 `vm` 配置档中，容器通过标签被发现。工作负载不应直接暴露宿主机端口；它们应与网关容器共享一个私有 Docker 网络。

## ACI 配置档流程

```text
Standard Load Balancer TCP 80/443 -> VNet 私网 ACI
routes config/API -> route model -> Caddy config -> VM 私网上游
```

Load Balancer 不终止 TLS，也不识别域名。ACI 没有本地 Docker Engine，因此除非将来增加远程 agent 或服务注册中心，否则该配置档不支持自动发现。NAT Gateway 提供 ACI 出站，Azure Files 持久化整个 `/data`。UAMI 用于 ACR 与控制面 Azure 操作，system identity 供 Caddy 的 Azure DNS-01 provider 使用。

## 状态与持久化

平台将路由、审计数据和 Console 证书策略保存在 `/data/platform`；POSIX 文件系统上证书策略创建权限为 `0600`。Caddy 证书存储位于 `/data/caddy`。生产环境必须持久化整个 `/data`。VM 中 `start.sh` bind mount `~/docker_files/caddy-reverse-proxy`，ACI 通过 CIFS 使用 Azure Files。
