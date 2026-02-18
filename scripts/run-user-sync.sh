#!/bin/bash
export DATABASE_URL="postgres://admin:password123@localhost:5433/fantasy_db?sslmode=disable"
echo "=== Syncing users from WordPress ==="
/root/app/sync-tools/sync_users_bulk
echo "=== Linking teams to users ==="
/root/app/sync-tools/bulk_link_teams
echo "=== Done ==="
