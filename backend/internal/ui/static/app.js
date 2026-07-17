const supportedLocales = ['en', 'zh-CN'];
const defaultLocale = navigator.language && navigator.language.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en';
const configurationArchiveFiles = new Set(['manifest.json', 'routes.json', 'settings.json', 'certificate-policy.json']);

function validConfigurationArchive(data) {
  const bytes = new Uint8Array(data);
  if (bytes.length < 22) return false;
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  if (view.getUint32(0, true) !== 0x04034b50) return false;

  let endOffset = -1;
  const earliestEndOffset = Math.max(0, bytes.length - 65557);
  for (let offset = bytes.length - 22; offset >= earliestEndOffset; offset -= 1) {
    if (view.getUint32(offset, true) !== 0x06054b50) continue;
    if (offset + 22 + view.getUint16(offset + 20, true) === bytes.length) {
      endOffset = offset;
      break;
    }
  }
  if (endOffset < 0 || view.getUint16(endOffset + 4, true) !== 0 || view.getUint16(endOffset + 6, true) !== 0) return false;

  const diskEntries = view.getUint16(endOffset + 8, true);
  const totalEntries = view.getUint16(endOffset + 10, true);
  const directorySize = view.getUint32(endOffset + 12, true);
  const directoryOffset = view.getUint32(endOffset + 16, true);
  if (diskEntries !== configurationArchiveFiles.size || totalEntries !== configurationArchiveFiles.size || directoryOffset + directorySize !== endOffset) return false;

  const names = new Set();
  let entryOffset = directoryOffset;
  for (let index = 0; index < totalEntries; index += 1) {
    if (entryOffset + 46 > endOffset || view.getUint32(entryOffset, true) !== 0x02014b50) return false;
    const nameLength = view.getUint16(entryOffset + 28, true);
    const extraLength = view.getUint16(entryOffset + 30, true);
    const commentLength = view.getUint16(entryOffset + 32, true);
    const nextOffset = entryOffset + 46 + nameLength + extraLength + commentLength;
    if (nextOffset > endOffset) return false;
    names.add(new TextDecoder().decode(bytes.subarray(entryOffset + 46, entryOffset + 46 + nameLength)));
    entryOffset = nextOffset;
  }
  return entryOffset === endOffset && names.size === configurationArchiveFiles.size && [...configurationArchiveFiles].every((name) => names.has(name));
}

