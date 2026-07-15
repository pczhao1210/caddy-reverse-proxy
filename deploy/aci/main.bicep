@description('Azure region. It must match the existing virtual network region.')
param location string = resourceGroup().location

@description('Base name used for the gateway resources.')
param name string = 'caddy-reverse-proxy'

@description('Published gateway image, including registry server and tag.')
param image string = 'pczhao1210/caddy-reverse-proxy:latest'

@secure()
@description('Initial management API token.')
param adminToken string

@description('Dedicated virtual network created for the gateway.')
param vnetName string = '${name}-vnet'

@description('Address prefix for the dedicated gateway virtual network.')
param vnetAddressPrefix string = '10.42.0.0/24'

@description('New, dedicated subnet name delegated to Azure Container Instances.')
param aciSubnetName string = 'snet-caddy-aci'

@description('Address prefix for the new ACI subnet. Use at least /28.')
param aciSubnetPrefix string = '10.42.0.0/28'

@description('Optional management host exposed through Caddy. Leave empty to keep management private on port 8080.')
param managementHost string = ''

@description('Azure DNS zones managed by the gateway. Each item requires name and resourceGroup.')
param dnsZones array = []

@description('Source IP and CIDR ranges allowed to use routes marked internal.')
param internalSourceRanges array = [
  '10.0.0.0/8'
  '172.16.0.0/12'
  '192.168.0.0/16'
  '127.0.0.0/8'
  '::1/128'
  'fc00::/7'
  'fe80::/10'
]

@description('Container CPU cores.')
@minValue(1)
param cpuCores int = 1

@description('Container memory in GB. Bicep integer values are used for predictable validation.')
@minValue(2)
param memoryInGB int = 2

@description('Optional Azure Container Registry name. Leave empty for a public image.')
param acrName string = ''

@description('Resource group containing the optional Azure Container Registry.')
param acrResourceGroupName string = resourceGroup().name

@description('Azure Files share quota in GB.')
@minValue(5)
param fileShareQuotaGB int = 10

var ingressPublicIPName = '${name}-ingress-pip'
var natPublicIPName = '${name}-nat-pip'
var natGatewayName = '${name}-nat'
var networkSecurityGroupName = '${name}-aci-nsg'
var loadBalancerName = '${name}-lb'
var backendPoolName = 'gateway-backend'
var healthProbeName = 'gateway-ready'
var identityName = '${name}-identity'
var containerGroupName = '${name}-aci'
var storageAccountName = 'st${take(uniqueString(subscription().id, resourceGroup().id, name), 22)}'
var fileShareName = 'gateway-data'

resource vnet 'Microsoft.Network/virtualNetworks@2024-05-01' = {
  name: vnetName
  location: location
  properties: {
    addressSpace: {
      addressPrefixes: [
        vnetAddressPrefix
      ]
    }
  }
}

resource ingressPublicIP 'Microsoft.Network/publicIPAddresses@2024-05-01' = {
  name: ingressPublicIPName
  location: location
  sku: {
    name: 'Standard'
  }
  properties: {
    publicIPAllocationMethod: 'Static'
  }
}

resource natPublicIP 'Microsoft.Network/publicIPAddresses@2024-05-01' = {
  name: natPublicIPName
  location: location
  sku: {
    name: 'Standard'
  }
  properties: {
    publicIPAllocationMethod: 'Static'
  }
}

resource natGateway 'Microsoft.Network/natGateways@2024-05-01' = {
  name: natGatewayName
  location: location
  sku: {
    name: 'Standard'
  }
  properties: {
    idleTimeoutInMinutes: 10
    publicIpAddresses: [
      {
        id: natPublicIP.id
      }
    ]
  }
}

resource networkSecurityGroup 'Microsoft.Network/networkSecurityGroups@2024-05-01' = {
  name: networkSecurityGroupName
  location: location
  properties: {
    securityRules: [
      {
        name: 'Allow-Internet-HTTP-HTTPS'
        properties: {
          priority: 100
          access: 'Allow'
          direction: 'Inbound'
          protocol: 'Tcp'
          sourcePortRange: '*'
          destinationPortRanges: [
            '80'
            '443'
          ]
          sourceAddressPrefix: 'Internet'
          destinationAddressPrefix: '*'
        }
      }
      {
        name: 'Allow-AzureLoadBalancer-Readiness'
        properties: {
          priority: 110
          access: 'Allow'
          direction: 'Inbound'
          protocol: 'Tcp'
          sourcePortRange: '*'
          destinationPortRange: '8080'
          sourceAddressPrefix: 'AzureLoadBalancer'
          destinationAddressPrefix: '*'
        }
      }
    ]
  }
}

