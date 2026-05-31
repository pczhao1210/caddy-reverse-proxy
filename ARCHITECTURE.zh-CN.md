# 架构

[English](ARCHITECTURE.md)

## 目标形态

```text
Internet
  -> platform gateway container :80/:443
    -> embedded Caddy runtime
    -> route table
    -> upstream Docker containers or explicit upstreams

Operators
  -> management UI/API :8080 or management host route
```

该平台被有意打包为单一网关镜像。控制平面拥有路由状态并渲染 Caddy JSON 配置。Caddy 负责 HTTP、HTTPS、自动证书管理、WebSocket 升级和反向代理行为。

## 运行时组件

- Gateway 进程：启动 API 服务、协调循环和 Caddy 运行时。
- Caddy 运行时：监听 80 和 443，通过仅本地可访问的管理端点接收生成后的配置。
- 路由来源：`vm` 配置档中来自 Docker 发现，`aci` 配置档中来自显式路由配置。
- Reconciler：合并路由来源，渲染期望的网关配置，重载 Caddy，并记录状态。
- Health checker：在协调期间探测配置的上游健康路径，并记录路由级就绪状态。
- Audit logger：将路由、bind、协调、DNS 和 NSG 变更摘要追加写入 JSONL 状态文件。
- 管理 UI：静态资源被内嵌进 Go 二进制并由 API 进程提供服务。

## VM 配置档流程

```text
Docker Engine -> discovery -> route model -> Caddy config -> public HTTPS route
```

在 `vm` 配置档中，容器通过标签被发现。工作负载不应直接暴露宿主机端口；它们应与网关容器共享一个私有 Docker 网络。

## ACI 配置档流程

```text
routes config/API -> route model -> Caddy config -> public HTTPS route
```

ACI 没有本地 Docker Engine。因此，除非将来增加远程 agent 或服务注册中心，否则该配置档不支持自动发现。

## 状态与持久化

平台将运行状态保存在 `/data/platform` 下。Caddy 证书存储使用 `/data/caddy`。生产环境必须持久化这些路径。VM 部署建议使用 Docker 卷；ACI 部署建议使用 Azure Files 或等价的持久化挂载。
