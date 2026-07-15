using './main.bicep'

param image = 'pczhao1210/caddy-reverse-proxy:latest'
param adminToken = readEnvironmentVariable('GATEWAY_ADMIN_TOKEN')