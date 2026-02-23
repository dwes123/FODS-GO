# Promote staging to production (frontofficedynastysports.com)
# Usage: .\promote-production.ps1

$ErrorActionPreference = "Stop"

Write-Host "Promoting staging to production..." -ForegroundColor Cyan
ssh root@178.128.178.100 "cp /root/app/staging/server /root/app/server && cp -r /root/app/staging/templates/* /root/app/templates/ && mkdir -p /root/app/static && cp -r /root/app/staging/static/* /root/app/static/"

Write-Host "Restarting production container..." -ForegroundColor Cyan
ssh root@178.128.178.100 "cd /root/app && nohup docker compose -f docker-compose.prod.yml up -d --build app > /tmp/docker_build.log 2>&1 &"

Write-Host ""
Write-Host "Done! Live at https://frontofficedynastysports.com" -ForegroundColor Green
Write-Host "(Check /tmp/docker_build.log on server if issues)" -ForegroundColor Yellow
