# 路线图与能力缺口

[English](roadmap.md)

本文档跟踪当前已经实现的能力，以及在网关具备类似小型 Azure Application Gateway 面向 Docker 和 Azure 工作负载的行为之前，仍需完成的部分。

## MVP 已实现

- 单一容器镜像，同时包含 Go 控制平面与内嵌 Caddy 运行时。
- 管理 API 与内嵌 Alpine.js UI。
- 支持 Listener、Backend Pool 和 Routing Rule CRUD，使用版本化 JSON 持久化并自动迁移旧路由。
- Caddy JSON 配置渲染与 Admin API 重载。
- `vm` 配置档下通过 Docker socket 进行容器发现。
- 支持 `caddy.enable`、`caddy.host`、`caddy.port`、`caddy.websocket` 和 `exposure.mode` 的 Docker 标签路由提示。
- 从已发现 Docker 容器手动绑定为持久化显式路由。
- 在 Caddy 路由层支持 `public`、`internal` 与 `protected` 暴露模式。
- 通过 `DefaultAzureCredential` 协调 Azure DNS A 记录。
- 清理陈旧的、由网关管理的 Azure DNS A 记录。
- 通过 `DefaultAzureCredential` 协调公网 Listener 端口的 VM NSG 规则，并保留 80/443 用于 ACME 和默认入口。
- 当不再存在公网路由时，清理由网关管理的 VM NSG 入站规则。
- 交互式独立 Azure VM 部署，支持 VNet/子网选择、静态公网 IP、受限 NSG、托管身份、Docker 安装和网关状态持久化。
- 通过管理员令牌保护管理 API。
- 面向小团队运维的多管理员令牌 allowlist。
- 可配置的 protected 路由策略，支持 bearer token、`X-Admin-Token` 和可选自定义 Header 匹配。
- 原子持久化的证书 UI/API，支持显式证书域名、Azure DNS-01 通配符签发、托管身份/App Registration 认证、密钥脱敏和触发 Caddy 重新加载的刷新动作。
- 路由与上游健康检查，并在 API/UI 状态中报告路由级错误。
- 审计日志，覆盖路由变更、手动 Docker bind、协调运行、DNS 变更和 NSG 变更摘要。
- 托管 VM 入站 NSG 规则的优先级和源地址前缀策略控制。
- `vm` 配置档下的 Docker socket proxy 部署选项。
- 覆盖 Caddy 与示例 Docker 服务路由路径的 E2E 脚本。
- Caddy 生命周期监督，以及 `/livez` 和 `/readyz` 编排探针。
- 串行协调、最近成功 Docker 发现路由回退和原子路由文件替换。
- internal CIDR 限制、确定性 Path 优先级、统一 upstream transport 和网关凭据移除。
- 使用显式入口公网 IP 的多 Azure DNS Zone 协调。
- 已有主机的单容器生命周期脚本，以及可从 Cloud Shell/本地 Azure CLI 运行的独立 Azure VM 部署脚本。

## 后续强化

- 生产级多用户治理建议用 Entra ID/OIDC 替换基于令牌的管理认证。
- 当前健康检查是简单 HTTP 状态探针；后续可增加按路由配置的间隔、阈值和主动/被动策略。
- 当前 E2E 测试是本地 Docker 脚本；当 CI runner 能暴露 80 和 8080 端口后，应提升为 CI 检查。
- 双活实例需要具备并发控制的外部路由存储；多个写入实例不能安全共享 `routes.json`。

## 路由资源模型

持久化 v2 模型和“路由”UI 现已使用三类可复用资源：

- 监听器（Listener）：前端主机名、端口和 HTTP/HTTPS 协议。
- 后端池（Backend Pool）：一组命名的 IP 地址或 DNS 名称。
- 路由规则（Routing Rule）：选择一个监听器和后端池，并设置后端协议/端口、路径、健康检查路径、暴露方式和 WebSocket 行为。

Store 会把这三类资源编译为现有 Reconciler、健康检查、Azure 和 Caddy 使用的运行时路由模型。旧版 `routes` 文件会在加载成功后原子迁移为 v2；旧 Route API 暂时保留为 Docker bind 和现有客户端的兼容适配层。

证书策略目前仍按全局 subject 管理，尚未成为按 Listener 绑定的独立资源。Docker 发现的服务身份也仍是运行时输入，而不是持久化的一等资源。

## 当前 UI 状态含义

- Azure `Enabled: No` 表示 Azure 协调器代码可用，但当前配置未启用。
- Azure `Configured: No` 表示缺少订阅、资源组、DNS Zone 或 NSG 名称等必要设置。
- 本地预览中的 Docker `Active: No` 通常表示预览启动时设置了 `GATEWAY_DOCKER_ENABLED=false`，或者未挂载 Docker socket。
- 独立网关 VM 使用显式私网后端路由并刻意关闭本地发现，因此 Docker `Active: No` 属于预期状态。

## 建议的下一里程碑

下一步建议推进 Entra ID/OIDC 管理认证和 CI 化 E2E 覆盖。网关现在已经具备运维闭环：部署容器、绑定路由、协调网络状态、获取 HTTPS、审计变更，并在 UI 中显示健康/错误状态。
