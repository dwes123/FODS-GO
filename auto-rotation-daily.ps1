# Daily rotation check - runs Mon-Sat 9 AM Pacific
# Requires SSH tunnel: ssh -N -L 15433:localhost:5433 root@178.128.178.100

$ErrorActionPreference = "Stop"
$ProjectDir = "C:\Users\Dan\Desktop\Front Office Dynasty Sports\fantasy-baseball-go"
$LogFile = "$ProjectDir\auto_rotation_log.txt"

# Start SSH tunnel if not running
$tunnel = Get-Process -Name "ssh" -ErrorAction SilentlyContinue | Where-Object { $_.CommandLine -like "*15433*" }
if (-not $tunnel) {
    Write-Host "Starting SSH tunnel..."
    Start-Process ssh -ArgumentList "-N -L 15433:localhost:5433 root@178.128.178.100" -WindowStyle Hidden
    Start-Sleep -Seconds 3
}

$timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
Add-Content $LogFile "`n[$timestamp] Running daily check..."

try {
    Set-Location $ProjectDir
    $env:DATABASE_URL = "postgres://admin:password123@localhost:15433/fantasy_db"
    $output = & go run ./cmd/auto_rotation --mode=daily 2>&1
    $output | ForEach-Object { Write-Host $_; Add-Content $LogFile "  $_" }
    Add-Content $LogFile "[$timestamp] Daily check completed successfully"
} catch {
    $err = $_.Exception.Message
    Write-Host "ERROR: $err"
    Add-Content $LogFile "[$timestamp] ERROR: $err"
}
