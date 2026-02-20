#!/bin/bash
# Refresh staging DB from production
# Usage: ssh root@178.128.178.100 "bash /root/app/scripts/refresh-staging-db.sh"
set -euo pipefail

CONTAINER="fantasy_postgres"

echo "Terminating staging DB connections..."
docker exec $CONTAINER psql -U admin -d postgres -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='fantasy_db_staging';" 2>/dev/null

echo "Dropping old staging DB..."
docker exec $CONTAINER psql -U admin -d postgres -c "DROP DATABASE IF EXISTS fantasy_db_staging;"

echo "Creating empty staging DB..."
docker exec $CONTAINER psql -U admin -d postgres -c "CREATE DATABASE fantasy_db_staging OWNER admin;"

echo "Cloning production data to staging..."
docker exec $CONTAINER bash -c 'pg_dump -U admin fantasy_db | psql -U admin fantasy_db_staging' > /dev/null

echo "Done! Staging DB refreshed from production."
