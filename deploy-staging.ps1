# Deploy to staging (app.frontofficedynastysports.com)
# Usage: .\deploy-staging.ps1

$ErrorActionPreference = "Stop"

Write-Host "Building Linux binary..." -ForegroundColor Cyan
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o server_linux ./cmd/api
if ($LASTEXITCODE -ne 0) { Write-Host "Build failed!" -ForegroundColor Red; exit 1 }

Write-Host "Creating staging directory on server..." -ForegroundColor Cyan
ssh root@178.128.178.100 "mkdir -p /root/app/staging"

Write-Host "Uploading binary to staging..." -ForegroundColor Cyan
scp server_linux root@178.128.178.100:/root/app/staging/server

Write-Host "Uploading templates to staging..." -ForegroundColor Cyan
ssh root@178.128.178.100 "rm -rf /root/app/staging/templates"
scp -r templates root@178.128.178.100:/root/app/staging/templates

Write-Host "Restarting staging container..." -ForegroundColor Cyan
ssh root@178.128.178.100 "cd /root/app && docker compose -f docker-compose.prod.yml up -d --build app-staging"

Remove-Item server_linux -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Done! Preview at https://app.frontofficedynastysports.com" -ForegroundColor Green
