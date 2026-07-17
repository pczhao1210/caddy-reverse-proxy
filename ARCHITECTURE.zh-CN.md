# 架构

[English](ARCHITECTURE.md)

## 目标形态

```text
Internet
  -> VM 静态公网 IP
  -> platform gateway container :80/:443
    -> embedded Caddy runtime
    -> route table
    -> 显式私网上游或同机 Docker 容器

Operators
  -> management UI/API :8080 or management host route
```

该平台被有意打包为单一网关镜像。控制平面拥有路由与证书策略状态并渲染 Caddy JSON 配置。Caddy 内置 Azure DNS provider，负责 HTTP、HTTPS、HTTP-01/DNS-01 证书管理、WebSocket 升级和反向代理行为。

## 运行时组件

- Gateway 进程：启动 API 服务、协调循环和 Caddy 运行时。
- Caddy 运行时：必需子进程，监听默认 80/443 入口及已配置的 Listener 端口，通过仅本地可访问的管理端点接收配置。启动失败或运行中异常退出会终止容器，由编排器重启。
- 路由来源：持久化 Listener、Backend Pool 和 Routing Rule，以及同机部署时可选的 Docker 发现。
- Reconciler：合并路由来源，渲染期望的网关配置，重载 Caddy，并记录状态。
- 请求安全基线：在每条代理规则前生成 Caddy 原生匹配器和处理器，执行请求体上限、方法/路径拒绝以及直连客户端 IP/CIDR 策略。
- 设置存储：原子持久化 Console 管理的安全、认证、期望 Deployment 和 Azure 设置。安全策略与令牌会传播到运行中的 API/Reconciler/Renderer；Deployment 和 Azure 客户端改动在重启后生效。
- Health checker：在协调期间探测配置的上游健康路径，并记录路由级就绪状态。
- Audit logger：将路由、bind、协调、DNS 和 NSG 变更摘要追加写入 JSONL 状态文件。
- 管理 UI：静态资源被内嵌进 Go 二进制并由 API 进程提供服务。
- 运行时探针：`/livez` 表示控制面存活；`/readyz` 和兼容端点 `/healthz` 要求 Caddy 已就绪。

## VM 流程

```text
Listener + Backend Pool + Routing Rule -> 路由编译器 -> 运行时路由模型 -> Caddy 配置
旧 Route API / Docker bind ----------------^              ^
本机 Docker Engine -> 可选自动发现 ------------------------+
```

独立网关 VM 默认关闭 Docker 发现，Console/API 路由指向 VNet 私网 IP 或 DNS。与工作负载同机时，可以通过受限 socket proxy 读取 Docker 标签。工作负载不应直接暴露宿主机端口，而应与网关容器共享私有 Docker 网络。

## 状态与持久化

平台将版本化路由资源、审计数据、Console 证书策略和 Console 管理的设置保存在 `/data/platform`。`settings.json` 包含管理员令牌；它与其他敏感状态在 POSIX 文件系统上的创建权限为 `0600`。旧路由文件会被原子迁移为 Listener、Backend Pool 和 Routing Rule。Caddy 证书存储位于 `/data/caddy`。生产环境必须持久化整个 `/data`。Azure VM 部署 bind mount `/var/lib/caddy-reverse-proxy`；已有主机上的 `start.sh` 使用 `~/docker_files/caddy-reverse-proxy`。
