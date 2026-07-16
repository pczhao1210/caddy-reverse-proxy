const supportedLocales = ['en', 'zh-CN'];
const defaultLocale = navigator.language && navigator.language.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en';

const messages = {
  en: {
    title: 'AI Docker Farm Gateway',
    'nav.primary': 'Primary',
    'nav.dashboard': 'Dashboard',
    'nav.routes': 'Routes',
    'nav.discovery': 'Discovery',
    'nav.network': 'Network',
    'language.label': 'Language',
    'actions.refresh': 'Refresh status',
    'actions.signOut': 'Sign out',
    'actions.apply': 'Reconcile now',
    'actions.add': 'Add',
    'actions.continue': 'Continue',
    'actions.delete': 'Delete',
    'actions.cancel': 'Cancel',
    'actions.confirmDelete': 'Delete route',
    'actions.bind': 'Bind',
    'actions.useSuggestedRoute': 'Use Suggested Route',
    'actions.requestRefresh': 'Reload TLS config',
    'actions.saveAndApply': 'Save and apply',
    'app.heading': 'Gateway Control Plane',
    'app.loading': 'Loading current state',
    'app.subtitle': 'Profile {profile} · Updated {time}',
    'metrics.profile': 'Profile',
    'metrics.routes': 'Saved routes',
    'metrics.docker': 'Docker',
    'metrics.caddy': 'Caddy',
    'sections.recentReconcile': 'Recent Reconcile',
    'sections.activeRoutes': 'Saved Routes',
    'sections.addRoute': 'Add Route',
    'sections.dockerDiscovery': 'Docker Discovery',
    'sections.azureNetwork': 'Azure & Network',
    'sections.runtimeSecurity': 'Runtime & Security',
    'sections.certificates': 'Certificates',
    'tables.host': 'Host',
    'tables.path': 'Path',
    'tables.exposure': 'Exposure',
    'tables.source': 'Source',
    'tables.upstream': 'Upstream',
    'tables.health': 'Health',
    'tables.https': 'HTTPS',
    'tables.container': 'Container',
    'tables.image': 'Image',
    'tables.status': 'Status',
    'tables.ports': 'Ports',
    'tables.bind': 'Bind',
    'forms.host': 'Host',
    'forms.pathPrefix': 'Path prefix',
    'forms.upstreamUrl': 'Upstream URL',
    'forms.upstreamScheme': 'Upstream protocol',
    'forms.healthPath': 'Health path',
    'forms.exposure': 'Exposure',
    'forms.https': 'Inbound HTTPS',
    'forms.websocket': 'WebSocket',
    'forms.certificateIssuer': 'Issuer',
    'forms.certificateEmail': 'ACME account email',
    'forms.certificateStaging': 'Use staging CA',
    'forms.caDirectory': 'CA directory URL',
    'forms.certificateSubjects': 'Certificate subjects',
    'forms.dnsProvider': 'DNS challenge provider',
    'forms.azureSubscriptionId': 'Azure subscription ID',
    'forms.azureResourceGroup': 'DNS zone resource group',
    'forms.azureAuthentication': 'Azure authentication',
    'forms.azureTenantId': 'Tenant ID',
    'forms.azureClientId': 'Client ID',
    'forms.azureClientSecret': 'Client secret',
    'auth.signIn': 'Sign in',
    'auth.adminToken': 'Admin token',
    'status.active': 'Active',
    'status.disabled': 'Disabled',
    'status.unavailable': 'Unavailable',
    'status.loaded': 'Loaded',
    'status.pending': 'Pending',
    'status.failed': 'Failed',
    'status.notRun': 'Not reconciled',
    'status.inactive': 'Not active',
    'status.on': 'On',
    'status.off': 'Off',
    'status.yes': 'Yes',
    'status.no': 'No',
    'status.none': 'None',
    'status.healthy': 'Healthy',
    'status.unhealthy': 'Unhealthy',
    'status.unknown': 'Unknown',
    'status.manual': 'Manual',
    'status.noTcpPort': 'No TCP port',
    'status.selectPort': 'Select a port',
    'status.runtimeOnly': 'Runtime',
    'status.persisted': 'Persisted',
    'status.running': 'Running',
    'status.paused': 'Paused',
    'status.restarting': 'Restarting',
    'status.exited': 'Exited',
    'status.created': 'Created',
    'status.dead': 'Stopped',
    'empty.noRoutes': 'No routes',
    'empty.noContainers': 'No workload containers discovered',
    'exposure.public': 'Public',
    'exposure.protected': 'Protected',
    'exposure.internal': 'Internal',
    'certificate.default': 'Caddy default',
    'certificate.letsencrypt': "Let's Encrypt",
    'certificate.zerossl': 'ZeroSSL',
    'certificate.custom': 'Custom ACME',
    'certificate.dnsNone': 'None (HTTP-01 / TLS-ALPN-01)',
    'certificate.dnsAzure': 'Azure DNS',
    'certificate.managedIdentity': 'Managed Identity',
    'certificate.appRegistration': 'App Registration',
    'certificate.secretConfigured': 'Configured',
    'certificate.persistedNote': 'Settings are persisted before Caddy is reloaded and survive container restarts.',
    'details.started': 'Started',
    'details.finished': 'Finished',
    'details.explicit': 'Explicit',
    'details.discovered': 'Discovered',
    'details.applied': 'Applied',
    'details.healthChecks': 'Health Checks',
    'details.unhealthyRoutes': 'Unhealthy Routes',
    'details.azureDns': 'Azure DNS',
    'details.dnsDeleted': 'DNS Deleted',
    'details.azureNsg': 'Azure NSG',
    'details.nsgDeleted': 'NSG Deleted',
    'details.error': 'Error',
    'details.active': 'Active',
    'details.enabled': 'Enabled',
    'details.profile': 'Profile',
    'details.socket': 'Socket',
    'details.endpoint': 'Endpoint',
    'details.reason': 'Reason',
    'details.nextActions': 'Next Actions',
    'details.configured': 'Configured',
    'details.manageDns': 'Manage DNS',
    'details.manageNsg': 'Manage NSG',
    'details.mode': 'Mode',
    'details.capabilities': 'Capabilities',
    'details.missingSettings': 'Missing Settings',
    'details.publicIp': 'Public IP',
    'details.dnsUpserts': 'DNS Upserts',
    'details.nsgUpserts': 'NSG Upserts',
    'details.warnings': 'Warnings',
    'details.certificateIssuer': 'Certificate Issuer',
    'details.certificateEmail': 'Certificate Email',
    'details.certificateStaging': 'Certificate Staging',
    'details.caDirectory': 'CA Directory',
    'details.certificateSubjects': 'Certificate Subjects',
    'details.dnsProvider': 'DNS Challenge',
    'details.healthEnabled': 'Health Checks',
    'details.healthTimeout': 'Health Timeout',
    'details.securityEnabled': 'Security Baseline',
    'details.securityBodyLimit': 'Request Body Limit',
    'details.securityDeniedMethods': 'Denied Methods',
    'details.securityDeniedPaths': 'Denied Paths',
    'details.securityAllowedCidrs': 'Allowed CIDRs',
    'details.securityBlockedCidrs': 'Blocked CIDRs',
    'details.auditEnabled': 'Audit Log',
    'details.auditFile': 'Audit File',
    'source.explicit': 'Explicit',
    'source.docker': 'Docker',
    'source.management': 'Management',
    'aria.routeHost': 'Route host',
    'aria.upstreamPort': 'Upstream port',
    'aria.exposure': 'Exposure',
    'msg.certificateSaved': 'Certificate policy saved and Caddy reloaded',
    'msg.certificateSaveFailed': 'Certificate policy was saved, but Caddy reload failed: {error}',
    'msg.certificateRefreshed': 'TLS configuration reloaded',
    'msg.certificateRefreshFailed': 'TLS configuration reload failed: {error}',
    'msg.unsavedCertificate': 'Save or discard certificate changes before reloading TLS configuration',
    'msg.routeSaved': 'Route saved and applied',
    'msg.routeSaveFailed': 'Route was saved, but reconcile failed: {error}',
    'msg.routeDeleted': 'Route deleted and configuration reconciled',
    'msg.routeDeleteFailed': 'Route was deleted, but reconcile failed: {error}',
    'msg.routeBound': 'Container route bound and applied',
    'msg.routeBindFailed': 'Container route was saved, but reconcile failed: {error}',
    'msg.reconcileComplete': 'Configuration reconciled successfully',
    'msg.reconcileFailed': 'Reconcile failed: {error}',
    'msg.caddyNotLoaded': 'Caddy did not load the generated configuration',
    'msg.confirmDeleteRoute': 'Delete the saved route for {host}?',
    'msg.invalidAdminToken': 'The admin token is invalid',
    'msg.selectContainerPort': 'Select an upstream port before binding',
    'msg.wildcardRequiresDNS': 'Wildcard certificate subjects require a DNS challenge provider',
    'msg.customCARequired': 'Custom ACME issuer requires a CA directory URL',
    'msg.azureDisabled': 'Azure managers are disabled',
    'msg.enableAzure': 'Enable Azure integration',
    'msg.assignManagedIdentityRoles': 'Assign managed identity roles',
    'msg.setAzureIdentifiers': 'Set subscription, resource group, DNS zone, and NSG names',
    'msg.runApplyAzure': 'Run Apply to reconcile DNS records and NSG rules',
    'msg.dockerDisabled': 'Docker discovery is disabled by configuration or GATEWAY_DOCKER_ENABLED',
    'msg.enableDockerDiscovery': 'Use explicit routes, or set GATEWAY_DOCKER_ENABLED=true for local workloads',
    'msg.mountDockerSocket': 'Mount /var/run/docker.sock read-only or provide a Docker socket proxy when discovery is enabled',
    'msg.dockerNotInitialized': 'Docker discovery is configured but the discoverer was not initialized',
    'msg.checkStartupLogs': 'Check the gateway startup logs',
    'msg.verifyDockerSocketPath': 'Verify the Docker socket path is reachable from the container',
    'msg.dockerActive': 'Docker discovery is active',
    'msg.addDockerLabels': 'Add caddy.enable=true, caddy.host, and caddy.port labels to workload containers',
    'msg.bindUnknownGatewayNetwork': 'Gateway network visibility is unavailable, so Bind keeps the current direct-upstream behavior.',
    'msg.bindHostNetwork': 'This container uses Docker host networking. Add an explicit route to {upstream}; use {loopback} only if the gateway itself runs in host mode.',
    'msg.bindPublishedPort': 'This container is not on a gateway network. Add an explicit route to {upstream}, or attach it to one of the gateway networks: {networks}.',
    'msg.bindBridgeUnreachable': 'This container is only on Docker bridge and is not directly reachable from the gateway. Attach it to one of the gateway networks: {networks}.',
    'msg.bindNetworkUnreachable': 'This container does not share a network with the gateway. Attach it to one of the gateway networks: {networks}.',
    'msg.routePrefilled': 'Suggested route populated in Add Route',
    'msg.noPublicRoutesAzure': 'no public routes require Azure reconciliation',
    'msg.managedDnsNoRelativeName': 'managed DNS record without a relative name was skipped',
    'msg.nsgSourcePolicy': 'NSG priority and source-prefix policy',
    'msg.setMissingAzureSettings': 'Set the missing Azure settings, including DNS zones and the ingress public IP',
    'msg.runReconcileEnabledAzure': 'Run Reconcile now to update the enabled Azure resources',
    'msg.azureDefaultCredential': 'Azure default credential chain',
    'msg.azureDnsCapability': 'Azure DNS A record reconciliation',
    'msg.azureNsgCapability': 'VM NSG inbound rule reconciliation',
    'msg.gatewayCannotBind': 'The gateway container cannot be bound as an upstream',
    'msg.invalidUpstreamScheme': 'Upstream protocol must be HTTP or HTTPS'
  },
  'zh-CN': {
    title: 'AI Docker Farm 网关',
    'nav.primary': '主导航',
    'nav.dashboard': '仪表盘',
    'nav.routes': '路由',
    'nav.discovery': '发现',
    'nav.network': '网络',
    'language.label': '语言',
    'actions.refresh': '刷新状态',
    'actions.signOut': '退出登录',
    'actions.apply': '立即协调',
    'actions.add': '添加',
    'actions.continue': '继续',
    'actions.delete': '删除',
    'actions.cancel': '取消',
    'actions.confirmDelete': '删除路由',
    'actions.bind': '绑定',
    'actions.useSuggestedRoute': '使用建议路由',
    'actions.requestRefresh': '重新加载 TLS 配置',
    'actions.saveAndApply': '保存并应用',
    'app.heading': '网关控制平面',
    'app.loading': '正在加载当前状态',
    'app.subtitle': '配置档 {profile} · 更新于 {time}',
    'metrics.profile': '配置档',
    'metrics.routes': '已保存路由',
    'metrics.docker': 'Docker',
    'metrics.caddy': 'Caddy',
    'sections.recentReconcile': '最近一次协调',
    'sections.activeRoutes': '已保存路由',
    'sections.addRoute': '添加路由',
    'sections.dockerDiscovery': 'Docker 发现',
    'sections.azureNetwork': 'Azure 与网络',
    'sections.runtimeSecurity': '运行与安全',
    'sections.certificates': '证书',
    'tables.host': '主机名',
    'tables.path': '路径',
    'tables.exposure': '暴露方式',
    'tables.source': '来源',
    'tables.upstream': '上游',
    'tables.health': '健康',
    'tables.https': 'HTTPS',
    'tables.container': '容器',
    'tables.image': '镜像',
    'tables.status': '状态',
    'tables.ports': '端口',
    'tables.bind': '绑定',
    'forms.host': '主机名',
    'forms.pathPrefix': '路径前缀',
    'forms.upstreamUrl': '上游 URL',
    'forms.upstreamScheme': '上游协议',
    'forms.healthPath': '健康检查路径',
    'forms.exposure': '暴露方式',
    'forms.https': '入口 HTTPS',
    'forms.websocket': 'WebSocket',
    'forms.certificateIssuer': '签发器',
    'forms.certificateEmail': 'ACME 账户邮箱',
    'forms.certificateStaging': '使用测试 CA',
    'forms.caDirectory': 'CA Directory URL',
    'forms.certificateSubjects': '证书域名',
    'forms.dnsProvider': 'DNS Challenge 提供商',
    'forms.azureSubscriptionId': 'Azure 订阅 ID',
    'forms.azureResourceGroup': 'DNS Zone 资源组',
    'forms.azureAuthentication': 'Azure 认证方式',
    'forms.azureTenantId': '租户 ID',
    'forms.azureClientId': '客户端 ID',
    'forms.azureClientSecret': '客户端密钥',
    'auth.signIn': '登录',
    'auth.adminToken': '管理员令牌',
    'status.active': '活动',
    'status.disabled': '已禁用',
    'status.unavailable': '不可用',
    'status.loaded': '已加载',
    'status.pending': '待处理',
    'status.failed': '失败',
    'status.notRun': '尚未协调',
    'status.inactive': '未生效',
    'status.on': '开',
    'status.off': '关',
    'status.yes': '是',
    'status.no': '否',
    'status.none': '无',
    'status.healthy': '健康',
    'status.unhealthy': '异常',
    'status.unknown': '未知',
    'status.manual': '手动',
    'status.noTcpPort': '没有 TCP 端口',
    'status.selectPort': '请选择端口',
    'status.runtimeOnly': '运行时',
    'status.persisted': '已持久化',
    'status.running': '运行中',
    'status.paused': '已暂停',
    'status.restarting': '正在重启',
    'status.exited': '已退出',
    'status.created': '已创建',
    'status.dead': '已停止',
    'empty.noRoutes': '没有路由',
    'empty.noContainers': '未发现工作负载容器',
    'exposure.public': '公开',
    'exposure.protected': '受保护',
    'exposure.internal': '内部',
    'certificate.default': 'Caddy 默认',
    'certificate.letsencrypt': "Let's Encrypt",
    'certificate.zerossl': 'ZeroSSL',
    'certificate.custom': '自定义 ACME',
    'certificate.dnsNone': '无（HTTP-01 / TLS-ALPN-01）',
    'certificate.dnsAzure': 'Azure DNS',
    'certificate.managedIdentity': '托管身份',
    'certificate.appRegistration': 'App Registration',
    'certificate.secretConfigured': '已配置',
    'certificate.persistedNote': '设置会先持久化，再重新加载 Caddy，并在容器重启后保留。',
    'details.started': '开始时间',
    'details.finished': '完成时间',
    'details.explicit': '显式路由',
    'details.discovered': '发现路由',
    'details.applied': '已应用',
    'details.healthChecks': '健康检查',
    'details.unhealthyRoutes': '异常路由',
    'details.azureDns': 'Azure DNS',
    'details.dnsDeleted': 'DNS 已删除',
    'details.azureNsg': 'Azure NSG',
    'details.nsgDeleted': 'NSG 已删除',
    'details.error': '错误',
    'details.active': '活动',
    'details.enabled': '启用',
    'details.profile': '配置档',
    'details.socket': '套接字',
    'details.endpoint': '端点',
    'details.reason': '原因',
    'details.nextActions': '下一步操作',
    'details.configured': '已配置',
    'details.manageDns': '管理 DNS',
    'details.manageNsg': '管理 NSG',
    'details.mode': '模式',
    'details.capabilities': '能力',
    'details.missingSettings': '缺失设置',
    'details.publicIp': '公网 IP',
    'details.dnsUpserts': 'DNS 写入',
    'details.nsgUpserts': 'NSG 写入',
    'details.warnings': '警告',
    'details.certificateIssuer': '证书签发器',
    'details.certificateEmail': '证书邮箱',
    'details.certificateStaging': '证书测试模式',
    'details.caDirectory': 'CA Directory',
    'details.certificateSubjects': '证书域名',
    'details.dnsProvider': 'DNS Challenge',
    'details.healthEnabled': '健康检查',
    'details.healthTimeout': '健康超时',
    'details.securityEnabled': '安全基线',
    'details.securityBodyLimit': '请求体上限',
    'details.securityDeniedMethods': '拒绝方法',
    'details.securityDeniedPaths': '拒绝路径',
    'details.securityAllowedCidrs': '允许 CIDR',
    'details.securityBlockedCidrs': '阻止 CIDR',
    'details.auditEnabled': '审计日志',
    'details.auditFile': '审计文件',
    'source.explicit': '显式',
    'source.docker': 'Docker',
    'source.management': '管理',
    'aria.routeHost': '路由主机名',
    'aria.upstreamPort': '上游端口',
    'aria.exposure': '暴露方式',
    'msg.certificateSaved': '证书策略已保存，Caddy 已重新加载',
    'msg.certificateSaveFailed': '证书策略已保存，但 Caddy 重新加载失败：{error}',
    'msg.certificateRefreshed': 'TLS 配置已重新加载',
    'msg.certificateRefreshFailed': 'TLS 配置重新加载失败：{error}',
    'msg.unsavedCertificate': '请先保存或放弃证书改动，再重新加载 TLS 配置',
    'msg.routeSaved': '路由已保存并应用',
    'msg.routeSaveFailed': '路由已保存，但协调失败：{error}',
    'msg.routeDeleted': '路由已删除，配置已协调',
    'msg.routeDeleteFailed': '路由已删除，但协调失败：{error}',
    'msg.routeBound': '容器路由已绑定并应用',
    'msg.routeBindFailed': '容器路由已保存，但协调失败：{error}',
    'msg.reconcileComplete': '配置协调成功',
    'msg.reconcileFailed': '协调失败：{error}',
    'msg.caddyNotLoaded': 'Caddy 未加载生成的配置',
    'msg.confirmDeleteRoute': '确定删除 {host} 的已保存路由吗？',
    'msg.invalidAdminToken': '管理员令牌无效',
    'msg.selectContainerPort': '绑定前请选择上游端口',
    'msg.wildcardRequiresDNS': '通配符证书域名需要 DNS Challenge 提供商',
    'msg.customCARequired': '自定义 ACME 签发器需要 CA Directory URL',
    'msg.azureDisabled': 'Azure 管理器已禁用',
    'msg.enableAzure': '启用 Azure 集成',
    'msg.assignManagedIdentityRoles': '为托管身份分配角色',
    'msg.setAzureIdentifiers': '设置订阅、资源组、DNS Zone 和 NSG 名称',
    'msg.runApplyAzure': '运行“应用”以协调 DNS 记录和 NSG 规则',
    'msg.dockerDisabled': 'Docker 自动发现已被配置或 GATEWAY_DOCKER_ENABLED 禁用',
    'msg.enableDockerDiscovery': '使用显式路由，或为本机工作负载设置 GATEWAY_DOCKER_ENABLED=true',
    'msg.mountDockerSocket': '启用发现时，以只读方式挂载 /var/run/docker.sock，或提供 Docker socket 代理',
    'msg.dockerNotInitialized': 'Docker 自动发现已配置，但 discoverer 未初始化',
    'msg.checkStartupLogs': '检查网关启动日志',
    'msg.verifyDockerSocketPath': '确认容器内可访问 Docker socket 路径',
    'msg.dockerActive': 'Docker 自动发现处于活动状态',
    'msg.addDockerLabels': '为工作负载容器添加 caddy.enable=true、caddy.host 和 caddy.port 标签',
    'msg.bindUnknownGatewayNetwork': '当前无法识别 gateway 自身所在网络，因此“绑定”仍会沿用现有直接上游行为。',
    'msg.bindHostNetwork': '该容器使用 Docker host 网络。请添加显式路由到 {upstream}；只有当 gateway 自己也运行在 host 模式时，才可使用 {loopback}。',
    'msg.bindPublishedPort': '该容器不在 gateway 可直达的网络中。请添加显式路由到 {upstream}，或把容器加入以下 gateway 网络之一：{networks}。',
    'msg.bindBridgeUnreachable': '该容器只在 Docker bridge 上，gateway 不能直接访问。请把它加入以下 gateway 网络之一：{networks}。',
    'msg.bindNetworkUnreachable': '该容器与 gateway 没有共享网络。请把它加入以下 gateway 网络之一：{networks}。',
    'msg.routePrefilled': '已在“添加路由”中填入建议上游',
    'msg.noPublicRoutesAzure': '没有公网路由需要 Azure 协调',
    'msg.managedDnsNoRelativeName': '已跳过缺少相对名称的托管 DNS 记录',
    'msg.nsgSourcePolicy': 'NSG 优先级和源地址前缀策略',
    'msg.setMissingAzureSettings': '设置缺失的 Azure 配置，包括 DNS Zone 和入口公网 IP',
    'msg.runReconcileEnabledAzure': '运行“立即协调”以更新已启用的 Azure 资源',
    'msg.azureDefaultCredential': 'Azure 默认凭据链',
    'msg.azureDnsCapability': 'Azure DNS A 记录协调',
    'msg.azureNsgCapability': '虚拟机 NSG 入站规则协调',
    'msg.gatewayCannotBind': '不能将网关容器绑定为上游',
    'msg.invalidUpstreamScheme': '上游协议必须是 HTTP 或 HTTPS'
  }
};

