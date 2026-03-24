# Fantasy Baseball Go — CLAUDE.md

## Project Summary

Full Go rebuild of the live WordPress/PHP fantasy baseball platform at [frontofficedynastysports.com](https://frontofficedynastysports.com). PHP site stays live until Go reaches feature parity. Manages dynasty fantasy baseball leagues (MLB, AAA, AA, High-A) with 80+ teams, 39,000+ players, and complex contract/financial systems. **The four leagues are completely independent of each other** — different owners, rosters, trades, and transactions. What happens in one league has no connection to any other league. The same real-life player (by `mlb_id`) exists as separate player records in each league, owned by different teams.

## Infrastructure

- **Live PHP site:** https://frontofficedynastysports.com (IP: 178.128.178.100)
- **Hosting:** DigitalOcean Droplet, Ubuntu 24.04
- **Stack:** Go 1.24, PostgreSQL 15, Caddy (reverse proxy/SSL), Docker Compose
- **Git:** https://github.com/dwes123/FODS-GO.git
- **DB Backups:** https://github.com/dwes123/fods-db-backup (private, daily at 4 AM UTC via `scripts/backup-db.sh`)
- **DB container:** `fantasy_postgres` — DB: `fantasy_db`, User: `admin`, Password: `password123`

## Build & Deploy

```powershell
go run ./cmd/api                    # Run locally
.\deploy-staging.ps1                # Deploy to staging (app.frontofficedynastysports.com)
.\promote-production.ps1            # Promote staging → production
ssh root@178.128.178.100 "docker exec -it fantasy_postgres psql -U admin -d fantasy_db"  # Production DB
```

**Staging** uses isolated `fantasy_db_staging` DB with SMTP/Slack disabled. Refresh: `ssh root@178.128.178.100 "bash /root/app/scripts/refresh-staging-db.sh"`

**MCP Servers** (`.mcp.json`, gitignored): postgres via SSH tunnel (`ssh -N -L 15433:localhost:5433 root@178.128.178.100`), GitHub, IDE diagnostics.

## Architecture

```
cmd/api/main.go          — Entry point: Gin router, CORS, workers, routes, graceful shutdown
internal/
  handlers/              — HTTP handlers (one file per feature area)
  store/                 — Data access layer (raw SQL via pgx)
  middleware/            — Session auth + security headers
  worker/                — Background workers (bids, waivers, seasonal, HR monitor, stats, minor leaguer)
  notification/          — Slack + Brevo email
  db/database.go         — PostgreSQL connection pool
templates/               — HTML templates extending layout.html
migrations/              — Numbered SQL migration files
cmd/                     — Data sync tools (import_teams, sync_users_bulk, sync_team_ownership, sync_players, sync_transactions, sync_bid_history, sync_waivers, sync_site_settings)
```

## Coding Conventions

### Handlers
- Signature: `func HandlerName(db *pgxpool.Pool) gin.HandlerFunc`
- Extract user: `user := c.MustGet("user").(*store.User)`
- Page render: `RenderTemplate(c, "template.html", gin.H{...})`
- Admin check: `store.GetAdminLeagues(db, user.ID)` + `user.Role == "admin"` fallback
- 500-level errors: log with `fmt.Printf("ERROR [HandlerName]: %v\n", err)`, return generic message — never leak DB errors

### Store Functions
- Signature: `func Name(db *pgxpool.Pool, params...) (ReturnType, error)`
- Always use `context.Background()` for DB operations
- Transactions: `db.Begin(ctx)` → `defer tx.Rollback(ctx)` → `tx.Commit(ctx)`

### Templates
- Extend `layout.html` via `{{define "content"}}...{{end}}`
- Template functions: `dict`, `safeHTML`, `seq`, `formatMoney`, `add`, `sub`, `mul`, `min`
- CSS variables: `--fod-blue-primary: #2E6DA4`, `--fod-orange-accent: #E87426`
- Dark mode: `body.dark-mode` selectors; `theme_preference` column on users

### Routes
- Registered in `cmd/api/main.go` under `authorized := r.Group("/")` with `AuthMiddleware`

## Database

- **League UUIDs:** MLB=`11111111-...`, AAA=`22222222-...`, AA=`33333333-...`, High-A=`44444444-...`
- **Contract columns:** `contract_2026` through `contract_2040` (TEXT — "$1000000", "TC", "ARB", "ARB 1/2/3", "UFA")
- **Player status:** `status_40_man` (BOOL), `status_26_man` (BOOL), `status_il` (TEXT), `fa_status` (TEXT), `is_international_free_agent` (BOOL), `dfa_only` (BOOL), `is_minor_leaguer` (BOOL)
- **JSONB columns on players:** `bid_history`, `roster_moves_log`, `contract_option_years`
- **Nullable columns:** `owner_name` on teams, all `contract_` columns — always use COALESCE
- **Teams financial columns:** `isbp_balance`, `milb_balance` (NUMERIC 12,2)
- **league_settings:** `luxury_tax_limit`, `roster_26_man_limit` (default 26), `roster_40_man_limit` (default 40), `sp_26_man_limit` (default 6)
- **league_dates date_types:** `trade_deadline`, `opening_day`, `extension_deadline`, `option_deadline`, `ifa_window_open/close`, `milb_fa_window_open/close`, `roster_expansion_start/end`
- **players.mlb_id** (INTEGER) — real MLB player ID, non-unique (same person across leagues)
- **transactions.transaction_type:** `Added Player`, `Dropped Player`, `Roster Move`, `Trade`
- **trades.status:** `PROPOSED`, `ACCEPTED`, `REJECTED`, `REVERSED`, `COUNTERED`; `parent_trade_id` FK for counter chains
- **Team ownership:** Always use `team_owners` junction table, never `teams.user_id` (legacy)
- **PostgreSQL COALESCE:** `COALESCE(uuid_col::TEXT, 'text')` — must cast UUID

## Key Business Logic

- **Bid multipliers:** 1yr=2.0, 2yr=1.8, 3yr=1.6, 4yr=1.4, 5yr=1.2; points = `(years × AAV × multiplier) / 1,000,000`
- **Bid validation:** 1-5 years, $760K AAV min, 1.0 bid point min
- **IFA/MiLB bid increment:** Minimum bid = lesser of 2x current bid or current + $100K; full balance always accepted
- **Extension pricing (WAR-based):** SP=3.3755, RP=5.0131, Hitter=2.8354 with decay; blocks players with >1 ARB year
- **Trade retention:** Date-based (10/25/50%) auto-applied, then optional per-player 50% on remainder; `trade_items.retain_salary`
- **DFA dead cap:** 75% current year, 50% future years; **Team option buyout:** 30%
- **Roster limits:** Configurable per league/year via `league_settings`; SP limit on 26-man (default 6)
- **Rookie contract auto-assign:** $760,000 current year, TC, TC, ARB 1, ARB 2, ARB 3
- **Offseason:** Oct 15 – Mar 15 (trades always open); **Waiver period:** 48 hours
- **SP/RP position swap:** 14-day cooldown; "P" players can assign to SP/RP with no cooldown

## Commissioner AI Agent

- **Path:** `/admin/agent` — Gemini 2.0 Flash via Vertex AI with 47 function-calling tools
- **Backend:** `internal/handlers/agent.go`; GCP service account at `/root/app/service-account.json`
- **League scoping:** All tools filtered by commissioner's `league_roles`
- **Audit log:** Write tools logged to `agent_audit_log` table
- **Tool categories:** Player search/create/delete, team management, roster operations, contract updates, bid management, trade block, leaderboards/game logs, pending approvals (actions/registrations/player requests), deadlines/windows, rotation status, transaction log, Fantrax queue, bug reports, owner management

## Gotchas

- **Deploy with `--build`** — `docker compose restart` does NOT rebuild; deploy scripts handle this
- **Staging vs Production** — `app.` subdomain = staging (`fantasy_db_staging`), root domain = production (`fantasy_db`)
- **Connection pool deadlock** — Never nest `db.Query` calls while iterating outer `rows`; collect first, close, then inner query
- **Production DB password** differs from dev — check `/root/app/.env` on server
- **Workers run in-process** — if server restarts, bid/waiver timers reset (no persistent job queue)
- **Email** requires `BREVO_API_KEY` + `SMTP_FROM` env vars; uses HTTP API (DigitalOcean blocks SMTP)
- **`GetManagedTeams` is lightweight** — does NOT populate `.Players`; use `GetTeamWithRoster` when player data needed
- **`docker-compose.prod.yml`** must be manually SCPed when changed (deploy scripts only copy binary + templates)

## Data Sync (PHP → Go)

Run in order: `import_teams` → `sync_users_bulk` → `sync_team_ownership` → `sync_players` → `sync_transactions` → `sync_bid_history` → `sync_waivers` → `sync_site_settings`. Build as Linux binaries, SCP to server, run with production `DATABASE_URL`. These call the WordPress REST API and stop working after PHP retirement.