resource aciSubnet 'Microsoft.Network/virtualNetworks/subnets@2024-05-01' = {
  parent: vnet
  name: aciSubnetName
  properties: {
    addressPrefix: aciSubnetPrefix
    delegations: [
      {
        name: 'aci-delegation'
        properties: {
          serviceName: 'Microsoft.ContainerInstance/containerGroups'
        }
      }
    ]
    natGateway: {
      id: natGateway.id
    }
    networkSecurityGroup: {
      id: networkSecurityGroup.id
    }
    serviceEndpoints: [
      {
        service: 'Microsoft.Storage'
      }
    ]
  }
}

resource identity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: identityName
  location: location
}

resource storageAccount 'Microsoft.Storage/storageAccounts@2023-05-01' = {
  name: storageAccountName
  location: location
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
  properties: {
    allowBlobPublicAccess: false
    allowSharedKeyAccess: true
    minimumTlsVersion: 'TLS1_2'
    supportsHttpsTrafficOnly: true
    publicNetworkAccess: 'Enabled'
  }
}

resource fileService 'Microsoft.Storage/storageAccounts/fileServices@2023-05-01' = {
  parent: storageAccount
  name: 'default'
}

resource fileShare 'Microsoft.Storage/storageAccounts/fileServices/shares@2023-05-01' = {
  parent: fileService
  name: fileShareName
  properties: {
    accessTier: 'TransactionOptimized'
    enabledProtocols: 'SMB'
    shareQuota: fileShareQuotaGB
  }
}

resource acr 'Microsoft.ContainerRegistry/registries@2023-07-01' existing = if (!empty(acrName)) {
  name: acrName
  scope: resourceGroup(acrResourceGroupName)
}

module acrPullRole 'modules/acr-pull-role.bicep' = if (!empty(acrName)) {
  name: 'acr-pull-${uniqueString(acrResourceGroupName, acrName)}'
  scope: resourceGroup(acrResourceGroupName)
  params: {
    registryName: acrName
    principalId: identity.properties.principalId
  }
}

module dnsZoneRoles 'modules/dns-zone-role.bicep' = [for zone in dnsZones: {
  name: 'dns-role-${uniqueString(zone.resourceGroup, zone.name)}'
  scope: resourceGroup(zone.resourceGroup)
  params: {
    zoneName: zone.name
    principalId: identity.properties.principalId
  }
}]

resource containerGroup 'Microsoft.ContainerInstance/containerGroups@2023-05-01' = {
  name: containerGroupName
  location: location
  identity: {
    type: 'SystemAssigned, UserAssigned'
    userAssignedIdentities: {
      '${identity.id}': {}
    }
  }
  properties: {
    osType: 'Linux'
    restartPolicy: 'Always'
    subnetIds: [
      {
        id: aciSubnet.id
      }
    ]
    ipAddress: {
      type: 'Private'
      ports: [
        {
          protocol: 'TCP'
          port: 80
        }
        {
          protocol: 'TCP'
          port: 443
        }
        {
          protocol: 'TCP'
          port: 8080
        }
      ]
    }
    imageRegistryCredentials: empty(acrName) ? [] : [
      {
        server: acr!.properties.loginServer
        identity: identity.id
      }
    ]
    containers: [
      {
        name: 'gateway'
        properties: {
          image: image
          resources: {
            requests: {
              cpu: cpuCores
              memoryInGB: memoryInGB
            }
          }
          ports: [
            {
              protocol: 'TCP'
              port: 80
            }
            {
              protocol: 'TCP'
              port: 443
            }
            {
              protocol: 'TCP'
              port: 8080
            }
          ]
          environmentVariables: [
            {
              name: 'GATEWAY_PROFILE'
              value: 'aci'
            }
            {
              name: 'GATEWAY_ADMIN_TOKEN'
              secureValue: adminToken
            }
            {
              name: 'GATEWAY_AUTH_REQUIRED'
              value: 'true'
            }
            {
              name: 'GATEWAY_DOCKER_ENABLED'
              value: 'false'
            }
            {
              name: 'GATEWAY_MANAGEMENT_HOST'
              value: managementHost
            }
            {
              name: 'GATEWAY_INTERNAL_SOURCE_RANGES'
              value: join(internalSourceRanges, ',')
            }
            {
              name: 'GATEWAY_AZURE_ENABLED'
              value: string(!empty(dnsZones))
            }
            {
              name: 'GATEWAY_AZURE_MANAGE_DNS'
              value: string(!empty(dnsZones))
            }
            {
              name: 'GATEWAY_AZURE_MANAGE_NSG'
              value: 'false'
            }
            {
              name: 'GATEWAY_AZURE_SUBSCRIPTION_ID'
              value: subscription().subscriptionId
            }
            {
              name: 'GATEWAY_AZURE_RESOURCE_GROUP'
              value: resourceGroup().name
            }
            {
              name: 'GATEWAY_AZURE_DNS_ZONES'
              value: string(dnsZones)
            }
            {
              name: 'GATEWAY_PUBLIC_IP_ADDRESS'
              value: ingressPublicIP.properties.ipAddress
            }
            {
              name: 'AZURE_CLIENT_ID'
              value: identity.properties.clientId
            }
          ]
          volumeMounts: [
            {
              name: 'gateway-data'
              mountPath: '/data'
              readOnly: false
            }
          ]
          livenessProbe: {
            httpGet: {
              path: '/livez'
              port: 8080
              scheme: 'Http'
            }
            initialDelaySeconds: 10
            periodSeconds: 15
            failureThreshold: 3
            timeoutSeconds: 3
          }
          readinessProbe: {
            httpGet: {
              path: '/readyz'
              port: 8080
              scheme: 'Http'
            }
            initialDelaySeconds: 5
            periodSeconds: 10
            failureThreshold: 3
            successThreshold: 1
            timeoutSeconds: 3
          }
        }
      }
    ]
    volumes: [
      {
        name: 'gateway-data'
        azureFile: {
          shareName: fileShare.name
          storageAccountName: storageAccount.name
          storageAccountKey: storageAccount.listKeys().keys[0].value
          readOnly: false
        }
      }
    ]
  }
  dependsOn: [
    acrPullRole
  ]
}

