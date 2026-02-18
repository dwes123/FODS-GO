# Fantasy Baseball Go — CLAUDE.md

## Project Summary

This is a full Go rebuild of the live WordPress/PHP fantasy baseball platform at [frontofficedynastysports.com](https://frontofficedynastysports.com). The PHP site is currently live; this Go codebase will replace it once feature parity is reached.

The platform manages dynasty fantasy baseball leagues (MLB, AAA, AA, High-A) with 80+ teams, 39,000+ players, and complex contract/financial systems.

## Infrastructure

- **Live PHP site:** https://frontofficedynastysports.com (IP: 178.128.178.100)
- **Hosting:** DigitalOcean Droplet, Ubuntu 24.04
- **Stack:** Go 1.24, PostgreSQL 15, Caddy (reverse proxy/SSL), Docker Compose
- **Git Backup:** https://github.com/dwes123/FODS-GO.git
- **DB Backups:** https://github.com/dwes123/fods-db-backup (private, daily at 4 AM UTC)
- **DB container:** `fantasy_postgres` — DB: `fantasy_db`, User: `admin`, Password: `password123`

## Build & Deploy Commands

```powershell
# Build Linux binary (from PowerShell on Windows)
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o server_linux ./cmd/api

# Run locally for development
go run ./cmd/api

# Deploy: SCP binary + templates to server
scp server_linux root@178.128.178.100:/root/app/server
scp -r templates root@178.128.178.100:/root/app/

# Restart on server (MUST use --build to pick up new binary; restart alone reuses old image)
ssh root@178.128.178.100 "cd /root/app && nohup docker compose -f docker-compose.prod.yml up -d --build app > /tmp/docker_build.log 2>&1 &"
# Note: run via nohup because SSH may drop during long builds; check /tmp/docker_build.log for status

# Access production DB
ssh root@178.128.178.100 "docker exec -it fantasy_postgres psql -U admin -d fantasy_db"

# Restore DB from backup
scp root@178.128.178.100:/root/backups/fantasy_db_YYYY-MM-DD.sql.gz .
gunzip fantasy_db_YYYY-MM-DD.sql.gz
ssh root@178.128.178.100 "docker exec -i fantasy_postgres psql -U admin -d fantasy_db" < fantasy_db_YYYY-MM-DD.sql
```

## Backups

- **Script:** `scripts/backup-db.sh` — installed at `/root/app/scripts/backup-db.sh` on server
- **Schedule:** Daily at 4 AM UTC via cron (`/var/log/fods-backup.log`)
- **Local retention:** `/root/backups/` — 30 days of dailies, monthly backups (1st of month) kept forever
- **Offsite:** Pushed to private repo `dwes123/fods-db-backup` via SSH deploy key
- **Size:** ~2 MB per compressed dump
- **Deploy key:** ED25519 key at `/root/.ssh/id_ed25519` on server, added to GitHub repo as write-access deploy key

## Architecture

```
cmd/api/main.go          — Entry point: Gin router, CORS, workers, routes
internal/
  handlers/              — HTTP handlers (one file per feature area)
    admin.go             — Commissioner dashboard, player editor, dead cap, approvals, settings
    admin_tools.go       — Trade reversal, Fantrax toggle, FOD IDs, bid export, trade review
    auth.go              — Login, register (approval queue), logout, RenderTemplate
    bids.go              — Bid submission (year cap, min bid, IFA/MiLB window checks), bid history page
    contracts.go         — Team options (deadline enforced), extensions (deadline enforced), restructures
    moves.go             — Roster moves (dynamic limits, SP limit, 40-man, 26-man, option, IL, DFA, trade block)
    players.go           — Player profile, free agents, trade block page
    roster.go            — Roster page, depth chart save
    trades.go            — Trade center, new trade, submit, accept
  store/                 — Data access layer (raw SQL via pgx)
    bids.go              — GetBidHistory (shared by bid history page + CSV export)
    leagues.go           — League/team queries, league dates, league settings, date window helpers
    players.go           — Player queries, AppendRosterMove, GetTradeBlockPlayers
    teams.go             — Team roster queries, roster counts, SP count, salary summaries
    trades.go            — CreateTradeProposal (ISBP validation), AcceptTrade (ISBP validation), ReverseTrade, IsTradeWindowOpen
    users.go             — User CRUD, sessions, registration requests, GetTeamOwnerEmails
  middleware/auth.go     — Session-based auth middleware
  worker/
    bids.go              — Bid finalization (background)
    waivers.go           — Waiver expiry processing with DFA clear actions + dead cap
    seasonal.go          — Hourly checks: option reset (Nov 1), IL clear (Oct 15)
    hr_monitor.go        — MLB Stats API poller for home run Slack alerts
  notification/
    slack.go             — Slack message posting
    email.go             — SMTP email (env-var configured, gracefully skips if unconfigured)
  db/database.go         — PostgreSQL connection pool
templates/               — HTML templates extending layout.html
migrations/              — Numbered SQL migration files
```

## Coding Conventions

### Handlers
- Signature: `func HandlerName(db *pgxpool.Pool) gin.HandlerFunc`
- Extract user: `user := c.MustGet("user").(*store.User)`
- Page render: `RenderTemplate(c, "template.html", gin.H{...})`
- JSON response: `c.JSON(http.StatusOK, gin.H{"message": "..."})`
- Admin check: `store.GetAdminLeagues(db, user.ID)` + `user.Role == "admin"` fallback

### Store Functions
- Signature: `func Name(db *pgxpool.Pool, params...) (ReturnType, error)`
- Always use `context.Background()` for DB operations
- Single row: `db.QueryRow(ctx, ...).Scan(&...)`
- Multiple rows: `db.Query(ctx, ...)` → `rows.Next()` → `rows.Scan()`
- Transactions: `db.Begin(ctx)` → `defer tx.Rollback(ctx)` → `tx.Commit(ctx)`

### Templates
- All extend `layout.html` via `{{define "content"}}...{{end}}`
- Template functions available: `dict`, `safeHTML`, `seq`, `formatMoney`
- CSS variables: `--fod-blue-primary: #2E6DA4`, `--fod-orange-accent: #E87426`
- Tables use class `fantasy-table-base`
- AJAX: vanilla `fetch()` with JSON body, no frameworks

### Routes
- Registered in `cmd/api/main.go` under `authorized := r.Group("/")` with `AuthMiddleware`
- Pattern: `authorized.GET("/path", handlers.Handler(database))`

## Database

- **League UUIDs** (hardcoded):
  - MLB: `11111111-1111-1111-1111-111111111111`
  - AAA: `22222222-2222-2222-2222-222222222222`
  - AA:  `33333333-3333-3333-3333-333333333333`
  - High-A: `44444444-4444-4444-4444-444444444444`
- **Contract columns:** `contract_2026` through `contract_2040` (TEXT — supports "$1000000", "TC", "ARB", "UFA")
- **Player status fields:** `status_40_man` (BOOL), `status_26_man` (BOOL), `status_il` (TEXT), `fa_status` (TEXT), `is_international_free_agent` (BOOL)
- **JSONB columns on players:** `bid_history`, `roster_moves_log`, `contract_option_years`
- **Nullable columns:** `owner_name` on teams, all `contract_` columns on players — always use COALESCE when scanning into Go strings
- **league_settings columns:** `luxury_tax_limit`, `roster_26_man_limit` (default 26), `roster_40_man_limit` (default 40), `sp_26_man_limit` (default 6)
- **league_dates date_type values:** `trade_deadline`, `opening_day`, `extension_deadline`, `option_deadline`, `ifa_window_open`, `ifa_window_close`, `milb_fa_window_open`, `milb_fa_window_close`, `roster_expansion_start`, `roster_expansion_end`

## Key Business Logic

- **Bid multipliers:** 1yr=2.0, 2yr=1.8, 3yr=1.6, 4yr=1.4, 5yr=1.2
- **Bid points:** `(years × AAV × multiplier) / 1,000,000`
- **Bid validation:** Contract length 1-5 years only, minimum $1M AAV, minimum 1.0 bid point
- **Extension pricing (WAR-based):** Base rates SP=3.3755, RP=5.0131, Hitter=2.8354; decay factors per year
- **Trade retention:** 50% salary retained by sending team
- **ISBP validation:** Balance checked at both proposal and acceptance time; cannot go negative
- **DFA dead cap:** 75% current year, 50% future years
- **Team option buyout:** 30% of option salary
- **Offseason:** Oct 15 – Mar 15 (trades always allowed)
- **Waiver period:** 48 hours from DFA
- **Roster limits:** Configurable per league/year via `league_settings` (default 26/40); SP limit on 26-man (default 6)
- **Roster expansion:** Optional date window in `league_dates` (`roster_expansion_start`/`roster_expansion_end`)
- **Deadline enforcement:** Extension deadline, team option deadline, IFA window, MiLB FA window — all configurable per league/year via `league_dates`

## Feature Implementation Status

### Core Features (original Go build)
Rosters, free agency/bidding, trades, waivers, arbitration, team options, financials, rotations, activity feed, commissioner dashboard, player editor, dead cap management, CSV importer, bug reports, Slack notifications, session auth

### Completed Feature Batch (18 features — all implemented)
1. **Extension Calculator** — WAR-based pricing on player profile (SP=3.3755, RP=5.0131, Hitter=2.8354, decay factors, $700K floor)
2. **Rule 5 Eligibility Display** — Shows `rule_5_eligibility_year` on player profile
3. **Roster Moves Log** — JSONB-backed per-player history, appended on every move, displayed on profile
4. **Bid History Page** — `/bids/history` with league/team filters, parses `bid_history` JSONB
5. **Trade Block Page** — `/trade-block` showing all players on the block, grouped by team
6. **Trade Reversal** — Commissioner tool: swaps players back, reverses ISBP, removes dead cap, sets status to REVERSED
7. **Fantrax Processing Toggle** — AJAX toggle on activity feed for commissioners
8. **FOD ID Generator** — Batch assigns FOD-XXXXX IDs using atomic `system_counters`
9. **Bid Export CSV** — `/admin/export-bids` downloads CSV of bid history
10. **Trade Deadline Enforcement** — `IsTradeWindowOpen()` checks `league_dates` table; offseason always open
11. **DFA Clear Actions** — Modal with "Release" (dead cap 75%/50%) or "Send to Minors" (off 40-man, stays on team)
12. **Depth Chart Sorting** — SortableJS drag-and-drop on roster page, saves `depth_rank` per player
13. **Account Approval Queue** — Registration creates pending request; admin approves at `/admin/approvals`
14. **Seasonal Workers** — Hourly: Nov 1 resets option years, Oct 15 clears IL statuses
15. **League Settings Page** — `/admin/settings` with trade deadline + opening day date pickers per league/year
16. **Email Notifications** — SMTP via env vars (`SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_FROM`); trade proposals email receiving team
17. **Slack HR Monitor** — Polls MLB Stats API every 30s during game hours (Apr-Oct, 1PM-midnight ET); posts to Slack on rostered player HRs

### Business Rules Enforcement (all implemented)
1. **Roster Expansion** — Configurable 26/40-man limits per league/year via `league_settings`; optional expansion window via `league_dates`
2. **ISBP Balance Validation** — Checked at trade proposal and acceptance; prevents negative balances
3. **Contract Year Cap** — FA bids limited to 1-5 years (server-side enforcement)
4. **Bid Minimum** — Requires $1M AAV minimum and 1.0 bid point minimum
5. **Extension Deadline** — Configurable per league via `league_dates`; blocks submissions after deadline
6. **IFA Signing Window** — Configurable open/close dates; blocks IFA bids outside window
7. **MiLB FA Window** — Configurable open/close dates; blocks MiLB FA bids outside window
8. **SP Limit on 26-Man** — Configurable per league (default 6); blocks SP promotions when at limit
9. **Team Option Deadline** — Configurable per league; blocks option decisions after deadline
10. **Option Years Highlighting** — Roster page highlights contract cells for team option years (orange accent)
11. **Admin Settings Expansion** — Settings page now includes all deadline/window date pickers and roster limit inputs per league

### Commissioner Tools Enhancements
- **Bid/FA Management in Player Editor** — Commissioners can manually set `fa_status`, pending bid fields, and `bid_type` on any player
- **IFA Toggle in Player Editor** — `is_international_free_agent` checkbox in Status section; IFA filter on free agents page uses this field
- **Bid History Tracking** — Every bid appends to `bid_history` JSONB; displayed on player profile as collapsible table

### Migrations Required
- `migrations/013_feature_batch.sql` — Adds: `transactions.fantrax_processed`, `players.fod_id`, `registration_requests` table, `league_dates` table, `system_counters` table
- `migrations/014_business_rules.sql` — Adds: `roster_26_man_limit`, `roster_40_man_limit`, `sp_26_man_limit` columns to `league_settings`

### Not Implemented (deferred)
- Draft Room (Feature 2) — complex real-time feature, deferred
- Mobile App (Feature 18) — separate project
- NBA Support (Feature 20) — separate product

## Gotchas

- **CRITICAL: Deploy with `--build`** — `docker compose restart` does NOT rebuild the image; always use `docker compose up -d --build app` to deploy new code. Run via `nohup` because SSH may drop during builds.
- **Connection pool deadlock** — Never make nested `db.Query` calls while iterating outer `rows`; collect results first, close rows, then do inner queries (see `league_financials.go` for example)
- `go.mod` says Go 1.24 but Dockerfile uses `golang:1.23-alpine` — binary is built locally so this only matters for on-server builds
- CORS is hardcoded to `localhost:3000` in main.go — needs production domain before Go site goes live
- **Production DB password** differs from dev default — check `/root/app/.env` on server for actual credentials; DB exposed on port 5433 externally
- Many `cmd/` utilities (`sync_players`, `import_teams`, etc.) reference the old WordPress API and may not work after PHP site is retired
- 60+ one-off SQL scripts in project root are migration artifacts — not part of the app
- Workers run in-process — if the server restarts, bid/waiver timers reset (no persistent job queue)
- Seasonal worker uses `system_counters` to prevent duplicate runs across restarts
- HR monitor requires `players.mlb_id` to be populated for cross-referencing with MLB Stats API
- Email notifications require SMTP env vars; silently disabled if not configured
- Registration now goes through approval queue — existing users are unaffected
