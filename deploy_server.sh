#!/bin/bash
set -e

echo "🚀 Starting Automated Deployment..."

# 1. Clean up old mess
echo "🧹 Cleaning up old files..."
rm -rf /root/app/web
rm -rf /root/app/.git
rm -rf /root/app/node_modules

# 2. Fix Dockerfile (Force correct Go version)
echo "🛠️  Creating correct Dockerfile..."
cat <<EOF > /root/app/Dockerfile
# Build Stage
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
# FORCE GO version to match container
RUN sed -i 's/go 1.25.7/go 1.23.0/' go.mod
RUN go mod tidy
RUN go mod download
COPY . .
RUN go build -o server ./cmd/api

# Run Stage
FROM alpine:latest
WORKDIR /root/
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/server .
COPY --from=builder /app/templates ./templates
EXPOSE 8080
CMD ["./server"]
EOF

# 3. Create Environment File
echo "🔐 Setting up environment..."
echo "DB_USER=admin" > /root/app/.env
echo "DB_PASSWORD=password123" >> /root/app/.env

# 4. Launch
echo "🔥 Launching Application..."
cd /root/app
docker compose -f docker-compose.prod.yml down
docker compose -f docker-compose.prod.yml up -d --build

# 5. Import Data (Only if needed)
if [ -f "dump.sql" ]; then
    echo "📦 Importing Database..."
    # Wait for DB to be ready
    sleep 10
    cat dump.sql | docker exec -i app-db-1 psql -U admin -d fantasy_db > /dev/null 2>&1 || echo "DB Import skipped (errors or already exists)"
fi

echo "✅ DEPLOYMENT COMPLETE! Visit http://178.128.178.100"