const backendMessageKeys = new Map([
  ['Azure managers are disabled', 'msg.azureDisabled'],
  ['Enable Azure integration', 'msg.enableAzure'],
  ['Assign managed identity roles', 'msg.assignManagedIdentityRoles'],
  ['Assign the required managed identity roles', 'msg.assignManagedIdentityRoles'],
  ['Set subscription, resource group, DNS zone, and NSG names', 'msg.setAzureIdentifiers'],
  ['Set the missing Azure settings, including DNS zones and the ingress public IP', 'msg.setMissingAzureSettings'],
  ['Run Apply to reconcile DNS records and NSG rules', 'msg.runApplyAzure'],
  ['Run Apply to reconcile the enabled Azure resources', 'msg.runReconcileEnabledAzure'],
  ['DefaultAzureCredential', 'msg.azureDefaultCredential'],
  ['Azure DNS A record reconciliation', 'msg.azureDnsCapability'],
  ['VM NSG inbound rule reconciliation', 'msg.azureNsgCapability'],
  ['Docker discovery is disabled by configuration or GATEWAY_DOCKER_ENABLED', 'msg.dockerDisabled'],
  ['Use explicit routes, or set GATEWAY_DOCKER_ENABLED=true for local workloads', 'msg.enableDockerDiscovery'],
  ['Mount /var/run/docker.sock read-only or provide a Docker socket proxy when discovery is enabled', 'msg.mountDockerSocket'],
  ['Docker discovery is configured but the discoverer was not initialized', 'msg.dockerNotInitialized'],
  ['Check the gateway startup logs', 'msg.checkStartupLogs'],
  ['Verify the Docker socket path is reachable from the container', 'msg.verifyDockerSocketPath'],
  ['Docker discovery is active', 'msg.dockerActive'],
  ['Add caddy.enable=true, caddy.host, and caddy.port labels to workload containers', 'msg.addDockerLabels'],
  ['no public routes require Azure reconciliation', 'msg.noPublicRoutesAzure'],
  ['managed DNS record without a relative name was skipped', 'msg.managedDnsNoRelativeName'],
  ['NSG priority and source-prefix policy', 'msg.nsgSourcePolicy'],
  ['the gateway container cannot be bound as an upstream', 'msg.gatewayCannotBind'],
  ['upstream scheme must be http or https', 'msg.invalidUpstreamScheme']
]);

