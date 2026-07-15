# 生产部署

[English](deployment.md)

本项目提供两个互斥的生产部署档。两者使用同一个镜像，由 Caddy 终止 TLS，并按 Host 和 Path 将请求转发到不同服务与端口。

| 部署档 | 公网入口 | 服务发现 | 适用场景 |
|---|---|---|---|
| VM | VM Standard 静态公网 IP | Docker 标签和显式路由 | 网关与后端服务在同一台 VM |
| ACI | Standard Public Load Balancer | 显式路由 | 网关需要与后端 VM 分离 |

Application Gateway 不在最终数据路径中。Standard Load Balancer 只负责四层 TCP 转发；TLS、证书和七层路由全部由 Caddy 处理，因此域名数量不会增加 Azure Load Balancer 规则。

## 通用准备

构建并推送带固定版本标签的镜像：

```sh
make test
make docker-build IMAGE=<registry>/caddy-reverse-proxy:<version>
make docker-push IMAGE=<registry>/caddy-reverse-proxy:<version>
```

生产环境需要：

- 为每个公网域名设置指向入口静态公网 IP 的 A 记录。
- 公网开放 TCP 80 和 443，供 HTTP 跳转、ACME HTTP-01 和 HTTPS 使用。
- 持久化整个 `/data`，其中包含路由、审计日志和 Caddy 证书。
- 使用高强度随机管理员令牌，不能保留 `change-me`。
- 保持 8080 和 Caddy Admin API `127.0.0.1:2019` 不对互联网开放。

## VM 部署档

### 拓扑

```text
DNS -> VM Standard 静态公网 IP -> Caddy :80/:443
                                  -> Docker 私有网络中的 service:port
```

1. 给 VM 分配 Standard SKU 静态公网 IP，只在 NSG 中开放 TCP 80/443。
2. 如果工作负载使用自定义 Docker network，在启动前创建：

```sh
docker network create gateway-workloads
```

3. 启动单个网关容器。脚本会生成初始管理员令牌，并将整个 `/data` 持久化到 `~/docker_files/caddy-reverse-proxy`：

```sh
IMAGE=<registry>/caddy-reverse-proxy:<version> \
DOCKER_NETWORKS=gateway-workloads \
./start.sh start
```

通过下方 SSH 隧道打开 Console，再配置路由、ACME 邮箱、证书域名和 DNS challenge。只有必须在 Console 可访问前生效的基础设施集成才需要写入 `.env`。

可选的运行时 Azure A 记录协调需要在 `.env` 中配置 VM 入口 IP 与 Zone：

```dotenv
GATEWAY_AZURE_ENABLED=true
GATEWAY_AZURE_MANAGE_DNS=true
GATEWAY_AZURE_DNS_ZONES=[{"name":"example.com","resourceGroup":"dns-rg"},{"name":"example.net","resourceGroup":"dns-rg"}]
```

给 VM 托管身份在每个 Zone 上授予 `DNS Zone Contributor`。只有启用运行时 NSG 协调时才需要 `Network Contributor`。

4. 通过 SSH 隧道访问管理 UI：

```sh
ssh -L 8080:127.0.0.1:8080 <vm>
```

浏览器访问 `http://127.0.0.1:8080`。不要把宿主机 8080 绑定到 `0.0.0.0`。

### VM 验证

```sh
docker inspect caddy-reverse-proxy --format '{{.State.Status}} {{.State.Health.Status}}'
curl -fsS http://127.0.0.1:8080/livez
curl -fsS http://127.0.0.1:8080/readyz
curl --resolve app.example.com:443:<VM 公网 IP> https://app.example.com/
```

## ACI + Standard Load Balancer 部署档

### 拓扑

```text
DNS -> Standard Public Load Balancer
       TCP 80  -> VNet ACI:80
       TCP 443 -> VNet ACI:443
       HTTP readiness probe -> VNet ACI:8080/readyz

ACI/Caddy -> VM 私网 IP:不同端口
ACI 出站  -> NAT Gateway -> ACME、Azure API、镜像仓库
```

模板创建以下资源：

- Standard Load Balancer 和独立的静态入站公网 IP。
- TCP 80、TCP 443 两条负载均衡规则。
- `/readyz` HTTP 健康探针。
- 专用 VNet、ACI 委派子网、NSG、NAT Gateway 和独立出站公网 IP。
- 私网 ACI 容器组，私网开放 80、443、8080。
- Azure Files 共享并挂载到 `/data`。
- 用于 ACR/控制面操作的 UAMI，以及用于 Caddy DNS-01 的 system identity。
- 可选 `AcrPull`，以及在每个已配置 DNS Zone 上同时授予两个身份的 `DNS Zone Contributor`。

### 前置条件

