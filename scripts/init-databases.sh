#!/bin/bash
# Postgres init script — runs once when the data volume is empty.
# Creates the secondary basketball database alongside the default fantasy_db.
#
# NOTE: This script runs only on a fresh volume. For an existing Postgres
# container/volume (i.e., production), the basketball DB must be created
# manually:
#
#   docker exec -it fantasy_postgres psql -U admin -c "CREATE DATABASE fantasy_basketball_db;"
#   docker exec -it fantasy_postgres psql -U admin -c "CREATE DATABASE fantasy_basketball_db_staging;"

set -e

DB_NBA_NAME="${DB_NBA_NAME:-fantasy_basketball_db}"
DB_NBA_STAGING_NAME="${DB_NBA_NAME}_staging"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
    CREATE DATABASE "$DB_NBA_NAME";
    CREATE DATABASE "$DB_NBA_STAGING_NAME";
    CREATE DATABASE fantasy_db_staging;
EOSQL

echo "Created databases: $DB_NBA_NAME, $DB_NBA_STAGING_NAME, fantasy_db_staging"