document.addEventListener('alpine:init', () => {
  Alpine.data('gatewayApp', () => ({
    token: localStorage.getItem('gatewayToken') || '',
    loginToken: '',
    loginError: '',
    locale: localStorage.getItem('gatewayLocale') || defaultLocale,
    activeView: 'dashboard',
    alert: '',
    notice: '',
    lastUpdated: null,
    isRefreshing: false,
    isActing: false,
    certificateDirty: false,
    routeToDelete: null,
    status: null,
    containers: [],
    discoveryWarning: '',
    bindForms: {},
    certificateForm: emptyCertificateForm(),
    routeForm: emptyRouteForm(),
    navItems: [
      { view: 'dashboard', label: 'nav.dashboard' },
      { view: 'routes', label: 'nav.routes' },
      { view: 'discovery', label: 'nav.discovery' },
      { view: 'network', label: 'nav.network' }
    ],

    init() {
      if (!supportedLocales.includes(this.locale)) this.locale = 'en';
      this.loginToken = this.token;
      this.applyLocale();
      this.$watch('locale', () => this.applyLocale());
      this.$nextTick(() => {
        if (!this.token) this.openLogin();
        else this.refreshAll({ forceCertificate: true });
      });
    },

    applyLocale() {
      localStorage.setItem('gatewayLocale', this.locale);
      document.documentElement.lang = this.locale;
      document.title = this.t('title');
    },

    async login() {
      const token = this.loginToken.trim();
      if (!token || this.isActing) return;
      this.loginError = '';
      this.token = token;
      this.isActing = true;
      try {
        await this.api('/api/status');
        localStorage.setItem('gatewayToken', token);
        this.closeLogin();
        await this.refreshAll({ forceCertificate: true });
      } catch (error) {
        this.token = '';
        localStorage.removeItem('gatewayToken');
        this.loginError = error.status === 401 ? this.t('msg.invalidAdminToken') : this.translateBackendText(error.message);
      } finally {
        this.isActing = false;
      }
    },

    signOut() {
      localStorage.removeItem('gatewayToken');
      this.token = '';
      this.loginToken = '';
      this.loginError = '';
      this.status = null;
      this.containers = [];
      this.discoveryWarning = '';
      this.bindForms = {};
      this.certificateForm = emptyCertificateForm();
      this.certificateDirty = false;
      this.lastUpdated = null;
      this.activeView = 'dashboard';
      this.clearMessages();
      this.openLogin();
    },

    async refreshAll(options = {}) {
      if (this.isRefreshing) return;
      this.isRefreshing = true;
      try {
        this.clearMessages();
        const [statusResult, discoveryResult, certificateResult] = await Promise.allSettled([
          this.api('/api/status'),
          this.api('/api/discovery/containers'),
          this.api('/api/certificate')
        ]);
        const errors = [];
        if (statusResult.status === 'fulfilled') {
          this.status = statusResult.value;
        } else {
          errors.push(statusResult.reason);
        }
        if (discoveryResult.status === 'fulfilled') {
          const discovery = discoveryResult.value;
          this.discoveryWarning = discovery.warning || '';
          this.containers = discovery.containers || [];
          this.containers.forEach((container) => this.ensureBindForm(container));
        } else {
          errors.push(discoveryResult.reason);
        }
        if (certificateResult.status === 'fulfilled') {
          if (!this.certificateDirty || options.forceCertificate) this.setCertificateForm(certificateResult.value);
        } else {
          errors.push(certificateResult.reason);
        }
        this.lastUpdated = new Date();
        if (errors.length > 0) {
          const authError = errors.find((error) => error.status === 401 || error.status === 503);
          if (authError) {
            this.token = '';
            localStorage.removeItem('gatewayToken');
            this.loginError = authError.status === 401 ? this.t('msg.invalidAdminToken') : this.translateBackendText(authError.message);
            this.openLogin();
          } else {
            this.showAlert(errors.map((error) => this.translateBackendText(error.message)).join('; '));
          }
        }
      } finally {
        this.isRefreshing = false;
      }
    },

    async reconcile() {
      await this.runAction(async () => {
        const result = await this.api('/api/reconcile', { method: 'POST' });
        await this.refreshAll();
        this.showReconcileOutcome(result, 'msg.reconcileComplete', 'msg.reconcileFailed');
      });
    },

    async saveRoute() {
      const route = {
        host: this.routeForm.host,
        pathPrefix: this.routeForm.pathPrefix,
        exposure: this.routeForm.exposure,
        enabled: true,
        https: this.routeForm.https,
        source: 'explicit',
        upstreams: [{ name: 'primary', url: this.routeForm.upstream, healthPath: this.routeForm.healthPath }]
      };
      await this.runAction(async () => {
        const result = await this.api('/api/routes', { method: 'POST', body: JSON.stringify(route) });
        this.routeForm = emptyRouteForm();
        await this.refreshAll();
        this.showReconcileOutcome(result.reconcile, 'msg.routeSaved', 'msg.routeSaveFailed');
      });
    },

    requestDeleteRoute(route) {
      this.routeToDelete = route;
      const dialog = this.$refs.deleteRouteDialog;
      if (dialog && !dialog.open) dialog.showModal();
    },

    cancelDeleteRoute() {
      const dialog = this.$refs.deleteRouteDialog;
      if (dialog && dialog.open) dialog.close();
      this.routeToDelete = null;
    },

    async confirmDeleteRoute() {
      const route = this.routeToDelete;
      this.cancelDeleteRoute();
      if (route) await this.deleteRoute(route.id);
    },

    async deleteRoute(id) {
      await this.runAction(async () => {
        const result = await this.api('/api/routes/' + encodeURIComponent(id), { method: 'DELETE' });
        await this.refreshAll();
        this.showReconcileOutcome(result.reconcile, 'msg.routeDeleted', 'msg.routeDeleteFailed');
      });
    },

    async bindContainer(container) {
      if (!this.canBindContainer(container)) {
		this.showAlert(this.bindHint(container));
		return;
	  }
      const form = this.bindForms[container.id];
      const payload = {
        containerId: container.id,
        host: form.host,
        port: Number(form.port),
        scheme: form.scheme,
        exposure: form.exposure,
        https: form.https
      };
      await this.runAction(async () => {
        const result = await this.api('/api/discovery/bind', { method: 'POST', body: JSON.stringify(payload) });
        await this.refreshAll();
        this.showReconcileOutcome(result.reconcile, 'msg.routeBound', 'msg.routeBindFailed');
      });
    },

    async saveCertificate() {
      if (this.certificateForm.issuer === 'custom' && !this.certificateForm.caDirectory) {
        this.showAlert(this.t('msg.customCARequired'));
        return;
      }
      const subjects = this.certificateForm.subjectsText.split(/[\s,]+/).filter(Boolean);
      if (subjects.some((subject) => subject.startsWith('*.')) && !this.certificateForm.dnsProvider) {
        this.showAlert(this.t('msg.wildcardRequiresDNS'));
        return;
      }
      const payload = {
        issuer: this.certificateForm.issuer,
        email: this.certificateForm.email,
        staging: this.certificateForm.staging,
        caDirectory: this.certificateForm.caDirectory,
        subjects,
        dnsChallenge: {
          provider: this.certificateForm.dnsProvider,
          azure: {
            subscriptionId: this.certificateForm.azureSubscriptionId,
            resourceGroup: this.certificateForm.azureResourceGroup,
            authentication: this.certificateForm.azureAuthentication,
            tenantId: this.certificateForm.azureTenantId,
            clientId: this.certificateForm.azureClientId,
            clientSecret: this.certificateForm.azureClientSecret
          }
        }
      };
      await this.runAction(async () => {
        const result = await this.api('/api/certificate', { method: 'PUT', body: JSON.stringify(payload) });
        this.setCertificateForm(result.certificate);
        await this.refreshAll({ forceCertificate: true });
        this.showReconcileOutcome(result.reconcile, 'msg.certificateSaved', 'msg.certificateSaveFailed');
      });
    },

    async refreshCertificate() {
      if (this.certificateDirty) {
        this.showAlert(this.t('msg.unsavedCertificate'));
        return;
      }
      await this.runAction(async () => {
        const result = await this.api('/api/certificate/refresh', { method: 'POST' });
        this.setCertificateForm(result.certificate);
        await this.refreshAll({ forceCertificate: true });
        this.showReconcileOutcome(result.reconcile, 'msg.certificateRefreshed', 'msg.certificateRefreshFailed');
      });
    },

    async runAction(action) {
      if (this.isActing) return;
      this.isActing = true;
      try {
        this.clearMessages();
        await action();
      } catch (error) {
        this.showAlert(error.message);
        if (error.status === 401 || error.status === 503) {
          this.token = '';
          localStorage.removeItem('gatewayToken');
          this.loginError = error.status === 401 ? this.t('msg.invalidAdminToken') : this.translateBackendText(error.message);
          this.openLogin();
        }
      } finally {
        this.isActing = false;
      }
    },

    async api(path, options = {}) {
      const response = await fetch(path, {
        ...options,
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + this.token, ...(options.headers || {}) }
      });
      const text = await response.text();
      let payload = null;
      if (text) {
        try {
          payload = JSON.parse(text);
        } catch {
          const error = new Error(response.ok ? 'server returned an invalid JSON response' : text);
          error.status = response.status;
          throw error;
        }
      }
      if (!response.ok) {
        const error = new Error(payload?.error || response.statusText);
        error.status = response.status;
        throw error;
      }
      return payload;
    },

    showReconcileOutcome(result, successKey, failureKey) {
      const error = this.reconcileError(result);
      if (error) {
        this.showAlert(this.format(failureKey, { error }));
      } else {
        this.showNotice(this.t(successKey));
      }
    },

    reconcileError(result) {
      if (result?.error || result?.azure?.error) return this.translateBackendText(result.error || result.azure.error);
      if (!result?.caddyLoaded) return this.t('msg.caddyNotLoaded');
      return '';
    },

    setCertificateForm(certificate) {
      const source = certificate || {};
      const dns = source.dnsChallenge || {};
      const azure = dns.azure || {};
      this.certificateForm = {
        ...emptyCertificateForm(),
        issuer: source.issuer || 'letsencrypt',
        email: source.email || '',
        staging: Boolean(source.staging),
        caDirectory: source.caDirectory || '',
        subjectsText: (source.subjects || []).join('\n'),
        dnsProvider: dns.provider || '',
        azureSubscriptionId: azure.subscriptionId || '',
        azureResourceGroup: azure.resourceGroup || '',
        azureAuthentication: azure.authentication || 'managedidentity',
        azureTenantId: azure.tenantId || '',
        azureClientId: azure.clientId || '',
        azureClientSecret: '',
        clientSecretConfigured: Boolean(azure.clientSecretConfigured),
        runtimeOnly: Boolean(source.runtimeOnly),
        persisted: Boolean(source.persisted)
      };
      this.certificateDirty = false;
    },

    markCertificateDirty() {
      this.certificateDirty = true;
    },

    normalizeCertificateIssuerForm() {
      if (this.certificateForm.issuer !== 'letsencrypt') this.certificateForm.staging = false;
      if (this.certificateForm.issuer === 'zerossl') this.certificateForm.dnsProvider = '';
    },

    ensureBindForm(container) {
      if (this.bindForms[container.id]) return;
      const ports = this.tcpPorts(container);
      const host = this.defaultHost(container);
      const labeledPort = Number(container.labels?.['caddy.port']) || 0;
      const port = ports.some((candidate) => candidate.privatePort === labeledPort) ? labeledPort : ports.length === 1 ? ports[0].privatePort : 0;
      this.bindForms[container.id] = {
        host,
        port,
        scheme: port === 443 ? 'https' : 'http',
        exposure: container.labels?.['exposure.mode'] || 'public',
        https: !host.endsWith('.localhost')
      };
    },

    bindPolicy(container) {
      return container.bindPolicy || { canBind: true, mode: 'unknown', gatewayNetworks: [] };
    },

    canBindContainer(container) {
      return this.bindPolicy(container).canBind !== false && Number(this.bindForms[container.id]?.port) > 0;
    },

    canUseSuggestedRoute(container) {
      return Boolean(this.bindPolicy(container).suggestedUpstream);
    },

    onBindPortChanged(container) {
      const form = this.bindForms[container.id];
      if (form) form.scheme = Number(form.port) === 443 ? 'https' : 'http';
    },

    bindHint(container) {
      if (!Number(this.bindForms[container.id]?.port)) return this.t('msg.selectContainerPort');
      const policy = this.bindPolicy(container);
      const gatewayNetworks = this.listText(policy.gatewayNetworks || []);
      const loopback = 'http://127.0.0.1:' + this.bindForms[container.id].port;
      switch (policy.mode) {
        case 'host-network':
          return this.format('msg.bindHostNetwork', { upstream: policy.suggestedUpstream, loopback });
        case 'published-port':
          return this.format('msg.bindPublishedPort', { upstream: policy.suggestedUpstream, networks: gatewayNetworks || '-' });
        case 'bridge-unreachable':
          return this.format('msg.bindBridgeUnreachable', { networks: gatewayNetworks || '-' });
        case 'network-unreachable':
          return this.format('msg.bindNetworkUnreachable', { networks: gatewayNetworks || '-' });
        case 'unknown':
          return policy.gatewayNetworks?.length ? '' : this.t('msg.bindUnknownGatewayNetwork');
        default:
          return '';
      }
    },

    useSuggestedRoute(container) {
      const policy = this.bindPolicy(container);
      if (!policy.suggestedUpstream) return;
      const form = this.bindForms[container.id];
      this.routeForm = {
        host: form.host,
        pathPrefix: '',
        upstream: policy.suggestedUpstream.replace(/^https?:/, form.scheme + ':'),
        healthPath: '',
        exposure: form.exposure,
        https: form.https
      };
      this.setActiveView('routes');
      this.showNotice(this.t('msg.routePrefilled'));
    },

    setActiveView(view) {
      this.clearMessages();
      this.activeView = view;
      this.$nextTick(() => {
        window.scrollTo({ top: 0, left: 0, behavior: 'auto' });
        const heading = this.$root.querySelector('#' + view + ' h2');
        if (heading) {
          heading.setAttribute('tabindex', '-1');
          heading.focus({ preventScroll: true });
        }
      });
    },

    subtitle() {
      if (!this.status) return this.t('app.loading');
      const updated = this.lastUpdated || new Date();
      return this.format('app.subtitle', { profile: this.status.profile, time: updated.toLocaleString(this.locale) });
    },

    routes() {
      return this.status?.routes || [];
    },

    lastReconcile() {
      return this.status?.lastReconcile || {};
    },

    dockerActiveText() {
      const docker = this.status?.docker;
      if (docker?.active) return this.t('status.active');
      return docker?.enabled ? this.t('status.unavailable') : this.t('status.disabled');
    },

    caddyLoadedText() {
      const reconcile = this.lastReconcile();
      if (reconcile.caddyLoaded) return this.t('status.loaded');
      if (reconcile.error || reconcile.finishedAt) return this.t('status.failed');
      return this.t('status.notRun');
    },

    reconcileDetails() {
      const reconcile = this.lastReconcile();
      const azure = reconcile.azure || {};
      return this.detailList({
        [this.t('details.started')]: this.formatDate(reconcile.startedAt),
        [this.t('details.finished')]: this.formatDate(reconcile.finishedAt),
        [this.t('details.explicit')]: reconcile.explicitRoutes ?? 0,
        [this.t('details.discovered')]: reconcile.discoveredRoutes ?? 0,
        [this.t('details.applied')]: reconcile.appliedRoutes ?? 0,
        [this.t('details.healthChecks')]: reconcile.healthChecks ?? 0,
        [this.t('details.unhealthyRoutes')]: reconcile.unhealthyRoutes ?? 0,
        [this.t('details.azureDns')]: azure.dnsRecords ?? 0,
        [this.t('details.dnsDeleted')]: azure.dnsDeleted ?? 0,
        [this.t('details.azureNsg')]: azure.nsgRules ?? 0,
        [this.t('details.nsgDeleted')]: azure.nsgDeleted ?? 0,
        [this.t('details.error')]: reconcile.error || azure.error || this.t('status.none')
      });
    },

    dockerDetails() {
      const docker = this.status?.docker || {};
      return this.detailList({
        [this.t('details.active')]: this.yesNo(docker.active),
        [this.t('details.enabled')]: this.yesNo(docker.enabled),
        [this.t('details.profile')]: docker.profile || '-',
        [this.t('details.socket')]: docker.socketPath || '-',
        [this.t('details.endpoint')]: docker.endpoint || '-',
        [this.t('details.reason')]: this.translateBackendText(this.discoveryWarning || docker.reason || '-'),
        [this.t('details.nextActions')]: this.listText(docker.nextActions)
      });
    },

    networkDetails() {
      const azure = this.status?.azure || {};
      const reconcileAzure = this.status?.lastReconcile?.azure || {};
      return this.detailList({
        [this.t('details.enabled')]: this.yesNo(azure.enabled),
        [this.t('details.configured')]: this.yesNo(azure.configured),
        [this.t('details.manageDns')]: azure.enabled ? this.yesNo(azure.manageDNS) : this.t('status.inactive'),
        [this.t('details.manageNsg')]: azure.enabled ? this.yesNo(azure.manageNSG) : this.t('status.inactive'),
        [this.t('details.mode')]: azure.mode || '-',
        [this.t('details.capabilities')]: this.listText(azure.capabilities),
        [this.t('details.missingSettings')]: this.listText(azure.missingSettings),
        [this.t('details.publicIp')]: reconcileAzure.publicIpAddress || '-',
        [this.t('details.dnsUpserts')]: reconcileAzure.dnsRecords ?? 0,
        [this.t('details.nsgUpserts')]: reconcileAzure.nsgRules ?? 0,
        [this.t('details.warnings')]: this.listText([...(azure.warnings || []), ...(reconcileAzure.warnings || [])]) || this.t('status.none'),
        [this.t('details.nextActions')]: this.listText(azure.nextActions)
      });
    },

    runtimeDetails() {
      const health = this.status?.health || {};
      const security = this.status?.security || {};
      const audit = this.status?.audit || {};
      return this.detailList({
        [this.t('details.healthEnabled')]: this.yesNo(health.enabled),
        [this.t('details.healthTimeout')]: health.timeoutSeconds ? health.timeoutSeconds + 's' : '-',
        [this.t('details.securityEnabled')]: this.yesNo(security.enabled),
        [this.t('details.securityBodyLimit')]: this.formatBytes(security.maxRequestBodyBytes),
        [this.t('details.securityDeniedMethods')]: this.listText(security.deniedMethods),
        [this.t('details.securityDeniedPaths')]: this.listText(security.deniedPathPrefixes),
        [this.t('details.securityAllowedCidrs')]: this.listText(security.allowedCidrs),
        [this.t('details.securityBlockedCidrs')]: this.listText(security.blockedCidrs),
        [this.t('details.auditEnabled')]: this.yesNo(audit.enabled),
        [this.t('details.auditFile')]: audit.file || '-'
      });
    },

    detailList(items) {
      return Object.entries(items).map(([label, value]) => ({ label, value: String(value) }));
    },

    routeHealth(route) {
      const health = (this.status?.lastReconcile?.routeHealth || []).find((item) => item.routeId === route.id) || null;
      if (route.lastError) return { text: this.t('status.unhealthy'), className: 'warn', detail: route.lastError };
      if (!health) return { text: this.t('status.unknown'), className: '', detail: '' };
      return health.healthy
        ? { text: this.t('status.healthy'), className: 'ok', detail: '' }
        : { text: this.t('status.unhealthy'), className: 'warn', detail: health.error || '' };
    },

    tcpPorts(container) {
      return (container.ports || []).filter((port) => port.type === 'tcp' && port.privatePort);
    },

    portsText(container) {
      return (container.ports || []).map((port) => port.privatePort + '/' + port.type).join(', ') || '-';
    },

    containerStatusText(container) {
      const key = 'status.' + String(container.state || '').toLowerCase();
      const translated = this.t(key);
      return translated === key ? container.status || container.state || '-' : translated;
    },

    defaultHost(container) {
      return container.labels?.['caddy.host'] || this.slug(container.name) + '.localhost';
    },

    exposureClass(value) {
      return value === 'protected' ? 'warn' : value === 'internal' ? '' : 'ok';
    },

    exposureLabel(value) {
      return this.t('exposure.' + (value || 'public'));
    },

    sourceLabel(value) {
      const key = 'source.' + (value || 'explicit');
      return this.t(key) === key ? value : this.t(key);
    },

    yesNo(value) {
      return value ? this.t('status.yes') : this.t('status.no');
    },

    listText(values) {
      return (values || []).map((value) => this.translateBackendText(value)).join(', ') || '-';
    },

    formatBytes(value) {
      const bytes = Number(value) || 0;
      if (bytes === 0) return this.t('status.none');
      if (bytes % (1024 * 1024) === 0) return bytes / (1024 * 1024) + ' MiB';
      if (bytes % 1024 === 0) return bytes / 1024 + ' KiB';
      return bytes + ' B';
    },

    formatDate(value) {
      if (!value) return '-';
      const date = new Date(value);
      return Number.isNaN(date.getTime()) ? value : date.toLocaleString(this.locale);
    },

    translateBackendText(value) {
      const text = String(value ?? '');
      const key = backendMessageKeys.get(text);
      if (key) return this.t(key);
      if (this.locale === 'zh-CN') {
        const hostMatch = text.match(/^host "([^"]+)" must be a fully qualified DNS name$/);
        if (hostMatch) return `主机名“${hostMatch[1]}”必须是完整域名`;
      }
      return text;
    },

    t(key) {
      return messages[this.locale]?.[key] || messages.en[key] || key;
    },

    format(key, values) {
      return Object.entries(values).reduce((text, [name, value]) => text.replaceAll('{' + name + '}', value), this.t(key));
    },

    slug(value) {
      return String(value).toLowerCase().replace(/[^a-z0-9.-]+/g, '-').replace(/^[.-]+|[.-]+$/g, '') || 'service';
    },

    clearMessages() {
      this.alert = '';
      this.notice = '';
    },

    busy() {
      return this.isRefreshing || this.isActing;
    },

    showAlert(message) {
      this.notice = '';
      this.alert = this.translateBackendText(message);
    },

    showNotice(message) {
      this.alert = '';
      this.notice = message;
    },

    openLogin() {
      const dialog = this.$refs.loginDialog;
      if (dialog && !dialog.open) dialog.showModal();
    },

    closeLogin() {
      const dialog = this.$refs.loginDialog;
      if (dialog && dialog.open) dialog.close();
    }
  }));
});

function emptyRouteForm() {
  return { host: '', pathPrefix: '', upstream: '', healthPath: '', exposure: 'public', https: true };
}

function emptyCertificateForm() {
  return {
    issuer: 'letsencrypt', email: '', staging: false, caDirectory: '', subjectsText: '', dnsProvider: '',
    azureSubscriptionId: '', azureResourceGroup: '', azureAuthentication: 'managedidentity',
    azureTenantId: '', azureClientId: '', azureClientSecret: '', clientSecretConfigured: false,
    runtimeOnly: true, persisted: false
  };
}
