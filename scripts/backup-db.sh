#!/bin/bash
# backup-db.sh — Automated PostgreSQL backup with GitHub offsite storage
# Installed via cron: 0 4 * * * /root/app/scripts/backup-db.sh
#
# Keeps:
#   - Daily backups for 30 days (local + GitHub)
#   - Monthly backups forever (1st of each month)

set -euo pipefail

BACKUP_DIR="/root/backups"
REPO_DIR="/root/backups/git"
DATE=$(date +%Y-%m-%d)
MONTH_DAY=$(date +%d)
FILENAME="fantasy_db_${DATE}.sql.gz"

mkdir -p "$BACKUP_DIR"

# 1. Dump and compress
docker exec fantasy_postgres pg_dump -U admin fantasy_db | gzip > "${BACKUP_DIR}/${FILENAME}"

# 2. Clean up local backups older than 30 days (but keep monthly)
find "$BACKUP_DIR" -maxdepth 1 -name "fantasy_db_*.sql.gz" -mtime +30 ! -name "fantasy_db_*-01.sql.gz" -delete

# 3. Push to GitHub
if [ -d "$REPO_DIR/.git" ]; then
    cd "$REPO_DIR"

    # Copy today's backup
    cp "${BACKUP_DIR}/${FILENAME}" .

    # Remove daily backups older than 30 days from git (keep monthly)
    find . -maxdepth 1 -name "fantasy_db_*.sql.gz" -mtime +30 ! -name "fantasy_db_*-01.sql.gz" -exec git rm -f {} \; 2>/dev/null || true

    git add -A
    git commit -m "Backup ${DATE}" 2>/dev/null || true
    git push origin main 2>/dev/null || true
fi

echo "[$(date)] Backup complete: ${FILENAME}"
