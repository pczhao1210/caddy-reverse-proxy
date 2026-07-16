# 安全

[English](SECURITY.md)

## 公网边缘模型

平台容器是一个公网边缘网关。默认情况下，只有 80 和 443 端口应该对互联网开放。管理端口 8080 应保持私有，只绑定到本地回环，通过 VPN 保护，或者仅通过专用且带认证的管理域名暴露。

## 管理 API

当前实现使用管理员令牌或逗号分隔的令牌 allowlist 保护管理 API 调用。生产部署应迁移到 Entra ID/OIDC。在此之前，应将 `GATEWAY_ADMIN_TOKEN` 或 `GATEWAY_ADMIN_TOKENS` 设置为高强度随机值，并避免公开暴露 8080 端口。

受保护路由在代理请求前会删除所有已启用的网关凭据 Header。如果上游需要自己的 `Authorization`，应关闭 bearer 网关认证并改用专用网关 Header。

## Docker Socket

不要将 `/var/run/docker.sock` 直接暴露给不受信任的容器或网络。`vm` 配置档可以使用 `make compose-up-proxy` 通过 Docker socket proxy 进行发现，并限制为必要的检查权限。

## Caddy 管理端点

Caddy 的管理端点绑定在平台容器内部的 `127.0.0.1:2019`。绝不能将其映射为宿主机端口。

## Azure 身份

控制面 Azure 操作与 Azure DNS-01 都应优先使用 VM 的系统分配托管身份。没有托管身份的环境可以使用 App Registration；客户端密钥只保存在 `/data/platform/certificate.json` 中，API 永不回传。该文件在 POSIX 文件系统上以 `0600` 创建；应把 VM 磁盘和整个 `/data` 作为含敏感信息的状态保护。绝不能把客户端密钥、服务主体密码或本地 Azure 令牌写入镜像。

## 网络规则

VM NSG 应允许公网 TCP 80/443，并把 TCP 22 限制在运维来源 CIDR 或私有管理路径。8080 只绑定 VM 回环地址，不能创建入站规则。后端端口应只允许网关 VM 私网 IP 或所在子网；部署脚本不会修改后端 NSG 或主机防火墙。