- 模板默认创建专用 `10.42.0.0/24` VNet 和 `10.42.0.0/28` ACI 子网；与现有网络重叠时覆盖这些前缀。
- 上游使用私网地址时，将专用 VNet 与后端 VNet 建立 peering。后端 NSG 应只允许来自 ACI 子网的必要端口。
- 执行部署的身份能创建网络、ACI、存储和角色分配。
- 目标订阅与区域已验证支持 ACI 私网 IP作为 Standard Load Balancer IP backend。

### 参数与部署

[![Deploy to Azure](https://aka.ms/deploytoazurebutton)](https://portal.azure.com/#create/Microsoft.Template/uri/https%3A%2F%2Fraw.githubusercontent.com%2Fpczhao1210%2Fcaddy-reverse-proxy%2Fmain%2Fdeploy%2Faci%2Fazuredeploy.json)

Portal 只要求填写 `image` 与 `adminToken`。请先发布镜像；`dnsZones` 是可选参数，填写后模板会创建 A 记录协调与通配符证书 DNS-01 所需的两套 DNS 角色。证书域名与认证设置可稍后在 Console 中配置。

使用 CLI 部署时：

```sh
cp deploy/aci/main.example.bicepparam deploy/aci/main.bicepparam
export GATEWAY_ADMIN_TOKEN="$(openssl rand -base64 48)"
```

替换示例镜像名称；只有需要时才添加 ACR、VNet 前缀、管理域名或 `dnsZones` 等可选覆盖。

```sh
make aci-build
make aci-validate AZURE_RESOURCE_GROUP=<resource-group>
make aci-what-if AZURE_RESOURCE_GROUP=<resource-group>
make aci-deploy AZURE_RESOURCE_GROUP=<resource-group>
```

部署输出包含：

- `ingressPublicIPAddress`：所有公网域名应解析到此地址。
- `natPublicIPAddress`：ACI 出站地址，不能用于入站 DNS。
- `containerPrivateIPAddress`：当前 ACI 私网地址。
- `loadBalancerId`、`delegatedSubnetId`、两套 identity principal ID 和持久化存储账户名。

模板把实际 `containerPrivateIPAddress` 写入 LB backend pool。ACI 被 IaC重新创建时，重新运行 `make aci-deploy` 会同步 backend。不要在模板外单独删除并重建容器组；如果发生这种情况，立即重跑部署并检查 backend health。

管理端口 8080 没有 Load Balancer 规则，只能从 VNet、VPN 或跳板机访问。`managementHost` 默认为空；只有其 DNS 已指向入口 IP 且明确接受公网 token 保护管理入口时才设置。

进入 Console 后，同时添加 `*.example.com` 与 `example.com` 证书域名，选择 Azure DNS 和托管身份。部署输出的 system identity 需要 `DNS Zone Contributor`；通过 `dnsZones` 传入的 Zone 会自动授权。精确 Host 路由优先于 `*.example.com` fallback 路由。

### ACI 验证

```sh
az deployment group show -g <resource-group> -n <deployment-name> --query properties.outputs
az network lb show -g <resource-group> -n <name>-lb
az network lb show-backend-health -g <resource-group> -n <name>-lb
curl --resolve app.example.com:80:<ingress-ip> http://app.example.com/
curl --resolve app.example.com:443:<ingress-ip> https://app.example.com/
```

证书策略保存在 Azure Files 中，修改后不需要重新部署 ACI。

## 从 Application Gateway 迁移

1. 将现有 DNS TTL提前降低到 300 秒，并等待旧 TTL 过期。
2. 在新入口添加全部路由，但暂不改正式 DNS。
3. 使用 `curl --resolve` 分别验证 HTTP、HTTPS、WebSocket、Path路由和大请求。
4. 将 A 记录切换到 VM 或 Standard Load Balancer 的入站公网 IP。
5. 观察 Caddy日志、`/readyz`、LB backend health、证书签发和后端错误。
6. 保留旧 Application Gateway至少一个完整 TTL窗口。
7. 确认没有流量后再删除旧 listener、证书和 Application Gateway。

回滚时只需把 DNS A 记录改回旧 Application Gateway IP。不要同时让两个入口独立签发同一批证书并共用不一致的路由状态。

## 可用性边界

两个部署档当前都以单个 Caddy实例为写入主节点：

- Standard Load Balancer 提供稳定入口，但单个 ACI 仍不是高可用部署。
- `/data` 持久化可以避免重建后丢失证书和路由，但不能消除重启窗口。
- 不支持让多个可写实例同时共享 `routes.json`。双活前需要把路由状态迁移到具备并发控制的外部存储。
- 暂不启用 HTTP/3；启用时还需要 UDP 443 Load Balancer 规则和 ACI UDP 端口。