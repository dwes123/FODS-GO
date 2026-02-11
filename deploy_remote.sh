#!/bin/bash

# Usage: ./deploy_remote.sh user@ip_address

TARGET=$1

if [ -z "$TARGET" ]; then
    echo "Usage: ./deploy_remote.sh user@ip_address"
    echo "Example: ./deploy_remote.sh root@192.168.1.1"
    exit 1
fi

echo "🚀 Starting Deployment to $TARGET..."

# 1. Create directory
echo "📂 Creating remote directory..."
ssh $TARGET "mkdir -p /root/app"

# 2. Copy Files
echo "atk Copying files (this may take a minute)..."
# Using scp to copy everything, excluding some heavy/unneeded folders would be better with rsync
# but scp is more standard. We'll rely on a clean copy.
# Ideally use rsync: rsync -avz --exclude '.git' --exclude 'node_modules' --exclude 'web' . $TARGET:/root/app
# We will assume rsync might not be there for Windows users, so we use a tar pipe method or just scp specific folders.

# Let's try to copy specific necessary items to avoid bloat
FILES="cmd internal migrations templates Caddyfile Dockerfile docker-compose.prod.yml go.mod go.sum schema.sql"

for file in $FILES; do
    echo "   -> Sending $file..."
    scp -r $file $TARGET:/root/app/
done

# 3. Remote Setup & Launch
echo "🔥 Launching on Remote Server..."
ssh $TARGET "cd /root/app && 
    echo 'Creating .env if missing...' && 
    if [ ! -f .env ]; then echo 'DB_USER=admin' > .env; echo 'DB_PASSWORD=change_me_please' >> .env; fi && 
    echo 'Stopping old containers...' && 
    docker compose -f docker-compose.prod.yml down || true && 
    echo 'Building and Starting...' && 
    docker compose -f docker-compose.prod.yml up -d --build"

echo "✅ Deployment commands sent!"
echo "   NOTE: If this is the first run, you must initialize the database."
echo "   Run the following command manually if needed:"
echo "   ssh $TARGET 'cat /root/app/schema.sql | docker exec -i fantasy_postgres psql -U admin -d fantasy_db'"
