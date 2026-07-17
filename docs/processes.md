# 部署流程

本文档描述 [`deploy/vm/deploy.sh`](../deploy/vm/deploy.sh) 的实际控制流。脚本提供同机容器和独立 Azure VM 两种模式；同目录的三个 Docker Compose 文件是独立部署入口，不参与本脚本执行。

## 总体流程

```mermaid
flowchart TD
    A["启动 deploy.sh"] --> B{"Bash 版本不低于 4?"}
    B -- 否 --> X["报错退出"]
    B -- 是 --> C["读取环境变量和默认值"]
    C --> D["注册 EXIT、ERR、INT、TERM trap"]
    D --> E["校验镜像和回滚设置"]
    E --> F{"DEPLOY_MODE 是否指定?"}

    F -- 否 --> G["交互选择部署模式"]
    F -- local --> L["同机容器部署"]
    F -- azure-vm --> Z["独立 Azure VM 部署"]
    G -- 同机容器 --> L
    G -- Azure VM --> Z

    L --> L1["检测或修复 Docker"]
    L1 --> L2["校验本地设置并确认"]
    L2 --> L3["调用 start.sh start"]
    L3 --> S["部署成功"]

    Z --> Z1["检查 Azure CLI 并登录"]
    Z1 --> Z2["交互收集 Azure 配置和认证方式"]
    Z2 --> Z3["检查资源冲突并确认"]
    Z3 --> Z4["创建网络、VM 和网关"]
    Z4 --> Z5["等待网关健康"]
    Z5 --> S

    S --> O["输出连接方式和后续操作"]
    O --> Q["清理临时文件并退出"]
```

## 同机容器部署

```mermaid
flowchart TD
    A["进入 local 模式"] --> B{"docker 命令存在?"}

    B -- 否 --> C{"主机存在 apt-get?"}
    C -- 否 --> F["提示手工安装 Docker Engine"]
    C -- 是 --> D{"确认安装 docker.io?"}
    D -- 否 --> F
    D -- 是 --> E["安装 docker.io<br/>启用并启动 Docker"]
    E --> G

    B -- 是 --> G{"docker info 成功?"}
    G -- 是 --> H["Docker 可用"]
    G -- 否 --> I{"本地默认 context<br/>且服务未运行?"}

    I -- 是 --> J{"确认启动 Docker?"}
    J -- 否 --> F
    J -- 是 --> K["通过 systemctl 或 service 启动"]
    K --> G

    I -- 否 --> M{"标准 Docker socket<br/>返回 permission denied?"}
    M -- 否 --> F
    M -- 是 --> N["说明 docker 组具有主机 root 等级权限"]
    N --> P{"确认加入 docker 组?"}
    P -- 否 --> F
    P -- 是 --> R["usermod -aG docker"]
    R --> T{"临时 sg 组会话成功?"}
    T -- 是 --> G
    T -- 否 --> U["要求注销、重新登录后重试"]

    H --> V{"找到完整的现有 launcher?"}
    V -- 是 --> W["复用 start.sh"]
    V -- 否 --> Y["选择 launcher 安装目录"]
    Y --> Z["下载 start.sh、.env.example<br/>和 platform.example.json"]
    Z --> ZA["校验并原子安装 launcher"]

    W --> AB["校验容器名、端口和数据目录"]
    ZA --> AB
    AB --> AC["输入并校验已有 Docker networks"]
    AC --> AD["显示部署摘要"]
    AD --> AE{"确认部署?"}
    AE -- 否 --> F
    AE -- 是 --> AF["传递环境变量并执行 start.sh start"]
    AF --> AG["输出 stop 和 start 生命周期命令"]
```

同机模式不会创建或修改 Azure 资源、NSG、公网 IP 或 DNS。`DATA_DIR` 必须位于 `~/docker_files` 下，Console 只映射到主机回环地址。

## 独立 Azure VM 部署