const messages = {
  en: {
    title: 'Caddy Proxy',
    'nav.primary': 'Primary',
    'nav.dashboard': 'Dashboard',
    'nav.routes': 'Routes',
    'nav.discovery': 'Discovery',
    'nav.platform': 'Platform',
    'nav.certificates': 'Certificates',
    'nav.security': 'Security',
    'nav.settings': 'Settings',
    'nav.logs': 'Logs',
    'language.label': 'Language',
    'actions.refresh': 'Refresh data',
    'actions.refreshTitle': 'Fetch the latest state without changing the active configuration',
    'actions.signOut': 'Sign out',
    'actions.apply': 'Reconcile configuration',
    'actions.applyPending': 'Apply pending changes',
    'actions.applyTitle': 'Regenerate and apply the complete Caddy configuration',
    'actions.add': 'Add',
    'actions.addRoute': 'Add route',
    'actions.addListener': 'Add listener',
    'actions.addBackendPool': 'Add backend pool',
    'actions.edit': 'Edit',
    'actions.cancelEdit': 'Cancel edit',
    'actions.saveChanges': 'Save changes',
    'actions.retryReconcile': 'Retry reconcile',
    'actions.reviewDiscovery': 'Review discovery',
    'actions.reviewPlatform': 'Review platform',
    'actions.continue': 'Continue',
    'actions.delete': 'Delete',
    'actions.cancel': 'Cancel',
    'actions.confirmDelete': 'Delete',
    'actions.bind': 'Bind',
    'actions.useSuggestedRoute': 'Use Suggested Route',
    'actions.requestRefresh': 'Reload TLS config',
    'actions.refreshCertificateStatus': 'Refresh status',
    'actions.enableEarlyRenewal': 'Enable earlier renewal',
    'actions.saveAndApply': 'Save and apply',
    'actions.saveSecurity': 'Save security policy',
    'actions.saveSettings': 'Save settings',
    'actions.checkAzurePermissions': 'Check permissions',
    'actions.refreshLogs': 'Refresh logs',
    'actions.exportConfiguration': 'Export configuration',
    'actions.importConfiguration': 'Validate and import',
    'app.heading': 'Gateway Control Plane',
    'app.productType': 'Reverse Proxy Console',
    'app.loading': 'Loading current state',
    'app.subtitle': '{deployment} · Updated {time}',
    'views.dashboardDescription': 'Runtime health and the next actions that need attention.',
    'views.routesDescription': 'Connect reusable listeners and backend pools with routing rules.',
    'views.discoveryDescription': 'Review Docker workloads and bind reachable containers.',
    'views.platformDescription': 'Inspect Azure integration, runtime safeguards, and audit settings.',
    'views.certificatesDescription': 'Manage ACME issuance and automatic certificate renewal.',
    'views.securityDescription': 'Configure gateway-wide request controls and protected-route access.',
    'views.settingsDescription': 'Manage the next deployment mode, Azure integration, and the console login token.',
    'views.logsDescription': 'Inspect recent gateway, Caddy, and configuration audit events.',
    'metrics.profile': 'Deployment',
    'metrics.routes': 'Saved routes',
    'metrics.docker': 'Docker',
    'metrics.caddy': 'Caddy',
    'sections.recentReconcile': 'Recent Reconcile',
    'sections.nextActions': 'Needs Attention',
    'sections.activeRoutes': 'Routing Rules',
    'sections.addRoute': 'Add Routing Rule',
    'sections.listeners': 'Listeners',
    'sections.addListener': 'Add Listener',
    'sections.editListener': 'Edit Listener',
    'sections.backendPools': 'Backend Pools',
    'sections.addBackendPool': 'Add Backend Pool',
    'sections.editBackendPool': 'Edit Backend Pool',
    'sections.routingRules': 'Routing Rules',
    'sections.addRoutingRule': 'Add Routing Rule',
    'sections.editRoutingRule': 'Edit Routing Rule',
    'sections.dockerDiscovery': 'Docker Discovery',
    'sections.azureNetwork': 'Azure & Network',
    'sections.runtimeSecurity': 'Runtime & Security',
    'sections.certificates': 'Certificates',
    'sections.certificatePolicy': 'Issuance policy',
    'sections.issuedCertificates': 'Issued certificates',
    'sections.securityBaseline': 'Request Security Baseline',
    'sections.accessPolicy': 'Access Policy',
    'sections.systemSettings': 'System Settings',
    'sections.azureSettings': 'Azure Integration',
    'sections.runtimeLogs': 'Runtime logs',
    'sections.configurationFiles': 'Configuration files',
    'tables.host': 'Frontend hostname',
    'tables.name': 'Name',
    'tables.hostname': 'Hostname',
    'tables.protocol': 'Protocol',
    'tables.port': 'Port',
    'tables.targets': 'Backend addresses',
    'tables.listener': 'Listener',
    'tables.backendPool': 'Backend pool',
    'tables.backend': 'Backend protocol / port',
    'tables.path': 'Route path',
    'tables.exposure': 'Exposure',
    'tables.source': 'Source',
    'tables.upstream': 'Backend targets',
    'tables.health': 'Health',
    'tables.https': 'HTTPS',
    'tables.container': 'Container',
    'tables.image': 'Image',
    'tables.status': 'Status',
    'tables.ports': 'Ports',
    'tables.bind': 'Bind',
    'tables.actions': 'Actions',
    'forms.host': 'Frontend hostname',
    'forms.name': 'Name',
    'forms.hostname': 'Frontend hostname',
    'forms.protocol': 'Frontend protocol',
    'forms.port': 'Frontend port',
    'forms.listenerPortContainerHint': 'Standard Container + Socket deployments publish ports 80 and 443. Publish a custom host port before using it here.',
    'forms.listenerPortAzureHint': 'Azure VM binds listener ports directly. Public custom ports also require managed NSG reconciliation or a manual NSG rule.',
    'forms.targets': 'Backend addresses',
    'forms.targetsHint': 'One target per line, without a scheme. Private/public IPs and hostnames are supported and resolved from the gateway. Normally omit the port and use the shared rule port; host:port is an optional per-target override.',
    'forms.listener': 'Listener',
    'forms.selectListener': 'Select a listener',
    'forms.backendPool': 'Backend pool',
    'forms.selectBackendPool': 'Select a backend pool',
    'forms.backendProtocol': 'Backend protocol',
    'forms.backendProtocolHint': 'Choose HTTP or HTTPS for the upstream connection. WebSocket upgrades are proxied automatically over either protocol.',
    'forms.backendPort': 'Backend port',
    'forms.backendHostHeader': 'Backend Host header',
    'forms.backendHostHeaderHint': 'Optional. Blank preserves the frontend hostname. Set ex.example.com when an external virtual host expects its own hostname.',
    'forms.enabled': 'Enabled',
    'forms.pathPrefix': 'Route path prefix',
    'forms.pathPrefixHint': 'Leave blank to forward every path on this listener. A value such as /api only matches /api and paths below it; the prefix is preserved upstream.',
    'forms.upstreamUrl': 'Backend target URL',
    'forms.upstreamScheme': 'Upstream protocol',
    'forms.healthPath': 'Backend health path',
    'forms.healthPathHint': 'Optional. Blank uses the global path ({path}); HTTP 2xx/3xx is healthy. If that path returns 404, enter the service readiness path.',
    'forms.exposure': 'Exposure',
    'forms.https': 'Inbound HTTPS',
    'exposureHint.public': 'Anyone who can reach the listener can access this route.',
    'exposureHint.protected': 'Requires a valid gateway token in Authorization: Bearer or X-Admin-Token.',
    'exposureHint.internal': 'Only clients in the configured internal source IP/CIDR ranges are allowed.',
    'forms.certificateIssuer': 'Issuer',
    'forms.certificateEmail': 'ACME account email',
    'forms.certificateStaging': 'Use staging CA',
    'forms.caDirectory': 'CA directory URL',
    'forms.certificateSubjects': 'Managed frontend hostnames',
    'forms.renewalWindowRatio': 'Automatic renewal window',
    'forms.renewalWindowRatioHint': 'Start renewal when this share of the certificate lifetime remains. 50% renews earlier than Caddy’s 33% default.',
    'forms.dnsProvider': 'DNS challenge provider',
    'forms.azureSubscriptionId': 'Azure subscription ID',
    'forms.azureResourceGroup': 'DNS zone resource group',
    'forms.azureAuthentication': 'Azure authentication',
    'forms.azureTenantId': 'Tenant ID',
    'forms.azureClientId': 'Client ID',
    'forms.azureClientSecret': 'Client secret',
    'forms.logLevel': 'Level',
    'forms.logSource': 'Source',
    'forms.logSearch': 'Search',
    'forms.securityEnabled': 'Enable request security baseline',
    'forms.maxRequestBodyMiB': 'Maximum request body (MiB)',
    'forms.deniedMethods': 'Denied HTTP methods',
    'forms.deniedMethodsHint': 'One method per line or comma-separated. Use 0 MiB for no body-size limit.',
    'forms.deniedPaths': 'Denied path prefixes',
    'forms.deniedPathsHint': 'For example /.git and /.env. Prefixes apply within every routed hostname.',
    'forms.allowedCidrs': 'Allowed source CIDRs',
    'forms.allowedCidrsHint': 'Optional allowlist. When set, every source outside these ranges is rejected.',
    'forms.blockedCidrs': 'Blocked source CIDRs',
    'forms.internalSourceRanges': 'Internal route source ranges',
    'forms.internalSourceRangesHint': 'Routes with Internal exposure only accept direct client addresses in these IP/CIDR ranges.',
    'forms.allowBearerToken': 'Accept Authorization: Bearer on protected routes',
    'forms.allowAdminTokenHeader': 'Accept X-Admin-Token on protected routes',
    'forms.deploymentMode': 'Deployment mode after restart',
    'forms.adminTokenNew': 'New admin token',
    'forms.adminTokenHint': 'Leave blank to keep the current token. Saving a new value immediately invalidates the old login token.',
    'forms.azureEnabled': 'Enable Azure reconciliation',
    'forms.azureManageDns': 'Manage Azure DNS A records',
    'forms.azureManageNsg': 'Manage the NSG listener rule',
    'forms.azureDnsZones': 'DNS zones',
    'forms.azureDnsZonesHint': 'One per line as zone | resource group. The default resource group is used when the second value is omitted.',
    'forms.azureNsgResourceGroup': 'NSG resource group',
    'forms.azureNsgName': 'Network security group name',
    'forms.azurePublicIp': 'Ingress public IPv4 address',
    'forms.azureNsgPriority': 'Managed NSG rule priority',
    'forms.azureNsgSources': 'NSG source prefixes',
    'forms.azureNsgSourcesHint': 'One CIDR per line; use * to allow all sources.',
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
    'status.editing': 'Editing',
    'status.restartRequired': 'Restart required',
    'status.activeDeployment': 'Currently active: {deployment}',
    'status.granted': 'Granted',
    'status.missingPermissions': 'Missing permissions',
    'status.unableToVerify': 'Unable to verify',
    'status.notConfigured': 'Not configured',
    'status.pendingApply': 'Pending apply',
    'status.valid': 'Valid',
    'status.renewalDue': 'Renewal due',
    'status.expired': 'Expired',
    'status.notYetValid': 'Not yet valid',
    'status.running': 'Running',
    'status.paused': 'Paused',
    'status.restarting': 'Restarting',
    'status.exited': 'Exited',
    'status.created': 'Created',
    'status.dead': 'Stopped',
    'empty.noRoutes': 'No routes',
    'empty.noListeners': 'No listeners',
    'empty.noBackendPools': 'No backend pools',
    'empty.noRoutingRules': 'No routing rules',
    'empty.noContainers': 'No workload containers discovered',
    'empty.noLogs': 'No logs match the current filters.',
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
    'certificate.renewalDefault': 'Standard · 33% remaining',
    'certificate.renewalEarlier': 'Earlier · 50% remaining',
    'certificate.renewalEarliest': 'Earliest · 67% remaining',
    'certificate.persistedNote': 'Caddy renews managed certificates automatically before expiry. Keep /data/caddy persistent and challenge access available. Reload TLS config reapplies the policy; it does not force renewal.',
    'deployment.containerSocket': 'Container + Socket',
    'deployment.azureVM': 'Azure VM',
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
    'details.profile': 'Deployment',
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
    'source.compatibility': 'Migrated',
    'routes.resourceNavigation': 'Routing resources',
    'routes.listeners': 'Listeners',
    'routes.backendPools': 'Backend pools',
    'routes.routingRules': 'Routing rules',
    'routes.prerequisites': 'Create at least one listener and one backend pool before adding a routing rule.',
    'resources.listener': 'listener',
    'resources.backendPool': 'backend pool',
    'resources.routingRule': 'routing rule',
    'aria.routeHost': 'Route host',
    'aria.upstreamPort': 'Upstream port',
    'aria.exposure': 'Exposure',
    'msg.certificateSaved': 'Certificate policy saved and Caddy reloaded',
    'msg.certificateSaveFailed': 'Certificate policy was saved, but Caddy reload failed: {error}',
    'msg.certificateRefreshed': 'TLS configuration reloaded',
    'msg.certificateRefreshFailed': 'TLS configuration reload failed: {error}',
    'msg.certificateStatusRefreshed': 'Certificate status refreshed',
    'msg.earlyRenewalEnabled': 'Earlier renewal enabled and Caddy reloaded',
    'msg.unsavedCertificate': 'Save or discard certificate changes before reloading TLS configuration',
    'msg.routeSaved': 'Route saved and applied',
    'msg.routeSaveFailed': 'Route was saved, but reconcile failed: {error}',
    'msg.routeUpdated': 'Route updated and applied',
    'msg.routeUpdateFailed': 'Route was updated, but reconcile failed: {error}',
    'msg.routeDeleted': 'Route deleted and configuration reconciled',
    'msg.routeDeleteFailed': 'Route was deleted, but reconcile failed: {error}',
    'msg.listenerSaved': 'Listener saved',
    'msg.listenerUpdated': 'Listener updated and applied',
    'msg.listenerUpdateFailed': 'Listener was updated, but reconcile failed: {error}',
    'msg.listenerDeleted': 'Listener deleted',
    'msg.backendPoolSaved': 'Backend pool saved',
    'msg.backendPoolUpdated': 'Backend pool updated and applied',
    'msg.backendPoolUpdateFailed': 'Backend pool was updated, but reconcile failed: {error}',
    'msg.backendPoolDeleted': 'Backend pool deleted',
    'msg.routingChangeSaved': 'Saved. Use Apply pending changes when the route configuration is ready.',
    'msg.logsRefreshed': 'Logs refreshed',
    'msg.confirmDeleteResource': 'Delete {type} “{name}”?',
    'msg.noBackendTargets': 'Add at least one backend address',
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
    'msg.routePrefilled': 'Listener, backend pool, and routing rule forms populated from the suggestion',
    'msg.noPublicRoutesAzure': 'no public routes require Azure reconciliation',
    'msg.managedDnsNoRelativeName': 'managed DNS record without a relative name was skipped',
    'msg.nsgSourcePolicy': 'NSG priority and source-prefix policy',
    'msg.setMissingAzureSettings': 'Set the missing Azure settings, including DNS zones and the ingress public IP',
    'msg.runReconcileEnabledAzure': 'Run Reconcile now to update the enabled Azure resources',
    'msg.azureDefaultCredential': 'Azure default credential chain',
    'msg.azureDnsCapability': 'Azure DNS A record reconciliation',
    'msg.azureNsgCapability': 'VM NSG listener-port reconciliation',
    'msg.gatewayCannotBind': 'The gateway container cannot be bound as an upstream',
    'msg.invalidUpstreamScheme': 'Upstream protocol must be HTTP or HTTPS',
    'msg.securitySaved': 'Security policy saved and applied',
    'msg.securitySaveFailed': 'Security policy was saved, but Caddy reload failed: {error}',
    'msg.settingsSaved': 'Settings saved',
    'msg.settingsSavedRestart': 'Settings saved. Restart Caddy Proxy to activate Deployment or Azure changes.',
    'msg.settingsUnavailable': 'Settings persistence is unavailable in this runtime.',
    'msg.configurationExported': 'Configuration archive downloaded',
    'msg.configurationExportInvalid': 'The server returned an invalid configuration archive. Refresh the page and try again.',
    'msg.configurationImported': 'Validated {listeners} listeners, {pools} backend pools, {rules} routing rules, and {subjects} certificate subjects. Nothing has been persisted yet; click Apply pending changes.',
    'msg.selectConfigurationArchive': 'Select a configuration ZIP archive first.',
    'msg.applyImportedFirst': 'Apply the imported configuration before changing settings or certificate policy.',
    'settings.deploymentNote': 'Deployment changes affect Docker discovery and Azure clients. They are persisted for the next process start; network mode, socket mounts, and published ports must also match the selected deployment.',
    'settings.azureIdentityNote': 'Azure uses DefaultAzureCredential. On an Azure VM, assign a managed identity with DNS Zone Contributor for managed zones and Network Contributor for the target NSG; no client secret is stored here.',
    'settings.azureStatusNote': 'Apply upserts desired A records, lists managed A records for cleanup, and waits for the NSG rule operation. Platform shows the latest counts, warnings, and Azure API error; Configured validates inputs, not identity permissions.',
    'settings.azurePermissionCheckNote': 'Checks the effective ARM actions available to this running process without writing resources. On an Azure VM this normally checks its managed identity; local development may use Azure CLI or another DefaultAzureCredential identity.',
    'settings.azurePermissionsTitle': 'Effective permissions',
    'settings.azurePermissionMetadata': 'Checked with {credential} · {time}',
    'settings.azureDnsPermissions': 'DNS zones',
    'settings.azureNetworkPermissions': 'Network security group',
    'settings.azurePermissionNotConfigured': 'This capability is not enabled in the current form.',
    'settings.azurePermissionResourceGroup': 'Resource group: {resourceGroup}',
    'settings.azurePermissionMissing': 'Missing actions: {actions}',
    'settings.configurationDescription': 'Move routing, Console settings, and certificate issuance policy between Caddy Proxy instances.',
    'settings.configurationExportTitle': 'Export',
    'settings.configurationExportNote': 'Downloads caddyproxy_config_yyyymmdd.zip. Issued certificates, private keys, runtime logs, audit logs, and secrets are excluded.',
    'settings.configurationImportTitle': 'Import',
    'settings.configurationImportNote': 'The ZIP is fully parsed, normalized, and rendered before it becomes an in-memory draft. Apply writes it locally and activates routes and certificate policy; deployment and Azure startup settings require a restart.',
    'settings.configurationValidatedTitle': 'Validated import',
    'settings.configurationPending': 'An imported configuration is validated and waiting for Apply. Settings and certificate policy changes cannot be saved until then.',
    'msg.protectedPolicyRequired': 'Keep at least one protected-route token header enabled.',
    'dashboard.reconcileIssue': 'The last reconcile did not fully apply the generated configuration.',
    'dashboard.noRoutes': 'No saved routes are configured. Add a route to publish a service.',
    'dashboard.dockerIssue': 'Docker discovery needs attention: {details}',
    'dashboard.azureIssue': 'Azure integration is enabled but incomplete: {details}',
    'routes.pendingApply': 'Route changes are saved but not active. Finish editing, then use Apply pending changes in the top-right corner.',
    'logs.allLevels': 'All levels',
    'logs.allSources': 'All sources',
    'logs.time': 'Time',
    'logs.level': 'Level',
    'logs.source': 'Source',
    'logs.message': 'Message',
    'logs.fields': 'Details',
    'logs.searchPlaceholder': 'Message, source, or details',
    'certificates.storage': 'Caddy storage',
    'certificates.scannedAt': 'Scanned',
    'certificates.expires': 'Expires',
    'certificates.renewalStarts': 'Renewal window starts',
    'certificates.issuer': 'Issuer',
    'certificates.certificateFile': 'Certificate file',
    'certificates.privateKeyFile': 'Private key file',
    'certificates.metadataFile': 'Metadata file',
    'certificates.fingerprint': 'SHA-256 fingerprint',
    'certificates.subjects': 'Certificate names',
    'certificates.noCertificates': 'No issued certificate files were found in Caddy storage.',
    'certificates.runtimeUnavailable': 'Certificate storage inspection is unavailable.',
    'certificates.policyNote': 'Saving updates Caddy’s automation policy. Reloading TLS applies the current policy; it does not force an ACME renewal.',
    'certificates.earlyRenewalTitle': 'Set the renewal window to 50% of certificate lifetime and apply the policy.'
  },
  'zh-CN': {
    title: 'Caddy Proxy',
    'nav.primary': '主导航',
    'nav.dashboard': '仪表盘',
    'nav.routes': '路由',
    'nav.discovery': '发现',
    'nav.platform': '平台',
    'nav.certificates': '证书',
    'nav.security': '安全',
    'nav.settings': '设置',
    'nav.logs': '日志',
    'language.label': '语言',
    'actions.refresh': '刷新数据',
    'actions.refreshTitle': '只获取最新状态，不修改当前生效配置',
    'actions.signOut': '退出登录',
    'actions.apply': '协调并应用配置',
    'actions.applyPending': '应用待处理更改',
    'actions.applyTitle': '重新生成并应用完整的 Caddy 配置',
    'actions.add': '添加',
    'actions.addRoute': '添加路由',
    'actions.addListener': '添加监听器',
    'actions.addBackendPool': '添加后端池',
    'actions.edit': '编辑',
    'actions.cancelEdit': '取消编辑',
    'actions.saveChanges': '保存修改',
    'actions.retryReconcile': '重试协调',
    'actions.reviewDiscovery': '查看发现',
    'actions.reviewPlatform': '查看平台',
    'actions.continue': '继续',
    'actions.delete': '删除',
    'actions.cancel': '取消',
    'actions.confirmDelete': '删除',
    'actions.bind': '绑定',
    'actions.useSuggestedRoute': '使用建议路由',
    'actions.requestRefresh': '重新加载 TLS 配置',
    'actions.refreshCertificateStatus': '刷新状态',
    'actions.enableEarlyRenewal': '启用提前续期',
    'actions.saveAndApply': '保存并应用',
    'actions.saveSecurity': '保存安全策略',
    'actions.saveSettings': '保存设置',
    'actions.checkAzurePermissions': '检查权限',
    'actions.refreshLogs': '刷新日志',
    'actions.exportConfiguration': '导出配置',
    'actions.importConfiguration': '校验并导入',
    'app.heading': '网关控制平面',
    'app.productType': '反向代理控制台',
    'app.loading': '正在加载当前状态',
    'app.subtitle': '{deployment} · 更新于 {time}',
    'views.dashboardDescription': '查看运行健康状态，以及当前需要处理的操作。',
    'views.routesDescription': '使用可复用的监听器和后端池组成路由规则。',
    'views.discoveryDescription': '查看 Docker 工作负载并绑定网关可达的容器。',
    'views.platformDescription': '检查 Azure 集成、运行保护和审计设置。',
    'views.certificatesDescription': '管理 ACME 签发和证书自动续期。',
    'views.securityDescription': '配置网关全局请求防护与受保护路由的访问策略。',
    'views.settingsDescription': '管理下次启动的部署方式、Azure 集成和控制台登录令牌。',
    'views.logsDescription': '查看最近的网关、Caddy 运行日志和配置审计事件。',
    'metrics.profile': '部署方式',
    'metrics.routes': '已保存路由',
    'metrics.docker': 'Docker',
    'metrics.caddy': 'Caddy',
    'sections.recentReconcile': '最近一次协调',
    'sections.nextActions': '需要处理',
    'sections.activeRoutes': '路由规则',
    'sections.addRoute': '添加路由规则',
    'sections.listeners': '监听器',
    'sections.addListener': '添加监听器',
    'sections.editListener': '编辑监听器',
    'sections.backendPools': '后端池',
    'sections.addBackendPool': '添加后端池',
    'sections.editBackendPool': '编辑后端池',
    'sections.routingRules': '路由规则',
    'sections.addRoutingRule': '添加路由规则',
    'sections.editRoutingRule': '编辑路由规则',
    'sections.dockerDiscovery': 'Docker 发现',
    'sections.azureNetwork': 'Azure 与网络',
    'sections.runtimeSecurity': '运行与安全',
    'sections.certificates': '证书',
    'sections.certificatePolicy': '签发策略',
    'sections.issuedCertificates': '已签发证书',
    'sections.securityBaseline': '请求安全基线',
    'sections.accessPolicy': '访问策略',
    'sections.systemSettings': '系统设置',
    'sections.azureSettings': 'Azure 集成',
    'sections.runtimeLogs': '运行日志',
    'sections.configurationFiles': '配置文件',
    'tables.host': '前端主机名',
    'tables.name': '名称',
    'tables.hostname': '主机名',
    'tables.protocol': '协议',
    'tables.port': '端口',
    'tables.targets': '后端地址',
    'tables.listener': '监听器',
    'tables.backendPool': '后端池',
    'tables.backend': '后端协议 / 端口',
    'tables.path': '路由路径',
    'tables.exposure': '暴露方式',
    'tables.source': '来源',
    'tables.upstream': '后端目标',
    'tables.health': '健康',
    'tables.https': 'HTTPS',
    'tables.container': '容器',
    'tables.image': '镜像',
    'tables.status': '状态',
    'tables.ports': '端口',
    'tables.bind': '绑定',
    'tables.actions': '操作',
    'forms.host': '前端主机名',
    'forms.name': '名称',
    'forms.hostname': '前端主机名',
    'forms.protocol': '前端协议',
    'forms.port': '前端端口',
    'forms.listenerPortContainerHint': '标准“容器 + Docker Socket”部署只发布 80 和 443；使用其他端口前，请先显式发布对应宿主机端口。',
    'forms.listenerPortAzureHint': 'Azure VM 会直接绑定监听端口；公网自定义端口还需要启用托管 NSG 协调或手动添加 NSG 规则。',
    'forms.targets': '后端地址',
    'forms.targetsHint': '每行一个目标，不要填写协议。支持私网/公网 IP 和主机名，主机名由网关环境解析。通常省略端口并使用路由共享端口；也可用 host:port 为单个目标覆盖端口。',
    'forms.listener': '监听器',
    'forms.selectListener': '请选择监听器',
    'forms.backendPool': '后端池',
    'forms.selectBackendPool': '请选择后端池',
    'forms.backendProtocol': '后端协议',
    'forms.backendProtocolHint': '选择网关连接上游时使用 HTTP 或 HTTPS；两者都会自动代理 WebSocket Upgrade，无需单独开关。',
    'forms.backendPort': '后端端口',
    'forms.backendHostHeader': '后端 Host Header',
    'forms.backendHostHeaderHint': '可选。留空时保留前端主机名；外部虚拟主机要求自身域名时填写，例如 ex.example.com。',
    'forms.enabled': '启用',
    'forms.pathPrefix': '路由路径前缀',
    'forms.pathPrefixHint': '留空表示转发该监听器下的所有路径。填写 /api 时只匹配 /api 及其子路径，转发时不会移除此前缀。',
    'forms.upstreamUrl': '后端目标 URL',
    'forms.upstreamScheme': '上游协议',
    'forms.healthPath': '后端健康检查路径',
    'forms.healthPathHint': '可留空；留空时使用全局路径（{path}），HTTP 2xx/3xx 视为健康。如果该路径返回 404，请填写服务真实的就绪路径。',
    'forms.exposure': '暴露方式',
    'forms.https': '入口 HTTPS',
    'exposureHint.public': '任何能够访问该监听器的客户端都可访问此路由。',
    'exposureHint.protected': '必须通过 Authorization: Bearer 或 X-Admin-Token 提供有效网关令牌。',
    'exposureHint.internal': '仅允许来自已配置内部源 IP/CIDR 范围的客户端。',
    'forms.certificateIssuer': '签发器',
    'forms.certificateEmail': 'ACME 账户邮箱',
    'forms.certificateStaging': '使用测试 CA',
    'forms.caDirectory': 'CA Directory URL',
    'forms.certificateSubjects': '托管前端域名',
    'forms.renewalWindowRatio': '自动续期窗口',
    'forms.renewalWindowRatioHint': '当证书剩余此比例的有效期时开始续期；50% 会早于 Caddy 默认的 33%。',
    'forms.dnsProvider': 'DNS Challenge 提供商',
    'forms.azureSubscriptionId': 'Azure 订阅 ID',
    'forms.azureResourceGroup': 'DNS Zone 资源组',
    'forms.azureAuthentication': 'Azure 认证方式',
    'forms.azureTenantId': '租户 ID',
    'forms.azureClientId': '客户端 ID',
    'forms.azureClientSecret': '客户端密钥',
    'forms.logLevel': '级别',
    'forms.logSource': '来源',
    'forms.logSearch': '搜索',
    'forms.securityEnabled': '启用请求安全基线',
    'forms.maxRequestBodyMiB': '最大请求体（MiB）',
    'forms.deniedMethods': '拒绝的 HTTP 方法',
    'forms.deniedMethodsHint': '每行一个或使用逗号分隔；请求体上限填 0 表示不限制。',
    'forms.deniedPaths': '拒绝的路径前缀',
    'forms.deniedPathsHint': '例如 /.git 和 /.env；这些前缀会应用到每个路由域名。',
    'forms.allowedCidrs': '允许的来源 CIDR',
    'forms.allowedCidrsHint': '可选白名单；填写后，范围以外的所有来源都会被拒绝。',
    'forms.blockedCidrs': '阻止的来源 CIDR',
    'forms.internalSourceRanges': '内部路由来源范围',
    'forms.internalSourceRangesHint': '“内部”暴露方式只允许直接客户端地址位于这些 IP/CIDR 范围内。',
    'forms.allowBearerToken': '受保护路由接受 Authorization: Bearer',
    'forms.allowAdminTokenHeader': '受保护路由接受 X-Admin-Token',
    'forms.deploymentMode': '重启后的部署方式',
    'forms.adminTokenNew': '新管理员令牌',
    'forms.adminTokenHint': '留空表示保持当前令牌；保存新值后，旧登录令牌会立即失效。',
    'forms.azureEnabled': '启用 Azure 协调',
    'forms.azureManageDns': '管理 Azure DNS A 记录',
    'forms.azureManageNsg': '管理 NSG 监听规则',
    'forms.azureDnsZones': 'DNS Zone',
    'forms.azureDnsZonesHint': '每行格式为 zone | 资源组；省略第二项时使用默认资源组。',
    'forms.azureNsgResourceGroup': 'NSG 资源组',
    'forms.azureNsgName': '网络安全组名称',
    'forms.azurePublicIp': '入口公网 IPv4 地址',
    'forms.azureNsgPriority': '托管 NSG 规则优先级',
    'forms.azureNsgSources': 'NSG 来源前缀',
    'forms.azureNsgSourcesHint': '每行一个 CIDR；使用 * 表示允许所有来源。',
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
    'status.editing': '正在编辑',
    'status.restartRequired': '需要重启',
    'status.activeDeployment': '当前生效：{deployment}',
    'status.granted': '已授权',
    'status.missingPermissions': '缺少权限',
    'status.unableToVerify': '无法验证',
    'status.notConfigured': '未配置',
    'status.pendingApply': '等待应用',
    'status.valid': '有效',
    'status.renewalDue': '需要续期',
    'status.expired': '已过期',
    'status.notYetValid': '尚未生效',
    'status.running': '运行中',
    'status.paused': '已暂停',
    'status.restarting': '正在重启',
    'status.exited': '已退出',
    'status.created': '已创建',
    'status.dead': '已停止',
    'empty.noRoutes': '没有路由',
    'empty.noListeners': '没有监听器',
    'empty.noBackendPools': '没有后端池',
    'empty.noRoutingRules': '没有路由规则',
    'empty.noContainers': '未发现工作负载容器',
    'empty.noLogs': '当前筛选条件下没有日志。',
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
    'certificate.renewalDefault': '标准 · 剩余 33% 时续期',
    'certificate.renewalEarlier': '较早 · 剩余 50% 时续期',
    'certificate.renewalEarliest': '最早 · 剩余 67% 时续期',
    'certificate.persistedNote': 'Caddy 会在到期前自动续签托管证书。请持久化 /data/caddy，并确保签发验证仍可用。“重新加载 TLS 配置”只会重新应用策略，不会强制续期。',
    'deployment.containerSocket': '容器 + Docker Socket',
    'deployment.azureVM': 'Azure VM',
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
    'details.profile': '部署方式',
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
    'source.compatibility': '已迁移',
    'routes.resourceNavigation': '路由资源',
    'routes.listeners': '监听器',
    'routes.backendPools': '后端池',
    'routes.routingRules': '路由规则',
    'routes.prerequisites': '添加路由规则前，请先创建至少一个监听器和一个后端池。',
    'resources.listener': '监听器',
    'resources.backendPool': '后端池',
    'resources.routingRule': '路由规则',
    'aria.routeHost': '路由主机名',
    'aria.upstreamPort': '上游端口',
    'aria.exposure': '暴露方式',
    'msg.certificateSaved': '证书策略已保存，Caddy 已重新加载',
    'msg.certificateSaveFailed': '证书策略已保存，但 Caddy 重新加载失败：{error}',
    'msg.certificateRefreshed': 'TLS 配置已重新加载',
    'msg.certificateRefreshFailed': 'TLS 配置重新加载失败：{error}',
    'msg.certificateStatusRefreshed': '证书状态已刷新',
    'msg.earlyRenewalEnabled': '提前续期已启用，Caddy 已重新加载',
    'msg.unsavedCertificate': '请先保存或放弃证书改动，再重新加载 TLS 配置',
    'msg.routeSaved': '路由已保存并应用',
    'msg.routeSaveFailed': '路由已保存，但协调失败：{error}',
    'msg.routeUpdated': '路由已更新并应用',
    'msg.routeUpdateFailed': '路由已更新，但协调失败：{error}',
    'msg.routeDeleted': '路由已删除，配置已协调',
    'msg.routeDeleteFailed': '路由已删除，但协调失败：{error}',
    'msg.listenerSaved': '监听器已保存',
    'msg.listenerUpdated': '监听器已更新并应用',
    'msg.listenerUpdateFailed': '监听器已更新，但协调失败：{error}',
    'msg.listenerDeleted': '监听器已删除',
    'msg.backendPoolSaved': '后端池已保存',
    'msg.backendPoolUpdated': '后端池已更新并应用',
    'msg.backendPoolUpdateFailed': '后端池已更新，但协调失败：{error}',
    'msg.backendPoolDeleted': '后端池已删除',
    'msg.routingChangeSaved': '已保存。完成路由编辑后，请点击右上角“应用待处理更改”。',
    'msg.logsRefreshed': '日志已刷新',
    'msg.confirmDeleteResource': '确定删除{type}“{name}”吗？',
    'msg.noBackendTargets': '请至少添加一个后端地址',
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
    'msg.routePrefilled': '已根据建议填入监听器、后端池和路由规则表单',
    'msg.noPublicRoutesAzure': '没有公网路由需要 Azure 协调',
    'msg.managedDnsNoRelativeName': '已跳过缺少相对名称的托管 DNS 记录',
    'msg.nsgSourcePolicy': 'NSG 优先级和源地址前缀策略',
    'msg.setMissingAzureSettings': '设置缺失的 Azure 配置，包括 DNS Zone 和入口公网 IP',
    'msg.runReconcileEnabledAzure': '运行“立即协调”以更新已启用的 Azure 资源',
    'msg.azureDefaultCredential': 'Azure 默认凭据链',
    'msg.azureDnsCapability': 'Azure DNS A 记录协调',
    'msg.azureNsgCapability': '虚拟机 NSG 监听端口协调',
    'msg.gatewayCannotBind': '不能将网关容器绑定为上游',
    'msg.invalidUpstreamScheme': '上游协议必须是 HTTP 或 HTTPS',
    'msg.securitySaved': '安全策略已保存并应用',
    'msg.securitySaveFailed': '安全策略已保存，但 Caddy 重新加载失败：{error}',
    'msg.settingsSaved': '设置已保存',
    'msg.settingsSavedRestart': '设置已保存。请重启 Caddy Proxy 以启用 Deployment 或 Azure 改动。',
    'msg.settingsUnavailable': '当前运行环境不支持设置持久化。',
    'msg.configurationExported': '配置压缩包已下载',
    'msg.configurationExportInvalid': '服务器返回的配置压缩包无效。请刷新页面后重试。',
    'msg.configurationImported': '已校验 {listeners} 个监听器、{pools} 个后端池、{rules} 条路由规则和 {subjects} 个证书域名。当前尚未持久化，请点击右上角“应用待处理更改”。',
    'msg.selectConfigurationArchive': '请先选择配置 ZIP 压缩包。',
    'msg.applyImportedFirst': '请先应用已导入的配置，再修改设置或证书策略。',
    'settings.deploymentNote': 'Deployment 会影响 Docker 发现和 Azure 客户端。此处保存为进程下次启动配置；网络模式、Socket 挂载和端口发布也必须符合所选部署方式。',
    'settings.azureIdentityNote': 'Azure 使用 DefaultAzureCredential。在 Azure VM 上，请为托管身份授予托管 Zone 的 DNS Zone Contributor，以及目标 NSG 的 Network Contributor；此处不保存客户端密钥。',
    'settings.azureStatusNote': '“协调并应用”会写入期望 A 记录、列出托管 A 记录用于清理，并等待 NSG 规则操作完成；平台页显示最近一次数量、警告和 Azure API 错误。“配置完整”只校验输入，不代表身份权限已经验证。',
    'settings.azurePermissionCheckNote': '只读检查当前运行进程在 ARM 中的有效操作权限，不会写入资源。在 Azure VM 上通常检查其托管身份；本地开发时可能使用 Azure CLI 或 DefaultAzureCredential 选择的其他身份。',
    'settings.azurePermissionsTitle': '有效权限',
    'settings.azurePermissionMetadata': '使用 {credential} 检查于 {time}',
    'settings.azureDnsPermissions': 'DNS Zone',
    'settings.azureNetworkPermissions': '网络安全组',
    'settings.azurePermissionNotConfigured': '当前表单未启用此项能力。',
    'settings.azurePermissionResourceGroup': '资源组：{resourceGroup}',
    'settings.azurePermissionMissing': '缺少操作：{actions}',
    'settings.configurationDescription': '在 Caddy Proxy 实例之间迁移路由、Console 设置和证书申请策略。',
    'settings.configurationExportTitle': '导出',
    'settings.configurationExportNote': '下载 caddyproxy_config_yyyymmdd.zip；不包含已签发证书、私钥、运行日志、审计日志及任何秘密字段。',
    'settings.configurationImportTitle': '导入',
    'settings.configurationImportNote': 'ZIP 会先完成解压、规范化和 Caddy 渲染检查，再载入内存草稿。应用后才写入本地并启用路由与证书策略；Deployment 和 Azure 启动期设置需重启生效。',
    'settings.configurationValidatedTitle': '已校验的导入内容',
    'settings.configurationPending': '已导入的配置通过校验，正在等待应用。在此之前不能保存设置或证书策略变更。',
    'msg.protectedPolicyRequired': '请至少保留一种受保护路由令牌 Header。',
    'dashboard.reconcileIssue': '最近一次协调未能完整应用生成的配置。',
    'dashboard.noRoutes': '当前没有已保存路由。添加路由后即可发布服务。',
    'dashboard.dockerIssue': 'Docker 发现需要处理：{details}',
    'dashboard.azureIssue': 'Azure 集成已启用但配置不完整：{details}',
    'routes.pendingApply': '路由更改已保存但尚未生效。完成编辑后，请点击右上角“应用待处理更改”。',
    'logs.allLevels': '全部级别',
    'logs.allSources': '全部来源',
    'logs.time': '时间',
    'logs.level': '级别',
    'logs.source': '来源',
    'logs.message': '消息',
    'logs.fields': '详情',
    'logs.searchPlaceholder': '消息、来源或详情',
    'certificates.storage': 'Caddy 存储目录',
    'certificates.scannedAt': '扫描时间',
    'certificates.expires': '过期时间',
    'certificates.renewalStarts': '续期窗口开始',
    'certificates.issuer': '签发者',
    'certificates.certificateFile': '证书文件',
    'certificates.privateKeyFile': '私钥文件',
    'certificates.metadataFile': '元数据文件',
    'certificates.fingerprint': 'SHA-256 指纹',
    'certificates.subjects': '证书名称',
    'certificates.noCertificates': 'Caddy 存储中尚未找到已签发的证书文件。',
    'certificates.runtimeUnavailable': '当前无法检查证书存储。',
    'certificates.policyNote': '保存会更新 Caddy 自动化策略；重新加载 TLS 只应用当前策略，不会强制发起 ACME 续期。',
    'certificates.earlyRenewalTitle': '将续期窗口设为证书有效期的 50%，并应用该策略。'
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
  ['VM NSG listener-port reconciliation', 'msg.azureNsgCapability'],
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
  ['upstream scheme must be http or https', 'msg.invalidUpstreamScheme'],
  ['protected routes require at least one token header policy', 'msg.protectedPolicyRequired'],
  ['apply the imported configuration before changing settings or certificate policy', 'msg.applyImportedFirst']
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
    settings: null,
    azurePermissionCheck: null,
    securityForm: emptySecurityForm(),
    systemForm: emptySystemForm(),
    routeSection: 'routingRules',
    editingListenerId: '',
    editingBackendPoolId: '',
    editingRoutingRuleId: '',
    resourceToDelete: null,
    status: null,
    containers: [],
    discoveryWarning: '',
    bindForms: {},
    certificateForm: emptyCertificateForm(),
    certificateRuntime: emptyCertificateRuntime(),
    logEntries: [],
    logsLoaded: false,
    logLevel: 'all',
    logSource: 'all',
    logQuery: '',
    configurationFileName: '',
    configurationImportResult: null,
    listenerForm: emptyListenerForm(),
    backendPoolForm: emptyBackendPoolForm(),
    routingRuleForm: emptyRoutingRuleForm(),
    navItems: [
      { view: 'dashboard', label: 'nav.dashboard' },
      { view: 'routes', label: 'nav.routes' },
      { view: 'discovery', label: 'nav.discovery' },
      { view: 'platform', label: 'nav.platform' },
      { view: 'certificates', label: 'nav.certificates' },
      { view: 'security', label: 'nav.security' },
      { view: 'settings', label: 'nav.settings' },
      { view: 'logs', label: 'nav.logs' }
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
      this.updateDocumentTitle();
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
      this.certificateRuntime = emptyCertificateRuntime();
      this.logEntries = [];
      this.logsLoaded = false;
      this.configurationFileName = '';
      this.configurationImportResult = null;
      this.settings = null;
      this.securityForm = emptySecurityForm();
      this.systemForm = emptySystemForm();
      this.certificateDirty = false;
      this.routeSection = 'routingRules';
      this.listenerForm = emptyListenerForm();
      this.backendPoolForm = emptyBackendPoolForm();
      this.routingRuleForm = emptyRoutingRuleForm();
      this.editingListenerId = '';
      this.editingBackendPoolId = '';
      this.editingRoutingRuleId = '';
      this.resourceToDelete = null;
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
        const [statusResult, discoveryResult, certificateResult, settingsResult] = await Promise.allSettled([
          this.api('/api/status'),
          this.api('/api/discovery/containers'),
          this.api('/api/certificate'),
          this.api('/api/settings')
        ]);
        const errors = [];
        if (statusResult.status === 'fulfilled') {
          this.status = statusResult.value;
          if (!this.configurationImportPending()) this.configurationImportResult = null;
          if (!this.discoveryVisible() && this.activeView === 'discovery') {
            this.activeView = 'dashboard';
            this.updateDocumentTitle();
          }
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
          else this.setCertificateRuntime(certificateResult.value.runtime);
        } else {
          errors.push(certificateResult.reason);
        }
        if (settingsResult.status === 'fulfilled') {
          this.setSettingsForms(settingsResult.value);
        } else {
          errors.push(settingsResult.reason);
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

    routingChangesPending() {
      return Boolean(this.status?.routingChangesPending);
    },

    applyButtonText() {
      return this.t(this.routingChangesPending() ? 'actions.applyPending' : 'actions.apply');
    },

    async saveListener() {
      const editingID = this.editingListenerId;
      const existing = editingID ? this.listeners().find((listener) => listener.id === editingID) : null;
      const listener = {
        ...(existing || {}),
        name: this.listenerForm.name,
        hostname: this.listenerForm.hostname,
        port: Number(this.listenerForm.port),
        protocol: this.listenerForm.protocol
      };
      await this.runAction(async () => {
        const result = await this.api(editingID ? '/api/listeners/' + encodeURIComponent(editingID) : '/api/listeners', {
          method: editingID ? 'PUT' : 'POST',
          body: JSON.stringify(listener)
        });
        const listenerID = result.listener.id;
        this.resetListenerForm();
        await this.refreshAll();
        if (!this.routingRuleForm.listenerId) this.routingRuleForm.listenerId = listenerID;
        this.showNotice(this.t('msg.routingChangeSaved'));
      });
    },

    beginEditListener(listener) {
      this.editingListenerId = listener.id;
      this.listenerForm = {
        name: listener.name || '',
        hostname: listener.hostname || '',
        port: Number(listener.port) || 443,
        protocol: listener.protocol || 'https'
      };
      this.focusFormHeading('#listener-form-heading');
    },

    resetListenerForm() {
      this.listenerForm = emptyListenerForm();
      this.editingListenerId = '';
    },

    onListenerProtocolChanged() {
      const port = Number(this.listenerForm.port);
      if (this.listenerForm.protocol === 'https' && port === 80) this.listenerForm.port = 443;
      if (this.listenerForm.protocol === 'http' && port === 443) this.listenerForm.port = 80;
    },

    async saveBackendPool() {
      const editingID = this.editingBackendPoolId;
      const existing = editingID ? this.backendPools().find((pool) => pool.id === editingID) : null;
      let targets = parseBackendTargets(this.backendPoolForm.targetsText);
      if (targets.length === 0) {
        this.showAlert(this.t('msg.noBackendTargets'));
        return;
      }
      const existingTargets = existing?.targets || [];
      targets = targets.map((target) => {
        const preserved = existingTargets.find((candidate) => candidate.address === target.address && Number(candidate.port || 0) === Number(target.port || 0));
        return { ...(preserved || {}), ...target };
      });
      const pool = { ...(existing || {}), name: this.backendPoolForm.name, targets };
      await this.runAction(async () => {
        const result = await this.api(editingID ? '/api/backend-pools/' + encodeURIComponent(editingID) : '/api/backend-pools', {
          method: editingID ? 'PUT' : 'POST',
          body: JSON.stringify(pool)
        });
        const backendPoolID = result.backendPool.id;
        this.resetBackendPoolForm();
        await this.refreshAll();
        if (!this.routingRuleForm.backendPoolId) this.routingRuleForm.backendPoolId = backendPoolID;
        this.showNotice(this.t('msg.routingChangeSaved'));
      });
    },

    beginEditBackendPool(pool) {
      this.editingBackendPoolId = pool.id;
      this.backendPoolForm = {
        name: pool.name || '',
        targetsText: (pool.targets || []).map((target) => formatBackendTarget(target)).join('\n')
      };
      this.focusFormHeading('#backend-pool-form-heading');
    },

    resetBackendPoolForm() {
      this.backendPoolForm = emptyBackendPoolForm();
      this.editingBackendPoolId = '';
    },

    backendTargetLabel(target) {
      return formatBackendTarget(target);
    },

    async saveRoutingRule() {
      const editingID = this.editingRoutingRuleId;
      const existing = editingID ? this.routingRules().find((rule) => rule.id === editingID) : null;
      const rule = {
        ...(existing || {}),
        name: this.routingRuleForm.name,
        listenerId: this.routingRuleForm.listenerId,
        backendPoolId: this.routingRuleForm.backendPoolId,
        backendPort: Number(this.routingRuleForm.backendPort),
        backendProtocol: this.routingRuleForm.backendProtocol,
        headers: routingRuleHeaders(existing?.headers, this.routingRuleForm.backendHostHeader),
        pathPrefix: this.routingRuleForm.pathPrefix,
        healthPath: this.routingRuleForm.healthPath,
        exposure: this.routingRuleForm.exposure,
        enabled: Boolean(this.routingRuleForm.enabled)
      };
      await this.runAction(async () => {
        const result = await this.api(editingID ? '/api/routing-rules/' + encodeURIComponent(editingID) : '/api/routing-rules', {
          method: editingID ? 'PUT' : 'POST',
          body: JSON.stringify(rule)
        });
        this.resetRoutingRuleForm();
        await this.refreshAll();
        this.showNotice(this.t('msg.routingChangeSaved'));
      });
    },

    beginEditRoutingRule(rule) {
      this.editingRoutingRuleId = rule.id;
      this.routingRuleForm = {
        name: rule.name || '',
        listenerId: rule.listenerId || '',
        backendPoolId: rule.backendPoolId || '',
        backendPort: Number(rule.backendPort) || 80,
        backendProtocol: rule.backendProtocol || 'http',
        backendHostHeader: headerValue(rule.headers, 'Host'),
        pathPrefix: rule.pathPrefix || '',
        healthPath: rule.healthPath || '',
        exposure: rule.exposure || 'public',
        enabled: Boolean(rule.enabled)
      };
      this.focusFormHeading('#routing-rule-form-heading');
    },

    resetRoutingRuleForm() {
      this.routingRuleForm = emptyRoutingRuleForm();
      this.editingRoutingRuleId = '';
    },

    canSaveRoutingRule() {
      return Boolean(this.routingRuleForm.listenerId && this.routingRuleForm.backendPoolId && Number(this.routingRuleForm.backendPort));
    },

    onBackendProtocolChanged() {
      const port = Number(this.routingRuleForm.backendPort);
      if (this.routingRuleForm.backendProtocol === 'https' && port === 80) this.routingRuleForm.backendPort = 443;
      if (this.routingRuleForm.backendProtocol === 'http' && port === 443) this.routingRuleForm.backendPort = 80;
    },

    requestDeleteResource(kind, resource) {
      this.resourceToDelete = { kind, resource };
      const dialog = this.$refs.deleteResourceDialog;
      if (dialog && !dialog.open) dialog.showModal();
    },

    cancelDeleteResource() {
      const dialog = this.$refs.deleteResourceDialog;
      if (dialog && dialog.open) dialog.close();
      this.resourceToDelete = null;
    },

    deleteResourceConfirmation() {
      const target = this.resourceToDelete;
      if (!target) return '';
      return this.format('msg.confirmDeleteResource', {
        type: this.t('resources.' + target.kind),
        name: target.resource.name || target.resource.hostname || target.resource.id
      });
    },

    async confirmDeleteResource() {
      const target = this.resourceToDelete;
      this.cancelDeleteResource();
      if (!target) return;
      const settings = {
        listener: { endpoint: '/api/listeners/', editingID: this.editingListenerId, reset: () => this.resetListenerForm(), success: 'msg.listenerDeleted' },
        backendPool: { endpoint: '/api/backend-pools/', editingID: this.editingBackendPoolId, reset: () => this.resetBackendPoolForm(), success: 'msg.backendPoolDeleted' },
        routingRule: { endpoint: '/api/routing-rules/', editingID: this.editingRoutingRuleId, reset: () => this.resetRoutingRuleForm(), success: 'msg.routeDeleted', failure: 'msg.routeDeleteFailed' }
      }[target.kind];
      if (!settings) return;
      await this.runAction(async () => {
        await this.api(settings.endpoint + encodeURIComponent(target.resource.id), { method: 'DELETE' });
        if (settings.editingID === target.resource.id) settings.reset();
        await this.refreshAll();
        this.showNotice(this.t('msg.routingChangeSaved'));
      });
    },

    focusFormHeading(selector) {
      this.$nextTick(() => {
        const heading = this.$root.querySelector(selector);
        if (heading) {
          heading.setAttribute('tabindex', '-1');
          heading.focus({ preventScroll: false });
        }
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
        await this.api('/api/discovery/bind', { method: 'POST', body: JSON.stringify(payload) });
        await this.refreshAll();
        this.showNotice(this.t('msg.routingChangeSaved'));
      });
    },

    async saveCertificate(successKey = 'msg.certificateSaved') {
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
        renewalWindowRatio: Number(this.certificateForm.renewalWindowRatio),
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
        this.showReconcileOutcome(result.reconcile, successKey, 'msg.certificateSaveFailed');
      });
    },

    async refreshCertificateStatus() {
      await this.runAction(async () => {
        const certificate = await this.api('/api/certificate');
        if (this.certificateDirty) this.setCertificateRuntime(certificate.runtime);
        else this.setCertificateForm(certificate);
        this.lastUpdated = new Date();
        this.showNotice(this.t('msg.certificateStatusRefreshed'));
      });
    },

    async enableEarlyRenewal() {
      if (this.certificateDirty) {
        this.showAlert(this.t('msg.unsavedCertificate'));
        return;
      }
      this.certificateForm.renewalWindowRatio = 0.5;
      await this.saveCertificate('msg.earlyRenewalEnabled');
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

    async saveSecuritySettings() {
      const payload = {
        security: {
          enabled: Boolean(this.securityForm.enabled),
          maxRequestBodyBytes: Math.max(0, Number(this.securityForm.maxRequestBodyMiB) || 0) * 1024 * 1024,
          deniedMethods: parseList(this.securityForm.deniedMethodsText).map((value) => value.toUpperCase()),
          deniedPathPrefixes: parseList(this.securityForm.deniedPathsText),
          allowedCidrs: parseList(this.securityForm.allowedCidrsText),
          blockedCidrs: parseList(this.securityForm.blockedCidrsText)
        },
        internalSourceRanges: parseList(this.securityForm.internalSourceRangesText),
        protectedRoutes: {
          allowBearerToken: Boolean(this.securityForm.allowBearerToken),
          allowAdminTokenHeader: Boolean(this.securityForm.allowAdminTokenHeader)
        }
      };
      await this.runAction(async () => {
        const result = await this.api('/api/settings/security', { method: 'PUT', body: JSON.stringify(payload) });
        this.setSettingsForms(result.settings);
        await this.refreshAll();
        this.showReconcileOutcome(result.reconcile, 'msg.securitySaved', 'msg.securitySaveFailed');
      });
    },

    azureFormPayload() {
      return {
        enabled: Boolean(this.systemForm.azureEnabled),
        manageDNS: Boolean(this.systemForm.azureManageDNS),
        manageNSG: Boolean(this.systemForm.azureManageNSG),
        subscriptionId: this.systemForm.azureSubscriptionId,
        resourceGroup: this.systemForm.azureResourceGroup,
        dnsZoneName: '',
        dnsZones: parseAzureDNSZones(this.systemForm.azureDNSZonesText, this.systemForm.azureResourceGroup),
        networkSecurityGroupResourceGroup: this.systemForm.azureNSGResourceGroup,
        networkSecurityGroupName: this.systemForm.azureNSGName,
        publicIpAddress: this.systemForm.azurePublicIP,
        nsgPriority: Number(this.systemForm.azureNSGPriority) || 120,
        nsgSourceAddressPrefixes: parseList(this.systemForm.azureNSGSourcesText)
      };
    },

    async checkAzurePermissions() {
      this.azurePermissionCheck = null;
      await this.runAction(async () => {
        this.azurePermissionCheck = await this.api('/api/settings/azure/permissions', {
          method: 'POST',
          body: JSON.stringify({ azure: this.azureFormPayload() })
        });
      });
    },

    async saveSystemSettings() {
      const newToken = this.systemForm.adminToken.trim();
      const payload = {
        deploymentMode: this.systemForm.deploymentMode,
        adminToken: newToken,
        azure: this.azureFormPayload()
      };
      await this.runAction(async () => {
        const result = await this.api('/api/settings/system', { method: 'PUT', body: JSON.stringify(payload) });
        if (newToken) {
          this.token = newToken;
          this.loginToken = newToken;
          localStorage.setItem('gatewayToken', newToken);
        }
        this.setSettingsForms(result.settings);
        await this.refreshAll();
        const reconcileError = result.reconcile ? this.reconcileError(result.reconcile) : '';
        if (reconcileError) {
          this.showAlert(reconcileError);
        } else {
          this.showNotice(this.t(result.settings?.restartRequired ? 'msg.settingsSavedRestart' : 'msg.settingsSaved'));
        }
      });
    },

    configurationImportPending() {
      return Boolean(this.status?.configurationImportPending);
    },

    selectConfigurationArchive(event) {
      this.configurationFileName = event.target.files?.[0]?.name || '';
      this.configurationImportResult = null;
    },

    async exportConfiguration() {
      await this.runAction(async () => {
        const response = await fetch('/api/settings/configuration', {
          headers: { 'Authorization': 'Bearer ' + this.token }
        });
        if (!response.ok) throw await this.responseError(response);
        const data = await response.arrayBuffer();
        if (!validConfigurationArchive(data)) throw new Error(this.t('msg.configurationExportInvalid'));
        const disposition = response.headers.get('Content-Disposition') || '';
        const filenameMatch = disposition.match(/filename="?([^";]+)"?/i);
        const fallbackDate = new Date().toISOString().slice(0, 10).replaceAll('-', '');
        const download = document.createElement('a');
        const blob = new Blob([data], { type: 'application/zip' });
        const objectURL = URL.createObjectURL(blob);
        download.href = objectURL;
        download.download = filenameMatch?.[1] || `caddyproxy_config_${fallbackDate}.zip`;
        document.body.appendChild(download);
        download.click();
        download.remove();
        setTimeout(() => URL.revokeObjectURL(objectURL), 60000);
        this.showNotice(this.t('msg.configurationExported'));
      });
    },

    async importConfiguration() {
      const archive = this.$refs.configurationImportInput?.files?.[0];
      if (!archive) {
        this.showAlert(this.t('msg.selectConfigurationArchive'));
        return;
      }
      await this.runAction(async () => {
        const result = await this.api('/api/settings/configuration', {
          method: 'POST',
          headers: { 'Content-Type': 'application/zip' },
          body: archive
        });
        this.configurationImportResult = result;
        this.configurationFileName = '';
        this.$refs.configurationImportInput.value = '';
        await this.refreshAll({ forceCertificate: true });
        this.showNotice(this.format('msg.configurationImported', {
          listeners: result.listeners,
          pools: result.backendPools,
          rules: result.routingRules,
          subjects: result.certificateSubjects
        }));
      });
    },

    setSettingsForms(settings) {
      const source = settings || {};
      const security = source.security || {};
      const protectedRoutes = source.protectedRoutes || {};
      const azure = source.azure || {};
      this.settings = source;
      this.azurePermissionCheck = null;
      this.securityForm = {
        enabled: Boolean(security.enabled),
        maxRequestBodyMiB: (Number(security.maxRequestBodyBytes) || 0) / (1024 * 1024),
        deniedMethodsText: (security.deniedMethods || []).join('\n'),
        deniedPathsText: (security.deniedPathPrefixes || []).join('\n'),
        allowedCidrsText: (security.allowedCidrs || []).join('\n'),
        blockedCidrsText: (security.blockedCidrs || []).join('\n'),
        internalSourceRangesText: (source.internalSourceRanges || []).join('\n'),
        allowBearerToken: Boolean(protectedRoutes.allowBearerToken),
        allowAdminTokenHeader: Boolean(protectedRoutes.allowAdminTokenHeader)
      };
      this.systemForm = {
        deploymentMode: source.deploymentMode || this.status?.deploymentMode || 'container-socket',
        adminToken: '',
        azureEnabled: Boolean(azure.enabled),
        azureManageDNS: Boolean(azure.manageDNS),
        azureManageNSG: Boolean(azure.manageNSG),
        azureSubscriptionId: azure.subscriptionId || '',
        azureResourceGroup: azure.resourceGroup || '',
        azureDNSZonesText: formatAzureDNSZones(azure.dnsZones || [], azure.dnsZoneName, azure.resourceGroup),
        azureNSGResourceGroup: azure.networkSecurityGroupResourceGroup || azure.resourceGroup || '',
        azureNSGName: azure.networkSecurityGroupName || '',
        azurePublicIP: azure.publicIpAddress || '',
        azureNSGPriority: Number(azure.nsgPriority) || 120,
        azureNSGSourcesText: (azure.nsgSourceAddressPrefixes || ['*']).join('\n')
      };
    },

    activeDeploymentText() {
      const mode = this.settings?.activeDeploymentMode || this.status?.deploymentMode;
      const label = mode === 'azure-vm' ? this.t('deployment.azureVM') : this.t('deployment.containerSocket');
      return this.format('status.activeDeployment', { deployment: label });
    },

    azurePermissionGroups() {
      const result = this.azurePermissionCheck || {};
      return [
        { key: 'dns', label: this.t('settings.azureDnsPermissions'), configured: Boolean(result.dns?.configured), targets: result.dns?.targets || [] },
        { key: 'network', label: this.t('settings.azureNetworkPermissions'), configured: Boolean(result.network?.configured), targets: result.network?.targets || [] }
      ];
    },

    permissionGroupState(group) {
      if (!group.configured) return { text: this.t('status.notConfigured'), className: '' };
      if (!group.targets.length || group.targets.some((target) => target.error)) {
        return { text: this.t('status.unableToVerify'), className: 'error' };
      }
      if (group.targets.every((target) => target.granted)) {
        return { text: this.t('status.granted'), className: 'ok' };
      }
      return { text: this.t('status.missingPermissions'), className: 'warn' };
    },

    permissionTargetState(target) {
      if (target.granted) return { text: this.t('status.granted'), className: 'ok' };
      if (target.error) return { text: this.t('status.unableToVerify'), className: 'error' };
      return { text: this.t('status.missingPermissions'), className: 'warn' };
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

    async responseError(response) {
      const text = await response.text();
      let message = response.statusText;
      if (text) {
        try {
          message = JSON.parse(text)?.error || message;
        } catch {
          message = text;
        }
      }
      const error = new Error(message);
      error.status = response.status;
      return error;
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
        renewalWindowRatio: Number(source.renewalWindowRatio) || (1 / 3),
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
      this.setCertificateRuntime(source.runtime);
      this.certificateDirty = false;
    },

    setCertificateRuntime(runtime) {
      const source = runtime || {};
      this.certificateRuntime = {
        available: Boolean(source.available),
        storageDirectory: source.storageDirectory || '',
        scannedAt: source.scannedAt || '',
        certificates: Array.isArray(source.certificates) ? source.certificates : [],
        warnings: Array.isArray(source.warnings) ? source.warnings : [],
        error: source.error || ''
      };
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
      let upstream;
      try {
        upstream = new URL(policy.suggestedUpstream.replace(/^https?:/, form.scheme + ':'));
      } catch {
        this.showAlert(this.t('msg.invalidUpstreamScheme'));
        return;
      }
      const backendProtocol = upstream.protocol.replace(':', '');
      const backendPort = Number(upstream.port) || (backendProtocol === 'https' ? 443 : 80);
      this.resetListenerForm();
      this.resetBackendPoolForm();
      this.resetRoutingRuleForm();
      this.listenerForm = {
        name: form.host,
        hostname: form.host,
        protocol: form.https ? 'https' : 'http',
        port: form.https ? 443 : 80
      };
      this.backendPoolForm = {
        name: container.name || form.host,
        targetsText: upstream.hostname.replace(/^\[|\]$/g, '')
      };
      this.routingRuleForm = {
        ...emptyRoutingRuleForm(),
        name: container.name || form.host,
        backendPort,
        backendProtocol,
        exposure: form.exposure
      };
      this.routeSection = 'listeners';
      this.setActiveView('routes');
      this.showNotice(this.t('msg.routePrefilled'));
    },

    setRouteSection(section) {
      this.clearMessages();
      this.routeSection = section;
    },

    setActiveView(view) {
      this.clearMessages();
      this.activeView = view;
      this.updateDocumentTitle();
      if (view === 'logs') this.refreshLogs(false);
      this.$nextTick(() => {
        window.scrollTo({ top: 0, left: 0, behavior: 'auto' });
        const heading = this.$root.querySelector('.topbar h1');
        if (heading) {
          heading.setAttribute('tabindex', '-1');
          heading.focus({ preventScroll: true });
        }
      });
    },

    viewTitle() {
      const item = this.navItems.find((candidate) => candidate.view === this.activeView);
      return item ? this.t(item.label) : this.t('title');
    },

    visibleNavItems() {
      return this.navItems.filter((item) => item.view !== 'discovery' || this.discoveryVisible());
    },

    discoveryVisible() {
      return this.status?.deploymentMode !== 'azure-vm' || this.status?.docker?.enabled !== false;
    },

    deploymentLabel() {
      switch (this.status?.deploymentMode) {
        case 'container-socket':
          return this.t('deployment.containerSocket');
        case 'azure-vm':
          return this.t('deployment.azureVM');
        default:
          return this.status?.profile || '-';
      }
    },

    listenerPortHint() {
      return this.t(this.status?.deploymentMode === 'azure-vm' ? 'forms.listenerPortAzureHint' : 'forms.listenerPortContainerHint');
    },

    healthPathHint() {
      return this.format('forms.healthPathHint', { path: this.status?.health?.defaultPath || '/' });
    },

    exposureHint() {
      return this.t('exposureHint.' + (this.routingRuleForm.exposure || 'public'));
    },

    viewDescription() {
      return this.t('views.' + this.activeView + 'Description');
    },

    updateDocumentTitle() {
      document.title = this.viewTitle() + ' · ' + this.t('title');
    },

    subtitle() {
      if (!this.status) return this.t('app.loading');
      const updated = this.lastUpdated || new Date();
      return this.format('app.subtitle', { deployment: this.deploymentLabel(), time: updated.toLocaleString(this.locale) });
    },

    routes() {
      return this.status?.routes || [];
    },

    listeners() {
      return this.status?.listeners || [];
    },

    backendPools() {
      return this.status?.backendPools || [];
    },

    routingRules() {
      return this.status?.routingRules || [];
    },

    async refreshCurrentView() {
      if (this.activeView === 'logs') await this.refreshLogs();
      else await this.refreshAll();
    },

    async refreshLogs(showMessage = true) {
      await this.runAction(async () => {
        const [runtime, audit] = await Promise.all([
          this.api('/api/logs?limit=500'),
          this.api('/api/audit?limit=200')
        ]);
        const runtimeEntries = Array.isArray(runtime?.entries) ? runtime.entries : [];
        const auditEntries = (audit?.events || []).map((event) => ({
          time: event.time,
          source: 'audit',
          level: 'info',
          message: event.event,
          fields: event.fields || {}
        }));
        this.logEntries = [...runtimeEntries, ...auditEntries].sort((left, right) => new Date(right.time) - new Date(left.time));
        this.logsLoaded = true;
        this.lastUpdated = new Date();
        if (showMessage) this.showNotice(this.t('msg.logsRefreshed'));
      });
    },

    logSources() {
      return [...new Set(this.logEntries.map((entry) => entry.source).filter(Boolean))].sort();
    },

    filteredLogEntries() {
      const query = this.logQuery.trim().toLowerCase();
      return this.logEntries.filter((entry) => {
        const levelMatches = this.logLevel === 'all' || String(entry.level || 'info').toLowerCase() === this.logLevel;
        const sourceMatches = this.logSource === 'all' || entry.source === this.logSource;
        const searchText = `${entry.message || ''} ${entry.source || ''} ${this.logFields(entry.fields)}`.toLowerCase();
        return levelMatches && sourceMatches && (!query || searchText.includes(query));
      });
    },

    logLevelClass(level) {
      const value = String(level || 'info').toLowerCase();
      return value === 'error' ? 'error' : value === 'warn' || value === 'warning' ? 'warn' : value === 'info' ? 'ok' : '';
    },

    logFields(fields) {
      return fields && Object.keys(fields).length ? JSON.stringify(fields, null, 2) : '';
    },

    certificateName(certificate) {
      return certificate?.subjects?.[0] || certificate?.certificateFile?.split('/').pop() || '-';
    },

    listenerLabel(id) {
      const listener = this.listeners().find((candidate) => candidate.id === id);
      if (!listener) return id || '-';
      return `${listener.name} · ${listener.protocol.toUpperCase()}://${listener.hostname}:${listener.port}`;
    },

    backendPoolLabel(id) {
      return this.backendPools().find((candidate) => candidate.id === id)?.name || id || '-';
    },

    routingRuleHealth(rule) {
      if (this.routingChangesPending()) return { text: this.t('status.pendingApply'), className: 'warn', detail: '' };
      const route = this.routes().find((candidate) => candidate.id === rule.id);
      return route ? this.routeHealth(route) : { text: this.t('status.unknown'), className: '', detail: '' };
    },

    dashboardActions() {
      if (!this.status) return [];
      const actions = [];
      const reconcile = this.lastReconcile();
      if (reconcile.finishedAt && (reconcile.error || reconcile.azure?.error || !reconcile.caddyLoaded)) {
        actions.push({ text: this.t('dashboard.reconcileIssue'), label: this.t('actions.retryReconcile'), command: 'reconcile', tone: 'warn' });
      }
      if (this.routes().length === 0) {
        actions.push({ text: this.t('dashboard.noRoutes'), label: this.t('actions.addRoute'), view: 'routes' });
      }
      const docker = this.status.docker || {};
      if (docker.enabled && !docker.active) {
        actions.push({
          text: this.format('dashboard.dockerIssue', { details: this.translateBackendText(docker.reason || this.t('status.unavailable')) }),
          label: this.t('actions.reviewDiscovery'),
          view: 'discovery',
          tone: 'warn'
        });
      }
      const azure = this.status.azure || {};
      if (azure.enabled && !azure.configured) {
        actions.push({
          text: this.format('dashboard.azureIssue', { details: this.listText(azure.missingSettings) }),
          label: this.t('actions.reviewPlatform'),
          view: 'platform',
          tone: 'warn'
        });
      }
      return actions;
    },

    runDashboardAction(action) {
      if (action.command === 'reconcile') {
        this.reconcile();
      } else if (action.view) {
        if (action.view === 'routes') this.routeSection = 'routingRules';
        this.setActiveView(action.view);
      }
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
        [this.t('details.profile')]: this.deploymentLabel(),
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
        [this.t('details.mode')]: this.deploymentLabel(),
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

    certificateState(certificate) {
      const states = {
        valid: { text: this.t('status.valid'), className: 'ok' },
        renewal_due: { text: this.t('status.renewalDue'), className: 'warn' },
        expired: { text: this.t('status.expired'), className: 'error' },
        not_yet_valid: { text: this.t('status.notYetValid'), className: 'warn' }
      };
      return states[certificate?.state] || { text: this.t('status.unknown'), className: '' };
    },

    formatCertificateRemaining(value) {
      const seconds = Number(value);
      if (!Number.isFinite(seconds)) return '-';
      const absolute = Math.abs(seconds);
      const unit = absolute >= 86400 ? 'day' : absolute >= 3600 ? 'hour' : 'minute';
      const divisor = unit === 'day' ? 86400 : unit === 'hour' ? 3600 : 60;
      const amount = seconds < 0 ? Math.floor(seconds / divisor) : Math.ceil(seconds / divisor);
      return new Intl.RelativeTimeFormat(this.locale, { numeric: 'always' }).format(amount, unit);
    },

    shortFingerprint(value) {
      const fingerprint = String(value || '');
      return fingerprint ? fingerprint.match(/.{1,2}/g).join(':') : '-';
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

function emptyListenerForm() {
  return { name: '', hostname: '', protocol: 'https', port: 443 };
}

function emptyBackendPoolForm() {
  return { name: '', targetsText: '' };
}

function emptyRoutingRuleForm() {
  return {
    name: '', listenerId: '', backendPoolId: '', backendPort: 80, backendProtocol: 'http',
    backendHostHeader: '', pathPrefix: '', healthPath: '', exposure: 'public', enabled: true
  };
}

function headerValue(headers, name) {
  const key = Object.keys(headers || {}).find((candidate) => candidate.toLowerCase() === name.toLowerCase());
  return key ? headers[key] : '';
}

function routingRuleHeaders(existing, backendHostHeader) {
  const headers = { ...(existing || {}) };
  Object.keys(headers).forEach((name) => {
    if (name.toLowerCase() === 'host') delete headers[name];
  });
  const value = String(backendHostHeader || '').trim();
  if (value) headers.Host = value;
  return headers;
}

function emptySecurityForm() {
  return {
    enabled: true, maxRequestBodyMiB: 10, deniedMethodsText: '', deniedPathsText: '',
    allowedCidrsText: '', blockedCidrsText: '', internalSourceRangesText: '',
    allowBearerToken: true, allowAdminTokenHeader: true
  };
}

function emptySystemForm() {
  return {
    deploymentMode: 'container-socket', adminToken: '', azureEnabled: false,
    azureManageDNS: true, azureManageNSG: true, azureSubscriptionId: '', azureResourceGroup: '',
    azureDNSZonesText: '', azureNSGResourceGroup: '', azureNSGName: '', azurePublicIP: '', azureNSGPriority: 120,
    azureNSGSourcesText: '*'
  };
}

function parseList(value) {
  return [...new Set(String(value || '').split(/[\n,]+/).map((item) => item.trim()).filter(Boolean))];
}

function parseAzureDNSZones(value, defaultResourceGroup) {
  return String(value || '').split('\n').map((line) => line.trim()).filter(Boolean).map((line) => {
    const [name, resourceGroup] = line.split('|', 2).map((part) => part.trim());
    return { name, resourceGroup: resourceGroup || defaultResourceGroup };
  });
}

function formatAzureDNSZones(zones, legacyZoneName, defaultResourceGroup) {
  const values = [...(zones || [])];
  if (legacyZoneName && !values.some((zone) => zone.name === legacyZoneName)) {
    values.push({ name: legacyZoneName, resourceGroup: defaultResourceGroup });
  }
  return values.map((zone) => zone.resourceGroup && zone.resourceGroup !== defaultResourceGroup ? `${zone.name} | ${zone.resourceGroup}` : zone.name).join('\n');
}

function parseBackendTargets(value) {
  return String(value || '').split(/[\n,]+/).map((line) => line.trim()).filter(Boolean).map((line) => {
    if (line.startsWith('[')) {
      const closingBracket = line.indexOf(']');
      if (closingBracket > 0) {
        const address = line.slice(1, closingBracket);
        const portText = line.slice(closingBracket + 1).replace(/^:/, '');
        const port = Number(portText);
        return portText && Number.isInteger(port) ? { address, port } : { address };
      }
    }
    const firstColon = line.indexOf(':');
    const lastColon = line.lastIndexOf(':');
    if (firstColon > 0 && firstColon === lastColon) {
      const port = Number(line.slice(lastColon + 1));
      if (Number.isInteger(port)) return { address: line.slice(0, lastColon), port };
    }
    return { address: line };
  });
}

function formatBackendTarget(target) {
  const address = String(target?.address || '');
  const port = Number(target?.port) || 0;
  if (!port) return address;
  return (address.includes(':') ? `[${address}]` : address) + ':' + port;
}

function emptyCertificateForm() {
  return {
    issuer: 'letsencrypt', email: '', staging: false, caDirectory: '', subjectsText: '', dnsProvider: '',
    renewalWindowRatio: 1 / 3,
    azureSubscriptionId: '', azureResourceGroup: '', azureAuthentication: 'managedidentity',
    azureTenantId: '', azureClientId: '', azureClientSecret: '', clientSecretConfigured: false,
    runtimeOnly: true, persisted: false
  };
}

function emptyCertificateRuntime() {
  return { available: false, storageDirectory: '', scannedAt: '', certificates: [], warnings: [], error: '' };
}
