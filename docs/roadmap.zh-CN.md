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
- 串行配置提交、代次校验的状态发布、最近成功 Docker 发现路由回退和原子路由文件替换。
- internal CIDR 限制、确定性 Path 优先级、统一 upstream transport 和网关凭据移除。
- 使用显式入口公网 IP 的多 Azure DNS Zone 协调。
- 已有主机的单容器生命周期脚本，以及可从 Cloud Shell/本地 Azure CLI 运行的独立 Azure VM 部署脚本。

## 后续强化

### ARM64 运行执行追加验证（2026-10-30）

- 已定位候选失败原因为宿主机缺少 ARM64 执行支持，而非二进制错误：两个 ELF machine 均为 ARM64，受控执行复现 `exec format error`。经宿主机所有者明确授权后仅注册 ARM64 binfmt，未改变 builder 或发布镜像。
- 两个实际 AMD64/ARM64 候选均通过隔离启动/ELF/Caddy 插件/健康/鉴权及原始/SBOM/适用性扫描；全部后端普通 Go 测试以 ARM64 代码在 QEMU 下运行，使用真实候选 Caddy，AMD64 通过原生全量 race/vet。
- 启动/ELF 执行失败现在会输出状态/日志，不再误标为架构不匹配；扫描容器显式选择镜像平台。44 项扫描/发布回归全部通过。ARM64 race、原生 ARM 硬件/性能及真实云端/ACME 验证仍未完成。本次追加验证未晋升标签或部署。

### JOSE v4 修复与限定范围 VEX（2026-10-30）

