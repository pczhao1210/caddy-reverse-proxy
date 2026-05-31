# 安全

[English](SECURITY.md)

## 公网边缘模型

平台容器是一个公网边缘网关。默认情况下，只有 80 和 443 端口应该对互联网开放。管理端口 8080 应保持私有，只绑定到本地回环，通过 VPN 保护，或者仅通过专用且带认证的管理域名暴露。

## 管理 API

当前实现使用管理员令牌或逗号分隔的令牌 allowlist 保护管理 API 调用。生产部署应迁移到 Entra ID/OIDC。在此之前，应将 `GATEWAY_ADMIN_TOKEN` 或 `GATEWAY_ADMIN_TOKENS` 设置为高强度随机值，并避免公开暴露 8080 端口。

## Docker Socket

不要将 `/var/run/docker.sock` 直接暴露给不受信任的容器或网络。`vm` 配置档可以使用 `make compose-up-proxy` 通过 Docker socket proxy 进行发现，并限制为必要的检查权限。

## Caddy 管理端点

Caddy 的管理端点绑定在平台容器内部的 `127.0.0.1:2019`。绝不能将其映射为宿主机端口。

## Azure 身份

平台设计为使用 `DefaultAzureCredential` 与托管身份。不要在镜像中存储客户端密钥、服务主体密码或本地 Azure 令牌。

## 网络规则

`vm` 配置档应只管理 80 和 443 所需的最小入站规则。ACI 网络与 VM 的 NSG 行为不同；网络变更应基于能力判断，不应声称管理未附着到容器组的资源。