```mermaid
flowchart TD
    A["进入 azure-vm 模式"] --> B["检查 Azure CLI 和本地工具"]
    B --> C{"Azure CLI 已登录?"}
    C -- 否 --> D["启动设备代码登录"]
    C -- 是 --> E
    D --> E["选择订阅和区域"]

    E --> F["解析 Ubuntu 镜像架构和 Hyper-V generation"]
    F --> G["输入 VM、资源组和管理员用户名"]
    G --> H["选择已有或新建 VNet"]
    H --> I["选择已有或新建非委派子网"]
    I --> J["选择 VM 规格并校验镜像兼容性"]
    J --> K["选择系统盘类型和容量"]
    K --> L{"选择 VM 管理员认证方式"}

    L -- 已有 SSH 公钥 --> M{"选择公钥来源"}
    M -- Azure SSH Key --> M1["读取 Azure SSH Public Key 资源"]
    M -- Key Vault --> M2["读取 secret 中的 OpenSSH 公钥"]
    M -- 本地文件 --> M3["读取已有本地 .pub 文件"]
    M1 --> P["使用 ssh-keygen 校验公钥"]
    M2 --> P
    M3 --> P

    L -- 新建 SSH key --> N["选择新的本地私钥路径<br/>拒绝覆盖已有文件"]
    N --> N1["记录待生成 Ed25519 密钥对"]

    L -- 密码 --> O["选择 password 认证<br/>脚本不读取或保存密码"]

    P --> Q["确定认证摘要"]
    N1 --> Q
    O --> Q
    Q --> R["检测操作者公网 IP"]
    R --> S["输入并校验 SSH 来源 CIDR"]
    S --> T["检查 VM、磁盘、NSG、PIP、NIC、VNet、Subnet 重名"]
    T --> U["显示完整部署摘要"]
    U --> V{"确认创建?"}
    V -- 否 --> X["退出，不创建 Azure 资源或新密钥"]

    V -- 是 --> W{"认证准备"}
    W -- 新建 SSH key --> W1["ssh-keygen 生成 Ed25519 密钥对<br/>询问可选 passphrase"]
    W -- 已有 SSH 公钥 --> W2["校验公钥并组装 SSH 认证参数"]
    W -- 密码 --> W3["组装 password 认证参数"]
    W1 --> W2

    W2 --> Y["设置 DEPLOYMENT_STARTED=true"]
    W3 --> Y
    Y --> Z["创建或复用资源组"]
    Z --> ZA["创建或复用 VNet 和 Subnet"]
    ZA --> ZB["创建 NSG<br/>允许 Internet 到 80/443<br/>允许指定 CIDR 到 22"]
    ZB --> ZC["创建 Standard 静态公网 IP"]
    ZC --> ZD["创建并关联 NIC"]
    ZD --> ZE["生成 cloud-init"]
    ZE --> ZF["调用 az vm create"]

    ZF --> ZG{"认证类型"}
    ZG -- SSH --> ZH["上传所选公钥"]
    ZG -- password --> ZI["Azure CLI 隐藏输入密码和确认值<br/>执行 Azure 密码规则校验"]
    ZH --> ZJ["创建带托管身份的 Ubuntu VM"]
    ZI --> ZJ

    ZJ --> ZK["cloud-init 安装 Docker<br/>生成网关令牌<br/>使用 host network 启动容器<br/>管理端仅绑定 127.0.0.1:8080"]
    ZK --> ZL["Azure Run Command 等待 cloud-init"]
    ZL --> ZM{"readyz 成功且容器 healthy?"}
    ZM -- 未就绪且未超时 --> ZL
    ZM -- 超时 --> FAIL["输出容器状态和日志<br/>进入失败处理"]
    ZM -- 是 --> ZN["读取管理员令牌、公网 IP、私网 IP 和 Principal ID"]
    ZN --> ZO["设置 DEPLOYMENT_SUCCEEDED=true"]
    ZO --> DONE["输出 SSH、管理隧道和手工后续步骤"]
```

密码由 Azure CLI 的隐藏提示读取，不会进入脚本变量或输出。新建密钥模式只把 `.pub` 文件发送给 Azure；私钥保留在本机，最终 SSH 和隧道命令会包含对应的 `-i` 参数。

VM 中的 cloud-init 会安装 `docker.io`、生成 Console 管理令牌、创建网关环境文件，并直接使用 `docker pull` 和 `docker run` 启动网关。该路径不使用 Docker Compose。

## 失败与回滚

```mermaid
flowchart TD
    A["命令失败、fail、Ctrl+C 或 TERM"] --> B{"Azure 资源创建已经开始?"}
    B -- 否 --> C["清理临时文件并退出"]
    B -- 是 --> D{"部署已经成功?"}
    D -- 是 --> C
    D -- 否 --> E{"ROLLBACK_ON_ERROR=true?"}

    E -- 否 --> F["保留已创建资源并退出"]
    E -- 是 --> G["删除本次尝试创建的 VM 和系统盘"]
    G --> H["删除本次创建的 NIC、公网 IP 和 NSG"]
    H --> I["删除本次新建的 Subnet 或 VNet"]
    I --> J["复查是否有资源残留"]
    J --> C
```

回滚只删除脚本标记为本次创建的资源。资源组、已有 VNet、已有 Subnet 和其他原有资源始终保留。新生成的本地 SSH 密钥也会保留，不属于 Azure 回滚范围。

## Docker Compose 的关系

`deploy.sh` 和 `start.sh` 都不调用 Docker Compose。同目录 Compose 文件由 Makefile 的独立目标使用：

| Compose 文件 | 独立入口 |
| --- | --- |
| `docker-compose.yml` | `make compose-up` |
| `docker-compose.socket-proxy.yml` | `make compose-up-proxy` |
| `docker-compose.production.yml` | `make compose-prod-up` |