- 审查后追加策略隔离：三次 Trivy 扫描忽略外部过滤/配置/ignore 规则，保留传输与缓存设置，仅允许已验证 VEX 过滤告警。41 项扫描/发布回归通过，覆盖敌对过滤条件下仍阻断其他无修复版本漏洞，并完成真实同镜像正常/敌对环境对照。本次仅修改 shell 工作流，未重建镜像，也未关闭 ARM64 运行门槛。
- 新增固定校验和的 Smallstep 源码补丁，使用 go-jose v4.1.6，保留全部 audience 校验、旧 JWT builder 接口及 XEdDSA key ID，继续拒绝无效 Ed25519 公私钥对/过短 HMAC 密钥，没有删除 Caddy 默认 PKI/本地 ACME/Azure 模块。
- 构建时对照准确源码/二进制模块图，生成架构独立的哈希、源码/补丁来源与仅针对 OpenPGP 的 VEX。扫描包装脚本锁定 image ID、核验两个可执行文件、拒绝额外 Go 二进制，并单独扫描补丁上游基础版本，避免本地替换的版本信息盲点。两个多架构候选晋升目标标签前都必须通过，现在要求安装 Trivy。
- 验证通过：依赖 race 测试、真实 Caddy 全量 Go race/vet（含隔离内部 CA 签发）、37 项 Node 回归、Compose/shell 检查，以及最终镜像桌面/390px 浏览器流程。AMD64 原始告警现为两条 OpenPGP 模块记录；本次快照中适用告警与补丁基础版本 SBOM 告警均为零。
- ARM64 二进制及其独立源码图证据交叉构建成功；完整 ARM64 运行/镜像及真实云端/ACME 门槛仍未关闭。未发布或部署镜像。本地补丁需要维护，直到兼容且已修复的上游版本可替代；细节和移除条件见[运维指南](operations.zh-CN.md#本地-jose-迁移与证据绑定扫描)。

### 依赖与配置修复（2026-10-30）

本节记录当天较早的基线；上方 JOSE 修复记录已更新其剩余风险状态。

- 最新稳定版追加升级已按顺序实施：Go 1.27.3/Alpine 3.24.3 与 HAProxy 3.4.7，Caddy 的 CEL 0.33.1/QUIC 0.64.1/OpenTelemetry 1.47.0/Prometheus 系列及其他间接依赖，最后迁移 Azure Network SDK v12.1.0。已审阅并固定提交型更新，通过实际运行时依赖元数据核验版本，而非只看请求的替换版本。
- 新增真实 HTTP/3 路由/鉴权及 metrics 回归，以及 Azure SDK 请求版本/路径/请求体、资源不存在和读取错误检查。生产拓扑及所有权语义不变，真实云端 API 可用性仍是未验证门槛。
- 已升级控制平面与自定义 Caddy 的两套依赖图、共享 Go 构建工具链、运行时基础镜像及内嵌 Alpine.js。新增受控 socket-proxy/httpbin 重建；生产环境要求预加载 socket-proxy 镜像，不隐式构建或拉取。准确版本和准备步骤见[运维指南](operations.zh-CN.md#依赖升级与配置修复2026-10-30)。
- 修复安全策略/令牌/证书的即时更新：Caddy 接受候选配置后才落盘，保留待应用路由，并在加载/落盘失败时恢复旧运行配置。回归覆盖请求取消、确认丢失、回滚失败及提交后的云端错误；这不提供崩溃原子性保证。
- 统一 CRUD、配置包导入与 v1/v2/v3 文件加载中的空路径和 `/` 根路径。存在歧义的旧文件会被拒绝且保持原样，不再静默遮蔽路由。
- 验证通过：真实升级版 Caddy 的全量 Go race/vet、24 项 Node 测试、Compose 配置校验、原生 AMD64 网关/配套镜像构建、mock socket 与非 root httpbin 冒烟，以及桌面/390px 浏览器流程。ARM64 控制平面/Caddy/httpbin 交叉编译和 ELF 检查通过。ARM64 运行/完整镜像验证受本机 `exec format error` 阻塞；下方早期 ARM64 结果属于历史记录，不能作为此次升级的验证。
- 本地 AMD64 网关扫描从 62 条包记录降至 5 条，两种重建配套镜像在同一数据库下均为零项。尚无修复版本的 go-jose v3 与 OpenPGP 公告仍是安全发布门槛。未发布或部署镜像，未进行真实云端/ACME、生产证书或破坏性 E2E 操作。
- 下一门槛：修复或明确评估剩余公告，并在获批的原生/模拟环境中验证完整 ARM64 镜像，再考虑发布。

### 多架构发布（2026-09-22）

- 已实现并发布同时包含 `linux/amd64` 与 `linux/arm64` 的统一 `latest` 索引。原生交叉编译、不可变候选验证、最终镜像冒烟检查、目标并发变更检测及发布后摘要核验，由 `start.sh push` 与 `make docker-push` 共用；本地 `build` 仍保持单架构。
- 验证通过：15 项发布回归，原生全量 Go race/vet，QEMU ARM64 Caddy/证书测试（HTTP/TLS/Host/鉴权/存储锁），两个候选容器的健康/鉴权/ELF 检查，以及远端平台/摘要核验。本机普通 pull 自动选择 AMD64。发布索引、分架构体积、环境准备及回退说明见[运维指南](operations.zh-CN.md#多架构发布)。
- 剩余门槛：未进行原生 Ubuntu ARM64 部署/性能及真实云端/ACME 验证。依赖安全例外仍保留，包括在未变化的 AMD64 二进制中新增记录的 OpenTelemetry 告警。发布不等于生产部署或安全清零。下一最小步骤：授权 ARM64 预发布验证，并单独处理依赖风险。

### 依赖升级（2026-09-22）

- 本地实施完成：Go 1.26.8、Caddy 2.11.4、xcaddy 0.4.7、两端一致的 CertMagic 0.25.4、Azure SDK 与 Go 安全依赖、Alpine Linux 3.22.6、Alpine.js 3.17.4、socket proxy v0.5.0 及示例 httpbin 2.25.0。基础/附属镜像固定 digest，保留 Azure DNS 0.6.0；显式保留 HTTPS 上游 Host 兼容行为并增加回归测试。
- 验证通过：最终镜像构建，使用真实 Caddy 二进制的全量 Go race/vet，Node 6/6，浏览器登录/表单/证书归档 fixture 和 401/503 检查、桌面/390px 布局，Compose 渲染及隔离只读 socket proxy/httpbin 测试。控制平面源码扫描无可达漏洞，网关镜像 OS 包扫描无发现。
- 安全发布门槛尚未关闭：Caddy CEL/OpenPGP 公告、socket proxy OpenSSL 风险及示例 httpbin 的旧 Go 工具链。版本、证据与限制详见[依赖升级基线](operations.zh-CN.md#依赖升级基线2026-09-22)。未部署，未进行真实 Azure/公网 ACME 或生产归档。
- 下一最小步骤：解决或明确评估上游例外，重扫准确产物后再进行授权预发布演练。升级与构建成功不等于零漏洞。

### 证书生命周期修复（2026-09-22）

- 已实现：当前策略/历史/未知证书清单分类、独立的预计有效期窗口、有限的近期签发/续期事件，以及带确认的可恢复归档；包含运行配置、TLS、路径、文件身份、配置锁与 CertMagic 存储锁检查。
- 本地验证：全量 Go race 与 vet，渲染器/API/保护条件回归，真实 Caddy 2.10.2 活动配置读取及手动加载拒绝，Node UI 回归 6/6；浏览器 fixture 验证筛选、取消/确认归档、事件展示、桌面与 390px 布局。未请求公网 CA，也未删除生产证书。
- 剩余环境门槛：真实 ACME 续期、生产存储归档演练、共享存储/崩溃故障注入。不保证多实例归档协调与整机断电持久性。下一最小步骤：先备份数据，在明确授权的测试环境将新清单与实际运行配置核对，再演练归档，详见运维指南。

### 按优先级审查修复（2026-09-22）

- 阶段 1-4 已实现：HTTP/TLS 隔离、管理 API 凭据保留、desired/applied 版本重启隔离、并发 Apply 确认、Azure 实例归属、Docker 共享网络选择及非认证 503 处理。
- 阶段 5 已实现：提交锁外有界并发健康检查、逆向分块审计尾读、Azure 无变化跳过写入、共享原子 JSON 持久化，以及删除两个无引用包装函数。保留兼容 API。
- 阶段 6 本地验证完成：全量 `go test -race ./...`、`go vet ./...` 通过，包含通过 `CADDY_TEST_BIN` 启用的隔离 Caddy 2.10.2 HTTP/TLS/鉴权验证；Node 认证回归 4/4 通过。浏览器验证了真实登录、注入 401/503、草稿 pending，以及点击 Apply 后真实 Caddy 转发；390px 视口无横向溢出。JS/Shell 语法、编辑器诊断和 `git diff --check` 通过。SDK 使用 mock transport，没有改动云资源。百万条审计取尾部的本机基准约 0.15 ms/次，不代表生产吞吐承诺。
- 未执行的可选/环境门槛：真实 Azure、公网 ACME、完整 Docker 栈 E2E、多文件导入中途崩溃注入及生产压测。现有 E2E 脚本会关闭示例栈并删除卷，因此未对当前工作负载运行。多文件导入仍不保证进程崩溃时的跨文件原子性。
- 下一最小步骤：备份状态，在明确授权的测试环境演练 v3/资源归属迁移，再安排生产升级，详见运维指南。本轮没有部署或修改云资源。

- 生产级多用户治理建议用 Entra ID/OIDC 替换基于令牌的管理认证。
- 当前健康检查是简单 HTTP 状态探针；后续可增加按路由配置的间隔、阈值和主动/被动策略。
- 当前 E2E 测试是本地 Docker 脚本；当 CI runner 能暴露 80 和 8080 端口后，应提升为 CI 检查。
- 双活实例需要具备并发控制的外部路由存储；多个写入实例不能安全共享 `routes.json`。

## 路由资源模型

v2 资源模型和“路由”UI 使用三类可复用资源，磁盘由 v3 封装：

- 监听器（Listener）：前端主机名、端口和 HTTP/HTTPS 协议。
- 后端池（Backend Pool）：一组命名的 IP 地址或 DNS 名称。
- 路由规则（Routing Rule）：选择一个监听器和后端池，并设置后端协议/端口、路径、健康检查路径、暴露方式和 WebSocket 行为。

Store 会把资源编译为 Reconciler、健康检查、Azure 和 Caddy 使用的运行时模型。旧 v1/v2 文件原子迁移为包含 desired/applied 快照及版本的 v3 封装，以现有内容作为已应用基线。配置 ZIP 仍为 v2；旧 Route API 保留为 Docker bind 和现有客户端的兼容适配层。

证书策略目前仍按全局 subject 管理，尚未成为按 Listener 绑定的独立资源。Docker 发现的服务身份也仍是运行时输入，而不是持久化的一等资源。

## 当前 UI 状态含义

- Azure `Enabled: No` 表示 Azure 协调器代码可用，但当前配置未启用。
- Azure `Configured: No` 表示缺少订阅、资源组、DNS Zone 或 NSG 名称等必要设置。
- 本地预览中的 Docker `Active: No` 通常表示预览启动时设置了 `GATEWAY_DOCKER_ENABLED=false`，或者未挂载 Docker socket。
- 独立网关 VM 使用显式私网后端路由并刻意关闭本地发现，因此 Docker `Active: No` 属于预期状态。

## 建议的下一里程碑

生产上线前先关闭上方的依赖安全发布门槛，再推进 Entra ID/OIDC 管理认证和 CI 化 E2E 覆盖。网关现在已经具备运维闭环：部署容器、绑定路由、协调网络状态、获取 HTTPS、审计变更，并在 UI 中显示健康/错误状态。
