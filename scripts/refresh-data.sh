#!/bin/bash
export DATABASE_URL="postgres://admin:de2d631d64e90a591cd712561a57327f2c46f0eb6f657b51@localhost:5433/fantasy_db?sslmode=disable"

echo "=== Syncing players from WordPress ==="
/root/app/sync-tools/sync_players

echo "=== Syncing transactions ==="
/root/app/sync-tools/sync_transactions

echo "=== Syncing bid history ==="
/root/app/sync-tools/sync_bid_history

echo "=== Done ==="
