# deploy-staging.ps1 — Build, deploy, and sync data for staging
# Run from: C:\Users\Dan\Desktop\Front Office Dynasty Sports\fantasy-baseball-go
#
# Strategy: dump existing DB (which has correct team names), restore to staging,
# then refresh player/transaction data from WordPress API.

$ErrorActionPreference = "Stop"
$SERVER = "root@178.128.178.100"
$REMOTE_DIR = "/root/app"

Write-Host "=== Step 1: Build Go binaries for Linux ===" -ForegroundColor Cyan

$env:GOOS = "linux"
$env:GOARCH = "amd64"

# Create bin directory
New-Item -ItemType Directory -Force -Path bin | Out-Null

# Main server binary
Write-Host "  Building server..."
go build -o server_linux ./cmd/api

# Only the sync tools that don't overwrite team names
$syncTools = @(
    "sync_players",
    "sync_transactions",
    "sync_bid_history",
    "sync_key_dates"
)

foreach ($tool in $syncTools) {
    Write-Host "  Building $tool..."
    go build -o "bin/${tool}_linux" ./cmd/$tool
}

# Reset env
Remove-Item Env:\GOOS
Remove-Item Env:\GOARCH

Write-Host "`n=== Step 2: Dump existing database ===" -ForegroundColor Cyan
Write-Host "  Saving current DB (teams, users, everything)..."
ssh $SERVER "docker exec fantasy_postgres pg_dump -U admin -d fantasy_db --clean --if-exists > ${REMOTE_DIR}/db_backup.sql"

Write-Host "`n=== Step 3: Upload to server ===" -ForegroundColor Cyan

# Upload server binary, templates, Docker files, migrations
scp server_linux "${SERVER}:${REMOTE_DIR}/server"
scp -r templates "${SERVER}:${REMOTE_DIR}/"
scp docker-compose.staging.yml "${SERVER}:${REMOTE_DIR}/"
scp Dockerfile "${SERVER}:${REMOTE_DIR}/"
scp -r migrations "${SERVER}:${REMOTE_DIR}/"

# Upload sync tools
ssh $SERVER "mkdir -p ${REMOTE_DIR}/sync-tools"
foreach ($tool in $syncTools) {
    scp "bin/${tool}_linux" "${SERVER}:${REMOTE_DIR}/sync-tools/${tool}"
}
ssh $SERVER "chmod +x ${REMOTE_DIR}/sync-tools/*"

Write-Host "`n=== Step 4: Start staging containers ===" -ForegroundColor Cyan

ssh $SERVER @"
cd ${REMOTE_DIR}
docker compose -f docker-compose.staging.yml down -v 2>/dev/null || true
docker compose -f docker-compose.staging.yml up -d --build
echo 'Waiting 10s for PostgreSQL to be ready...'
sleep 10
"@

Write-Host "`n=== Step 5: Restore database ===" -ForegroundColor Cyan
Write-Host "  Restoring existing data (teams, users, all correct names)..."
ssh $SERVER "docker exec -i fantasy_postgres psql -U admin -d fantasy_db < ${REMOTE_DIR}/db_backup.sql"

Write-Host "`n=== Step 6: Run migration 013 (if not already applied) ===" -ForegroundColor Cyan
ssh $SERVER "docker exec -i fantasy_postgres psql -U admin -d fantasy_db < ${REMOTE_DIR}/migrations/013_feature_batch.sql"

Write-Host "`n=== Step 7: Refresh data from WordPress ===" -ForegroundColor Cyan

$DB_URL = "postgres://admin:password123@localhost:5433/fantasy_db?sslmode=disable"

# Only run sync tools that update player/transaction data — NOT team names
$syncOrder = @(
    @("sync_players",      "Syncing 39,000+ players (this takes a few minutes)..."),
    @("sync_transactions", "Syncing transaction history..."),
    @("sync_bid_history",  "Syncing bid history..."),
    @("sync_key_dates",    "Syncing league dates...")
)

foreach ($step in $syncOrder) {
    $tool = $step[0]
    $msg = $step[1]
    Write-Host "  $msg" -ForegroundColor Yellow
    ssh $SERVER "cd ${REMOTE_DIR} && DATABASE_URL='${DB_URL}' ./sync-tools/${tool}"
}

Write-Host "`n=== Step 8: Open firewall port ===" -ForegroundColor Cyan
ssh $SERVER "ufw allow 8080/tcp 2>/dev/null; echo 'Port 8080 open'"

Write-Host "`n=== DONE ===" -ForegroundColor Green
Write-Host "Test your site at: http://178.128.178.100:8080" -ForegroundColor Green
Write-Host "Login with your WordPress username and password: dynasty2026" -ForegroundColor Green
