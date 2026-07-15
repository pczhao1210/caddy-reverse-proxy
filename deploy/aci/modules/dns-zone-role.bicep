param zoneName string
param principalId string

var dnsZoneContributorRoleId = subscriptionResourceId('Microsoft.Authorization/roleDefinitions', 'befefa01-2a29-4197-83a8-272ff33ce314')

resource dnsZone 'Microsoft.Network/dnsZones@2018-05-01' existing = {
  name: zoneName
}

resource roleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(dnsZone.id, principalId, dnsZoneContributorRoleId)
  scope: dnsZone
  properties: {
    principalId: principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: dnsZoneContributorRoleId
  }
}

output roleAssignmentId string = roleAssignment.id