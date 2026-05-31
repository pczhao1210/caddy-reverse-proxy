@description('Azure region for the container group.')
param location string = resourceGroup().location

@description('Container group name.')
param name string = 'ai-docker-farm-gateway'

@description('Published gateway image.')
param image string

@secure()
@description('Initial admin token. Replace with OIDC in production.')
param adminToken string

@description('ACI DNS label. Use a globally unique value in the region.')
param dnsLabel string

@description('Optional host name that Caddy should route to the management UI.')
param managementHost string = ''

@description('Enable Azure DNS reconciliation from the gateway.')
param azureEnabled bool = false

@description('Azure subscription for DNS reconciliation.')
param azureSubscriptionId string = subscription().subscriptionId

@description('Resource group that contains the DNS zone.')
param azureResourceGroup string = resourceGroup().name

@description('Azure DNS zone name, for example example.com.')
param azureDnsZone string = ''

@description('Optional static public IP address for A records. Leave empty to let the gateway discover its egress IP.')
param publicIpAddress string = ''

resource identity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: '${name}-identity'
  location: location
}

resource containerGroup 'Microsoft.ContainerInstance/containerGroups@2023-05-01' = {
  name: name
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${identity.id}': {}
    }
  }
  properties: {
    osType: 'Linux'
    restartPolicy: 'Always'
    ipAddress: {
      type: 'Public'
      dnsNameLabel: dnsLabel
      ports: [
        {
          protocol: 'TCP'
          port: 80
        }
        {
          protocol: 'TCP'
          port: 443
        }
      ]
    }
    containers: [
      {
        name: 'platform'
        properties: {
          image: image
          resources: {
            requests: {
              cpu: 1
              memoryInGB: 1.5
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
              name: 'GATEWAY_MANAGEMENT_HOST'
              value: managementHost
            }
            {
              name: 'GATEWAY_AZURE_ENABLED'
              value: string(azureEnabled)
            }
            {
              name: 'GATEWAY_AZURE_SUBSCRIPTION_ID'
              value: azureSubscriptionId
            }
            {
              name: 'GATEWAY_AZURE_RESOURCE_GROUP'
              value: azureResourceGroup
            }
            {
              name: 'GATEWAY_AZURE_DNS_ZONE'
              value: azureDnsZone
            }
            {
              name: 'GATEWAY_AZURE_MANAGE_NSG'
              value: 'false'
            }
            {
              name: 'GATEWAY_PUBLIC_IP_ADDRESS'
              value: publicIpAddress
            }
          ]
        }
      }
    ]
  }
}

output fqdn string = containerGroup.properties.ipAddress.fqdn
output principalId string = identity.properties.principalId
