# 🚀 Deployment Guide - Fantasy Baseball Go

This guide will help you deploy your application to a DigitalOcean Droplet using Docker.

## 1. Prerequisites

1.  **DigitalOcean Account**: You need an active account.
2.  **Domain Name**: You need a domain name (e.g., `frontofficedynastysports.com`) pointing to your Droplet's IP address.
3.  **SSH Client**: You need to be able to SSH into your server. (Windows PowerShell `ssh` or Git Bash works).

## 2. Server Setup (DigitalOcean)

1.  **Create a Droplet**:
    *   Choose **Marketplace** -> **Docker** (This comes with Docker and Docker Compose pre-installed).
    *   Choose a plan (Basic Regular with 1GB RAM is the absolute minimum, 2GB recommended for Postgres + Go).
    *   Select your region.
    *   **Authentication**: Add your SSH Key.
    *   **Create Droplet**.

2.  **Configure DNS**:
    *   Go to your domain registrar (GoDaddy, Namecheap, etc.) or DigitalOcean Networking.
    *   Create an **A Record**:
        *   Host: `@` (or `www`)
        *   Value: `YOUR_DROPLET_IP_ADDRESS`
    *   *Note: DNS propagation can take up to 48 hours, but usually takes minutes.*

3.  **Update Caddyfile**:
    *   Open `Caddyfile` in your project.
    *   Ensure the domain name matches YOUR domain.
    *   Example:
        ```caddy
        your-domain.com {
            reverse_proxy app:8080
        }
        ```

## 3. Configuration

1.  **Environment Variables**:
    *   You will need to set `DB_USER` and `DB_PASSWORD` on the server.
    *   We will do this in the deployment step.

## 4. Deploying (From your Local Machine)

We have created a helper script `deploy_remote.sh` to automate the process.

### Method A: Using Git Bash / Terminal (Recommended)

1.  Open your terminal in the project root.
2.  Make the script executable:
    ```bash
    chmod +x deploy_remote.sh
    ```
3.  Run the script with your server IP:
    ```bash
    ./deploy_remote.sh root@YOUR_DROPLET_IP
    ```

### Method B: Manual Steps

If you cannot run the script, here is what it does manually:

1.  **Copy Files to Server**:
    ```powershell
    scp -r . root@YOUR_IP:/root/app
    ```
2.  **SSH into Server**:
    ```powershell
    ssh root@YOUR_IP
    ```
3.  **Setup Environment (On Server)**:
    ```bash
    cd /root/app
    echo "DB_USER=admin" > .env
    echo "DB_PASSWORD=secure_password_here" >> .env
    ```
4.  **Run Docker Compose**:
    ```bash
    docker compose -f docker-compose.prod.yml up -d --build
    ```
5.  **Initialize Database**:
    *   We need to run the schema and migrations.
    ```bash
    # Enter the DB container
    docker exec -i fantasy_postgres psql -U admin -d fantasy_db < schema.sql
    
    # Run migrations (loop through them)
    for f in migrations/*.sql; do
        docker exec -i fantasy_postgres psql -U admin -d fantasy_db < "$f"
    done
    ```

## 5. Verification

1.  Visit `https://your-domain.com`.
2.  Caddy will automatically provision an SSL certificate.
3.  You should see your login page.

## Troubleshooting

*   **View Logs**: `docker compose -f docker-compose.prod.yml logs -f`
*   **Database Connection**: Ensure `DB_USER` and `DB_PASSWORD` in `.env` match what you expect.