module certificateDnsZoneRoles 'modules/dns-zone-role.bicep' = [for zone in dnsZones: {
  name: 'certificate-dns-role-${uniqueString(zone.resourceGroup, zone.name)}'
  scope: resourceGroup(zone.resourceGroup)
  params: {
    zoneName: zone.name
    principalId: containerGroup.identity.principalId
  }
}]

resource loadBalancer 'Microsoft.Network/loadBalancers@2024-05-01' = {
  name: loadBalancerName
  location: location
  sku: {
    name: 'Standard'
  }
  properties: {
    frontendIPConfigurations: [
      {
        name: 'public-frontend'
        properties: {
          publicIPAddress: {
            id: ingressPublicIP.id
          }
        }
      }
    ]
    backendAddressPools: [
      {
        name: backendPoolName
        properties: {
          loadBalancerBackendAddresses: [
            {
              name: 'gateway-aci'
              properties: {
                ipAddress: containerGroup.properties.ipAddress.ip
                virtualNetwork: {
                  id: vnet.id
                }
              }
            }
          ]
        }
      }
    ]
    probes: [
      {
        name: healthProbeName
        properties: {
          protocol: 'Http'
          port: 8080
          requestPath: '/readyz'
          intervalInSeconds: 5
          numberOfProbes: 2
        }
      }
    ]
    loadBalancingRules: [
      {
        name: 'tcp-80'
        properties: {
          protocol: 'Tcp'
          frontendPort: 80
          backendPort: 80
          enableFloatingIP: false
          idleTimeoutInMinutes: 15
          enableTcpReset: true
          disableOutboundSnat: true
          frontendIPConfiguration: {
            id: resourceId('Microsoft.Network/loadBalancers/frontendIPConfigurations', loadBalancerName, 'public-frontend')
          }
          backendAddressPool: {
            id: resourceId('Microsoft.Network/loadBalancers/backendAddressPools', loadBalancerName, backendPoolName)
          }
          probe: {
            id: resourceId('Microsoft.Network/loadBalancers/probes', loadBalancerName, healthProbeName)
          }
        }
      }
      {
        name: 'tcp-443'
        properties: {
          protocol: 'Tcp'
          frontendPort: 443
          backendPort: 443
          enableFloatingIP: false
          idleTimeoutInMinutes: 15
          enableTcpReset: true
          disableOutboundSnat: true
          frontendIPConfiguration: {
            id: resourceId('Microsoft.Network/loadBalancers/frontendIPConfigurations', loadBalancerName, 'public-frontend')
          }
          backendAddressPool: {
            id: resourceId('Microsoft.Network/loadBalancers/backendAddressPools', loadBalancerName, backendPoolName)
          }
          probe: {
            id: resourceId('Microsoft.Network/loadBalancers/probes', loadBalancerName, healthProbeName)
          }
        }
      }
    ]
  }
}

output ingressPublicIPAddress string = ingressPublicIP.properties.ipAddress
output natPublicIPAddress string = natPublicIP.properties.ipAddress
output containerPrivateIPAddress string = containerGroup.properties.ipAddress.ip
output loadBalancerId string = loadBalancer.id
output delegatedSubnetId string = aciSubnet.id
output controlPlaneIdentityPrincipalId string = identity.properties.principalId
output certificateIdentityPrincipalId string = containerGroup.identity.principalId
output storageAccount string = storageAccount.name
output managementEndpoint string = empty(managementHost) ? 'http://${containerGroup.properties.ipAddress.ip}:8080' : 'https://${managementHost}'