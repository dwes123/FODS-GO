#!/bin/bash
set -e

echo "Removing problematic \\restrict line..."
grep -v '^\\\\restrict' /root/app/db_backup.sql > /tmp/db_clean.sql

echo "Copying dump into container..."
docker cp /tmp/db_clean.sql fantasy_postgres:/tmp/db_clean.sql

echo "Restoring database (this may take a few minutes)..."
docker exec fantasy_postgres psql -U admin -d fantasy_db -f /tmp/db_clean.sql 2>&1 | tail -5

echo "Checking counts..."
docker exec fantasy_postgres psql -U admin -d fantasy_db -c "SELECT 'teams' as tbl, count(*) FROM teams UNION ALL SELECT 'players', count(*) FROM players UNION ALL SELECT 'transactions', count(*) FROM transactions;"

echo "Done!"
