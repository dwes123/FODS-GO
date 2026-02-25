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
# Run locally for development
go run ./cmd/api

# Deploy to staging (preview at app.frontofficedynastysports.com)
.\deploy-staging.ps1

# Promote staging to production (frontofficedynastysports.com)
.\promote-production.ps1

# Access production DB
ssh root@178.128.178.100 "docker exec -it fantasy_postgres psql -U admin -d fantasy_db"

# Restore DB from backup
scp root@178.128.178.100:/root/backups/fantasy_db_YYYY-MM-DD.sql.gz .
gunzip fantasy_db_YYYY-MM-DD.sql.gz
ssh root@178.128.178.100 "docker exec -i fantasy_postgres psql -U admin -d fantasy_db" < fantasy_db_YYYY-MM-DD.sql
```

### Staging/Production Workflow
1. Make code changes locally
2. `.\deploy-staging.ps1` — builds, uploads to `/root/app/staging/`, restarts `app-staging` container
3. Test at `https://app.frontofficedynastysports.com`
4. `.\promote-production.ps1` — copies staging binary+templates to production, restarts `app` container
5. Verify at `https://frontofficedynastysports.com`

**Server layout:** Production at `/root/app/server` + `/root/app/templates/`, staging at `/root/app/staging/server` + `/root/app/staging/templates/`.

**Staging database:** `app-staging` uses an isolated `fantasy_db_staging` database (not the production `fantasy_db`). SMTP and Slack env vars are blanked out so staging won't send real notifications. To refresh staging data from production:
```bash
ssh root@178.128.178.100 "bash /root/app/scripts/refresh-staging-db.sh"
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
cmd/api/main.go          — Entry point: Gin router, CORS, workers, routes, graceful shutdown
internal/
  handlers/              — HTTP handlers (one file per feature area)
    admin.go             — Commissioner dashboard, player editor, dead cap, approvals, settings, Slack integration, balance editor
    admin_tools.go       — Trade reversal, Fantrax toggle, FOD IDs, bid export, trade review
    agent.go             — AI commissioner assistant (Gemini 2.0 Flash via Vertex AI) with tool-calling loop
    auth.go              — Login, register (approval queue), logout, RenderTemplate, formatMoney (comma-formatted)
    bids.go              — Bid submission (year cap, min bid, IFA/MiLB window checks), bid history page
    contracts.go         — Team options (deadline enforced), extensions (deadline enforced), restructures
    moves.go             — Roster moves (dynamic limits, SP limit, 40-man, 26-man, option, IL, DFA, trade block, rookie contract auto-assign)
    players.go           — Player profile, free agents, trade block page
    roster.go            — Roster page, depth chart save, IL summary table
    stats.go             — Pitching + hitting leaderboard handlers, game log AJAX, admin backfill
    trades.go            — Trade center, new trade (contract preview + salary impact), submit, accept, counter proposals
    waivers.go           — Waiver wire (league-filtered), waiver claims
    league_rosters.go    — League roster browser, bid calculator, commissioner waiver audit
    activity.go          — Transaction log with league + transaction type filters
    home.go              — Home page with waiver wire spotlight widget
  store/                 — Data access layer (raw SQL via pgx)
    bids.go              — GetBidHistory (shared by bid history page + CSV export)
    leagues.go           — League/team queries, league dates, league settings, date window helpers
    players.go           — Player queries, AppendRosterMove, GetTradeBlockPlayers
    teams.go             — Team roster queries, roster counts, SP count, salary summaries
    activity.go          — Transaction log: GetTransactionLog (league + type filters, COMPLETED only), GetDistinctTransactionTypes, LogActivity
    stats.go             — Fantasy points: scoring categories, daily player stats, leaderboards (pitching + hitting), game log, player points summary
    trades.go            — CreateTradeProposal (ISBP validation, counter support), AcceptTrade (ISBP validation, chain cleanup), GetTradeByID, ReverseTrade, IsTradeWindowOpen
    users.go             — User CRUD, sessions, registration requests, GetTeamOwnerEmails
  middleware/auth.go     — Session-based auth middleware
  middleware/security.go — Defense-in-depth security headers (X-Content-Type-Options, X-Frame-Options, Referrer-Policy)
  worker/
    bids.go              — Bid finalization (background)
    waivers.go           — Waiver expiry processing with DFA clear actions + dead cap
    seasonal.go          — Hourly checks: option reset (Nov 1), IL clear (Oct 15)
    hr_monitor.go        — MLB Stats API poller for home run Slack alerts
    stats.go             — MLB Stats API box score poller for pitching + hitting fantasy points
  notification/
    slack.go             — Slack message posting
    email.go             — SMTP email (env-var configured, gracefully skips if unconfigured)
  db/database.go         — PostgreSQL connection pool
templates/               — HTML templates extending layout.html
migrations/              — Numbered SQL migration files
cmd/
  import_teams/          — Sync teams from WP users' ACF managed_teams
  sync_users_bulk/       — Sync users from WP, link via wp_id
  sync_team_ownership/   — Populate team_owners from WP ACF data
  sync_players/          — Sync 39K+ players from WP playerdata CPT
  sync_transactions/     — Import activity feed from WP transaction CPT
  sync_bid_history/      — Reconstruct bid_history JSONB from transactions
  sync_waivers/          — Sync waiver status, end times, and claims from WP
  sync_site_settings/    — Sync ISBP, MILB balances, luxury tax from WP Site Settings (via fod-api-bridge plugin)
scripts/
  backup-db.sh           — Daily DB backup to local + GitHub (cron)
  refresh-staging-db.sh  — Clone production DB into fantasy_db_staging
deploy-staging.ps1       — Build + upload to staging + restart staging container
promote-production.ps1   — Copy staging to production + restart production container
docker-compose.prod.yml  — Production + staging services (app, app-staging, db, caddy)
Caddyfile                — Caddy routes: production (frontofficedynastysports.com → app), staging (app. → app-staging)
```

## Coding Conventions

### Handlers
- Signature: `func HandlerName(db *pgxpool.Pool) gin.HandlerFunc`
- Extract user: `user := c.MustGet("user").(*store.User)`
- Page render: `RenderTemplate(c, "template.html", gin.H{...})`
- JSON response: `c.JSON(http.StatusOK, gin.H{"message": "..."})`
- Admin check: `store.GetAdminLeagues(db, user.ID)` + `user.Role == "admin"` fallback
- 500-level errors: log with `fmt.Printf("ERROR [HandlerName]: %v\n", err)`, return generic message to client — never leak DB errors

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
- **Nav dropdowns:** Logged-in users see hover+click dropdowns (League, Roster, Free Agency, Trades) instead of flat links; non-logged-in users see flat links (Rosters, Free Agents, Standings, Login); classes: `.nav-dropdown`, `.nav-dropdown-label`, `.nav-dropdown-menu`; CSS `:hover` for desktop + JS click toggle (`.open` class) for Safari/touch compatibility; clicking outside closes open dropdowns

### Routes
- Registered in `cmd/api/main.go` under `authorized := r.Group("/")` with `AuthMiddleware`
- Pattern: `authorized.GET("/path", handlers.Handler(database))`

## Database

- **League UUIDs** (hardcoded):
  - MLB: `11111111-1111-1111-1111-111111111111`
  - AAA: `22222222-2222-2222-2222-222222222222`
  - AA:  `33333333-3333-3333-3333-333333333333`
  - High-A: `44444444-4444-4444-4444-444444444444`
- **Contract columns:** `contract_2026` through `contract_2040` (TEXT — supports "$1000000", "TC", "ARB", "ARB 1", "ARB 2", "ARB 3", "UFA")
- **Player status fields:** `status_40_man` (BOOL), `status_26_man` (BOOL), `status_il` (TEXT), `fa_status` (TEXT), `is_international_free_agent` (BOOL), `dfa_only` (BOOL)
- **JSONB columns on players:** `bid_history`, `roster_moves_log`, `contract_option_years`
- **Nullable columns:** `owner_name` on teams, all `contract_` columns on players — always use COALESCE when scanning into Go strings
- **Teams financial columns:** `isbp_balance` (NUMERIC 12,2), `milb_balance` (NUMERIC 12,2)
- **league_settings columns:** `luxury_tax_limit`, `roster_26_man_limit` (default 26), `roster_40_man_limit` (default 40), `sp_26_man_limit` (default 6)
- **league_integrations columns:** `slack_bot_token`, `slack_channel_transactions`, `slack_channel_completed_trades`, `slack_channel_stat_alerts`, `slack_channel_trade_block`
- **transactions.transaction_type values:** `Added Player`, `Dropped Player`, `Roster Move`, `Trade` (standardized in migration 020; all Go code uses these exact strings)
- **trades columns:** `parent_trade_id` (UUID, nullable FK to trades) — links counter proposals to their parent; `status` TEXT supports `PROPOSED`, `ACCEPTED`, `REJECTED`, `REVERSED`, `COUNTERED`
- **users columns:** `theme_preference` (VARCHAR(10), NOT NULL, DEFAULT 'light') — stores 'light' or 'dark'; scanned with `COALESCE` in all user queries
- **league_dates date_type values:** `trade_deadline`, `opening_day`, `extension_deadline`, `option_deadline`, `ifa_window_open`, `ifa_window_close`, `milb_fa_window_open`, `milb_fa_window_close`, `roster_expansion_start`, `roster_expansion_end`
- **players.mlb_id** (INTEGER, non-unique, indexed) — real MLB player ID for cross-referencing with MLB Stats API; same `mlb_id` shared across multiple player records (same person in different fantasy leagues); ~770 unique MLB IDs covering ~3,036 player records; used by stats worker and HR monitor
- **scoring_categories** — configurable scoring weights per stat; `stat_type` ('pitching'/'hitting'), `stat_key`, `display_name`, `points`, `is_active`; unique on `(stat_type, stat_key)`
- **daily_player_stats** — one row per player per game per stat type; `raw_stats` JSONB stores individual stat values; `fantasy_points` pre-calculated; unique on `(player_id, game_pk, stat_type)`; indexed on `game_date`, `player_id`, `team_id`, `league_id`, `mlb_id`
- **stats_processing_log** — tracks which dates have been processed per stat type; unique on `(game_date, stat_type)`; status 'completed' or 'error'

## Key Business Logic

- **Bid multipliers:** 1yr=2.0, 2yr=1.8, 3yr=1.6, 4yr=1.4, 5yr=1.2
- **Bid points:** `(years × AAV × multiplier) / 1,000,000`
- **Bid validation:** Contract length 1-5 years only, minimum $1M AAV, minimum 1.0 bid point
- **Extension pricing (WAR-based):** Base rates SP=3.3755, RP=5.0131, Hitter=2.8354; decay factors per year
- **Trade retention:** Two layers — (1) mandatory date-based retention (10%/25%/50% based on season timing) applied automatically, then (2) optional per-player 50% retention checkbox on trade form (applied on remainder after date-based); tracked via `trade_items.retain_salary` boolean; dead cap created for sending team; both sides of trade can retain
- **ISBP validation:** Balance checked at both proposal and acceptance time; cannot go negative
- **DFA dead cap:** 75% current year, 50% future years
- **Team option buyout:** 30% of option salary
- **Offseason:** Oct 15 – Mar 15 (trades always allowed)
- **Waiver period:** 48 hours from DFA
- **Roster limits:** Configurable per league/year via `league_settings` (default 26/40); SP limit on 26-man (default 6)
- **Roster expansion:** Optional date window in `league_dates` (`roster_expansion_start`/`roster_expansion_end`)
- **Deadline enforcement:** Extension deadline, team option deadline, IFA window, MiLB FA window — all configurable per league/year via `league_dates`
- **IFA signing:** International free agents use a separate ISBP-based signing flow — single dollar amount (no contract years/AAV/bid points); validates ISBP balance on bid, deducts on finalization; no contract written; IFA flag cleared on signing
- **Rookie contract auto-assign:** When a player with no current-year contract is promoted to 40-man or 26-man, `assignRookieContractIfEmpty()` automatically writes: $760,000 (current year), TC, TC, ARB 1, ARB 2, ARB 3 across 6 years; contract values "ARB 1"/"ARB 2"/"ARB 3" are displayed as text via `hasPrefix` template function
- **Trade counter proposals:** Receiver of a PROPOSED trade can counter instead of accept/reject; counter creates a new trade with `parent_trade_id` linking to the original, marks original as `COUNTERED`; roles flip (receiver becomes proposer); players/ISBP pre-populated from original with flipped sides; chainable — either side can keep countering; on accept, recursive CTE cleans up any stale PROPOSED trades in the chain

## Feature Implementation Status

### Core Features (original Go build)
Rosters, free agency/bidding, trades, waivers, arbitration, team options, financials, rotations, activity feed, commissioner dashboard, player editor, dead cap management, CSV importer, bug reports, Slack notifications, session auth

### Completed Feature Batch (18 features — all implemented)
1. **Extension Calculator** — WAR-based pricing on player profile (SP=3.3755, RP=5.0131, Hitter=2.8354, decay factors, $700K floor)
2. **Rule 5 Eligibility Display** — Shows `rule_5_eligibility_year` on player profile
3. **Roster Moves Log** — JSONB-backed per-player history, appended on every move, displayed on profile; all roster moves (promote 40/26-man, option, IL, activate, DFA) also log to the activity feed via `LogActivity`
4. **Bid History Page** — `/bids/history` with league/team filters, parses `bid_history` JSONB
5. **Trade Block Page** — `/trade-block` showing all players on the block, grouped by team
6. **Trade Reversal** — Commissioner tool: swaps players back, reverses ISBP, removes dead cap, sets status to REVERSED
7. **Fantrax Processing Toggle** — AJAX toggle on activity feed for commissioners
8. **~~FOD ID Generator~~** — Removed; all players have UUIDs natively via PostgreSQL
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

### Parity Gap Fixes (Feb 2026)
- **League Roster Browser** — `/league/rosters` with team cards showing 40-man, 26-man, and minors counts per league
- **FA Bid Calculator** — `/bid-calculator` interactive client-side calculator with multiplier reference table
- **Commissioner Waiver Audit** — `/admin/waiver-audit` shows all players on waivers across all leagues with claiming teams and time remaining
- **Waiver Wire Spotlight** — Home page sidebar widget showing top 5 expiring waivers with countdown timers and league name
- **Trade Proposal Contract Preview** — Trade proposal page shows inline contract data per player, live trade summary preview, and salary impact table (Salary OUT/IN/Net per year 2026-2030)
- **Waiver Wire League Filtering** — Waiver wire dropdown only shows leagues where the user has a team (not all leagues)
- **Roster Actions Column** — Moved to left of player name on roster page for better UX
- **Dollar Formatting** — `formatMoney` template function now parses string values and formats all amounts with commas ($760,000 not $760000)
- **IFA Signing Flow** — IFA players show orange "IFA" badge on free agents page; player profile shows dedicated ISBP signing form (single amount, no years/AAV/bid points) instead of standard bid form; bid worker deducts from team ISBP balance and clears IFA flag on finalization
- **Recent Activity League Filtering** — Home page recent activity feed only shows transactions from leagues where the logged-in user has a team; users with no teams see no activity
- **Roster Counts Summary Bar** — Roster page shows compact bar between team header and financials with 26-man (X/26), 40-man (X/40), SP on 26-man (X/6), minors count, Restructures (X/1), and Extensions (X/2); over-limit values colored red, under-limit green; roster limits pulled from `league_settings`; restructure/extension counts query `pending_actions` (PENDING + APPROVED) for current year (hardcoded limits: 1 restructure, 2 extensions per team per year); restructure/extension counts show ℹ icon with player name(s) tooltip on hover when usage > 0
- **Trade Center Role-Aware Buttons** — Proposer sees "Cancel Trade", receiver sees "Reject" + "Accept Trade"; `RejectTradeHandler` with ownership verification at `POST /trades/reject`
- **50% Salary Retention in Trades** — Per-player retention checkboxes on both sides of trade proposal form; `trade_items.retain_salary` column tracks per-item; `AcceptTrade` applies optional 50% on remainder after date-based retention; dead cap note reflects both layers; salary impact table includes Dead Cap column; pending trades show orange "50% retained" badge
- **ISBP Balance on Trade Form** — Both ISBP input fields show "Available: $X" for each team; proposer updates dynamically on team switch, target is server-rendered
- **Weekly Rotations Enhancements** — Full-week submission (all 7 days at once) replacing per-day saves; banked starters system (pitcher_2 can be "banked" and used on a later day, invalidated if pitcher has regular start in between); week navigation with prev/next arrows and date range display; `GetTeamWeekRotation` API endpoint to load existing rotation data; server-side validation (roster ownership, banked usage rules, duplicate pitcher checks); submission progress tracking (X/Y teams submitted per league); `rotations.day_of_week` stored as integer (0=Monday through 6=Sunday); `banked_starters` JSONB column on rotations table
- **Trade Counter Proposals** — Receivers can counter a trade instead of only accepting/rejecting; `GET /trades/counter?trade_id=X` loads `trade_counter.html` with pre-populated players and ISBP (roles flipped from original); `POST /trades/counter` creates new trade with `parent_trade_id` FK and marks parent as `COUNTERED`; chainable (either side can keep countering); trade center shows purple "Counter Proposal" badge and orange "Counter" button for receivers; `GetTradeByID` store function fetches single trade with items; `AcceptTrade` uses recursive CTE to clean up stale PROPOSED trades in the chain; email notification sent on counter; `migrations/016_counter_proposals.sql`
- **Dark Mode Toggle** — Per-user "Broadcast War Room" dark theme (deep navy `#0D1B2A`, electric cyan `#00E5FF`, warm orange `#FF8C42`); `theme_preference` column on users (default 'light'); server-rendered `<body class="dark-mode">` conditional (no FOUC); moon/sun toggle button in nav bar next to username; `POST /profile/update-theme` persists preference; profile page has explicit Display Preferences section; ~400 lines of dark mode CSS in `layout.html` using `body.dark-mode` selectors with `!important` to override inline styles across 30+ templates; dark mode `.section-title` uses `background: #0A1628` to avoid cyan-on-cyan unreadable text; `migrations/017_user_theme_preference.sql`
- **Dead Cap Detail Tables** — Roster page shows per-player dead cap breakdown (Player, Year, Amount, Note) below the Minors section, only when entries exist; `GetTeamDeadCap()` in `store/teams.go` queries `dead_cap_penalties` joined with `players`; player profile page also shows dead cap penalties table (Team, Year, Amount, Note) below Roster History; `GetPlayerDeadCap()` queries by `player_id`
- **Fantasy Points System (Pitching + Hitting)** — `internal/worker/stats.go` polls MLB Stats API box scores; runs daily 5-6 AM ET, Mar 25–Oct; 7-day catchup on each tick; fetches each box score once per game, processes both pitching and hitting; `ProcessDateStats()` exported for admin backfill; `POST /admin/stats/backfill` triggers background processing for a specific date
- **Pitching Scoring** — 19 categories: IP (+1), K (+1), GS (+1), SV (+2), HLD (+1), CG (+6), SHO (+6), QS (+7), NH (+8), PG (+10), IRS (+1), PKO (+1), ER (-1), HRA (-1), BB (-0.5), HBP (-0.5), WP (-0.5), BK (-1), BS (-2); derived stats: QS (6+ IP, ≤3 ER), NH (CG + 0 hits), PG (NH + 0 BB + 0 HBP)
- **Hitting Scoring** — 8 categories: H (+1), HR (+4), RBI (+1), R (+1), BB (+1), SB (+2), K (-0.5), CS (-1); player qualifies if `atBats > 0` or `plateAppearances > 0`
- **Pitching Leaderboard** — `GET /stats/pitching` with league/date filters; columns: GP, Total Pts, Avg Pts, IP, K, ER, QS, SV, HLD; template `stats_leaderboard.html`
- **Hitting Leaderboard** — `GET /stats/hitting` with league/date filters; columns: GP, Total Pts, Avg Pts, H, HR, RBI, R, BB, SB, K, CS; template `stats_hitting_leaderboard.html`; pitching/hitting toggle nav links on both leaderboard pages
- **Player Game Log** — `GET /api/player/:id/gamelog?type=pitching|hitting` returns JSON; player profile auto-detects position (SP/RP → pitching, all others → hitting) and renders appropriate columns
- **Roster FPTS Column** — Roster page shows season-total fantasy points per player via `GetPlayerPointsSummary()`; sums both pitching and hitting points
- **IL Summary Table** — Roster page shows compact table of all IL players (Player, Pos, IL Type, MLB Team) between the roster counts bar and financials; only renders when team has IL players
- **Transaction Log** — `/activity` page shows completed transactions with dual filters (league dropdown + transaction type dropdown); `GetTransactionLog()` queries with `WHERE status = 'COMPLETED'` and optional league/type filters; `GetDistinctTransactionTypes()` populates the type dropdown dynamically; 500 result limit; color-coded type badges (green=Added Player, red=Dropped Player, blue=Roster Move, orange=Trade); dark mode support
- **Transaction Type Reclassification** — All transaction types standardized to 4 clean categories: `Added Player` (FA signings, IFA, waiver claims), `Dropped Player` (DFA, releases), `Roster Move` (promotions, options, IL, arbitration, extensions, restructures, seasonal), `Trade` (trades, trade reversals); all `LogActivity` calls and direct `INSERT INTO transactions` statements updated to use new type names; `migrations/020_reclassify_transaction_types.sql` reclassified historical data using keyword matching on summaries
- **Player Add Request Form** — `/player/request` lets team owners request new players be added to the database; bulk submission supports up to 10 players at once with dynamic add/remove rows (JS reindexing), shared league dropdown and notes field; per-row fields: first name, last name, position (dropdown), MLB team, IFA checkbox; 2-column CSS grid layout per row with card-style containers; handler loops indexed fields (`first_name_0`, `position_1`, etc.) and creates individual `player_add_requests` rows; success message shows count ("3 player requests have been submitted..."); only users with teams can submit; commissioners approve/reject individually at `/admin/player-requests`; on approval, player auto-created as free agent (`fa_status='available'`, `is_international_free_agent` per request) and activity logged; commissioner scoping (global admins see all, league commissioners see their leagues); linked from nav bar and admin dashboard; `player_add_requests` table with PENDING/APPROVED/REJECTED status; `migrations/021_player_add_requests.sql`
- **My Open Bids Page** — `/bids/my-bids` shows only the logged-in user's active bids (where `pending_bid_team_id` matches any of their teams via `team_owners`); `GetUserOpenBids()` in `store/bids.go` reuses `PendingBidPlayer` struct; columns: Player, Position, League, Team, Bid Points, Years, AAV, Time Remaining, Action; live countdown timers with color coding; empty state when no bids; nav link before "Pending Bids"
- **Nested Nav Dropdowns** — Nav bar groups 19+ flat links into 4 CSS-only hover dropdowns for logged-in users: **League** (Rosters, Standings, Financials, Activity, Stats), **Roster** (Rotations, Team Options, Arbitration, Waiver Wire), **Free Agency** (Free Agents, My Bids, Pending Bids, Bid History, Bid Calc, Request Player), **Trades** (Trade Center, Trade Block); Home, Report Bug, and Commissioner remain as direct links; non-logged-in users see flat links; dark mode fully supported
- **Migrations:** `018_fantasy_points.sql` creates `scoring_categories`, `daily_player_stats`, `stats_processing_log` tables and seeds 19 pitching + 8 hitting categories; `019_activate_hitting.sql` sets `is_active = TRUE` for hitting categories; `020_reclassify_transaction_types.sql` standardizes transaction types to 4 categories

### Commissioner Tools Enhancements
- **Bid/FA Management in Player Editor** — Commissioners can manually set `fa_status`, pending bid fields, and `bid_type` on any player
- **IFA Toggle in Player Editor** — `is_international_free_agent` checkbox in Status section; IFA filter on free agents page uses this field
- **Bid History Tracking** — Every bid appends to `bid_history` JSONB; displayed on player profile as collapsible table
- **Slack Integration UI** — `/admin/settings` now has per-league Slack config: bot token, transactions channel, completed trades channel, stat alerts channel, trade block channel
- **Commissioner Role Management** — `/admin/roles` UI to add/remove league commissioners (`league_roles` table) and update global user roles (admin/user); admin-only, linked from dashboard
- **ISBP/MiLB Balance Editor** — `/admin/balance-editor` lets commissioners view and edit team ISBP and MiLB balances; league filter dropdown, modal edit form, linked from dashboard Financials card
- **Fantrax Processing Queue** — `/admin/fantrax-queue` shows roster-affecting transactions (Roster Move/Added Player/Trade) pending Fantrax sync; league filter dropdown, "Show Completed" toggle, "Mark Completed"/"Undo" buttons via existing `/admin/fantrax-toggle` endpoint; linked from dashboard with pink accent card
- **Player Editor Team Dropdowns** — Team assignment and bidding team fields use league-filtered dropdowns instead of raw UUID inputs; assignment dropdown filters by selected league via JS; bidding team dropdown grouped by league with `<optgroup>`
- **Player Editor Team Option Years** — Per-contract-year TO (Team Option) checkboxes (2026–2040) in contracts section; reads/writes `contract_option_years` JSONB column; pre-checked on edit
- **Player Editor DFA Only** — `dfa_only` checkbox in Status section; reads/writes `dfa_only` BOOLEAN column
- **Player Editor Save Confirmation** — Green success banner shown after saving via `?saved=1` query param
- **Team & User Management** — `/admin/team-owners` page for adding/removing team owners (`team_owners` table), creating new users (bcrypt, bypasses approval queue), and deleting users (cascading FK cleanup); global admins see all leagues, commissioners see only their leagues; `AddTeamOwner`/`RemoveTeamOwner` update `teams.owner_name` automatically; linked from dashboard

### Commissioner AI Agent
- **Chat UI** — `/admin/agent` chat-based interface for commissioners; extends `layout.html`, vanilla JS with conversation history
- **Backend** — `internal/handlers/agent.go` integrates Google Vertex AI Gemini 2.0 Flash with function calling (tool use loop)
- **GCP Auth** — Service account JSON key at `/root/app/service-account.json`, mounted into containers via docker-compose volume; env vars `GOOGLE_CLOUD_PROJECT=fantasy-435215` and `GOOGLE_APPLICATION_CREDENTIALS`
- **League Scoping** — All agent tools filtered by commissioner's `league_roles`; even global admins only see leagues they're commissioner of
- **Tools (28):** `search_players` (custom SQL with team JOIN), `get_player`, `list_teams`, `get_team_balance`, `assign_player_to_team`, `release_player`, `update_player_name`, `update_team_balance`, `run_query` (SELECT only, word-boundary keyword blocklist), `get_pending_approvals` (actions + registrations + trades with league_name), `process_pending_action`, `process_registration`, `get_team_roster`, `count_roster`, `get_team_payrolls` (sums contract columns per team with luxury tax + dead cap), `get_recent_activity` (transactions table with NULL league_id fallback to team's league), `check_roster_compliance` (26/40-man + SP limits + under-22 minimum per team), `get_waiver_status` (on-waivers players + time remaining + claiming teams), `get_league_deadlines` (all dates/windows with open/closed status), `find_expiring_contracts` (last dollar-value contract year), `update_player_contract` (set contract value for a year), `add_dead_cap` (add dead cap penalty via store function), `dfa_player` (DFA with 48h waiver period + roster move log + activity log), `set_league_date` (set opening day, trade deadline, etc. for one or all leagues via `UpsertLeagueDate`), `get_bug_reports` (structured bug list with exact IDs, filtered by status), `update_bug_status` (mark bug reports OPEN/CLOSED), `get_pending_arbitration` (ARB cases awaiting approval, optional league filter), `get_unsubmitted_arbitration` (ARB players whose teams haven't submitted a decision yet, optional league filter), `assign_team_owner` (assign user to team by email + team name, detects existing owners, requires `replace=true` to swap)
- **Contract parsing in payrolls** — Contract TEXT values can be `"1000000"`, `"$1,000,000"`, `"1000000(TO)"` — SQL strips `(TO)`, `$`, `,` then validates with regex before casting to NUMERIC; matches the Go-side pattern in `CalculateYearlySummary()`
- **System prompt enhancements** — Includes common SQL query patterns (players without team, most expensive contracts, dead cap totals, etc.) and multi-step reasoning instructions (chain search_players → get_player for trade analysis, use dedicated tools over run_query)
- **System Prompt** — Includes full DB schema reference, current year via `time.Now().Year()`, commissioner's league access, and behavioral instructions (use IDs from prior tool calls, filter by league when asked)
- **Protobuf workaround** — Tool results JSON-marshaled to string before passing to Gemini `FunctionResponse` (SDK can't serialize nested `[]map[string]interface{}`)
- **Markdown rendering** — `formatMarkdown()` JS function converts `**bold**`, `*italic*`, `` `code` `` to HTML; assistant messages use `innerHTML`, user messages use `textContent` for XSS safety
- **Deploy note** — `docker-compose.prod.yml` must be manually SCPed to server when changed (deploy scripts only copy binary + templates)

### Production Hardening (pre-cutover)
- **Caddyfile** — Updated for `frontofficedynastysports.com` (not yet deployed; sandbox still uses `app.` subdomain): security headers (HSTS, X-Frame-Options, X-Content-Type-Options, X-XSS-Protection, Referrer-Policy), gzip compression, www→root redirect
- **Security headers middleware** — `internal/middleware/security.go` sets defense-in-depth headers on every response (behind Caddy)
- **Trusted proxies** — `r.SetTrustedProxies([]string{"127.0.0.1"})` since Caddy is the only reverse proxy on localhost
- **Error sanitization** — All 500-level errors across 13 handler files now log real errors server-side (`fmt.Printf("ERROR [handler]: %v\n", err)`) and return generic "Internal server error" to clients; no DB errors leak to users
- **CSV upload hardening** — Admin role check on POST handler, 5 MB body size limit (`http.MaxBytesReader`), header read error handling, required column validation, `ReadAll()` error handling, row length bounds check
- **Graceful shutdown** — `signal.NotifyContext` for SIGINT/SIGTERM; cancels worker context, then gracefully shuts down HTTP server with 10-second timeout
- **Worker context** — All 5 workers (`bids`, `waivers`, `seasonal`, `hr_monitor`, `stats`) accept `context.Context` and use `select` on `ctx.Done()` to stop cleanly on shutdown

### Data Sync (PHP → Go)

Six `cmd/` tools sync data from the live WordPress/PHP site into the Go PostgreSQL database via the WP REST API. Run in this exact order (each depends on the previous):

| Order | Tool | What it does |
|-------|------|-------------|
| 1 | `cmd/import_teams` | Creates teams from WP users' ACF `managed_teams` with abbreviation + ISBP |
| 2 | `cmd/sync_users_bulk` | Creates Go users from WP users, links via `wp_id` |
| 3 | `cmd/sync_team_ownership` | Populates `team_owners` junction table (abbreviation → team UUID lookup) |
| 4 | `cmd/sync_players` | Syncs all 39K+ players with team_id resolution, contracts, dead cap |
| 5 | `cmd/sync_transactions` | Imports activity feed history (uses legacy types — run migration 020 after to reclassify) |
| 6 | `cmd/sync_bid_history` | Reconstructs `bid_history` JSONB on players from transaction text |
| 7 | `cmd/sync_waivers` | Syncs waiver status, end times, claims; clears stale waivers not on WP |
| 8 | `cmd/sync_site_settings` | Syncs ISBP/MILB balances, luxury tax thresholds from WP Site Settings |

**To run:** Build Linux binaries, SCP to server, run with production `DATABASE_URL`:
```bash
# Build (from PowerShell)
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o sync_players_linux ./cmd/sync_players
# SCP + run on server
scp sync_players_linux root@178.128.178.100:/root/app/
ssh root@178.128.178.100 "DATABASE_URL='postgres://admin:<prod-password>@localhost:5433/fantasy_db' /root/app/sync_players_linux"
```

**Last full sync (2026-02-19):** 123 teams, 88 users, 128 owner links, 39,753 players, 3,023 transactions, 148 dead cap entries, 60 ISBP balances, 30 MILB balances, 60 luxury tax entries, 6 active waivers with claims.

**WP API Bridge Plugin:** `tools/fod-api-bridge.php` — installed at `wp-content/mu-plugins/` on the PHP site. Exposes ACF Site Settings (ISBP, MILB, luxury tax, Slack config, key dates) via REST endpoint with key auth. Used by `sync_site_settings`. Remove after migration.

**Note:** These tools call the WordPress REST API and will stop working after the PHP site is retired.

### Migrations Required
- `migrations/013_feature_batch.sql` — Adds: `transactions.fantrax_processed`, `players.fod_id`, `registration_requests` table, `league_dates` table, `system_counters` table
- `migrations/014_business_rules.sql` — Adds: `roster_26_man_limit`, `roster_40_man_limit`, `sp_26_man_limit` columns to `league_settings`
- `migrations/016_counter_proposals.sql` — Adds: `parent_trade_id` UUID column (FK to trades) + index for counter proposal chains
- `migrations/017_user_theme_preference.sql` — Adds: `theme_preference` VARCHAR(10) column to users (default 'light')
- `migrations/018_fantasy_points.sql` — Adds: `scoring_categories`, `daily_player_stats`, `stats_processing_log` tables; seeds 19 pitching + 8 hitting scoring categories (hitting initially inactive)
- `migrations/019_activate_hitting.sql` — Activates 8 hitting scoring categories
- `migrations/020_reclassify_transaction_types.sql` — Standardizes `transaction_type` values from legacy types (ADD, DROP, ROSTER, TRADE, COMMISSIONER, WAIVER, SEASONAL) to 4 clean categories (Added Player, Dropped Player, Roster Move, Trade); uses keyword matching on summaries to reclassify COMMISSIONER entries
- `migrations/021_player_add_requests.sql` — Adds: `player_add_requests` table (id, first_name, last_name, position, mlb_team, league_id, is_ifa, notes, submitted_by, status, reviewed_by, reviewed_at, created_at)

### Not Implemented (deferred)
- Draft Room (Feature 2) — complex real-time feature, deferred
- Mobile App (Feature 18) — separate project
- NBA Support (Feature 20) — separate product

## Gotchas

- **CRITICAL: Deploy with `--build`** — `docker compose restart` does NOT rebuild the image; the deploy scripts handle this. If deploying manually, always use `docker compose up -d --build app` (or `app-staging`). Run via `nohup` because SSH may drop during builds.
- **Staging vs Production** — `app.frontofficedynastysports.com` is staging (`app-staging` container, `fantasy_db_staging`), `frontofficedynastysports.com` is production (`app` container, `fantasy_db`). Staging has its own isolated DB; SMTP/Slack are disabled. Use `deploy-staging.ps1` and `promote-production.ps1` to deploy.
- **Connection pool deadlock** — Never make nested `db.Query` calls while iterating outer `rows`; collect results first, close rows, then do inner queries (see `league_financials.go` for example)
- `go.mod` says Go 1.24 but Dockerfile uses `golang:1.23-alpine` — binary is built locally so this only matters for on-server builds
- CORS defaults to `https://frontofficedynastysports.com`; override with `CORS_ORIGIN` env var
- **Production DB password** differs from dev default — check `/root/app/.env` on server for actual credentials; DB exposed on port 5433 externally
- Many `cmd/` utilities (`sync_players`, `import_teams`, etc.) reference the old WordPress API and may not work after PHP site is retired
- 60+ one-off SQL scripts in project root are migration artifacts — not part of the app
- Workers run in-process with graceful shutdown — if the server restarts, bid/waiver timers reset (no persistent job queue)
- Seasonal worker uses `system_counters` to prevent duplicate runs across restarts
- HR monitor and stats worker both require `players.mlb_id` to be populated for cross-referencing with MLB Stats API; `mlb_id` is non-unique (same real player appears in multiple fantasy leagues); ~770 unique MLB IDs cover ~3,036 player records; stats worker uses `LIMIT 1` when looking up by `mlb_id`
- Email notifications require SMTP env vars; silently disabled if not configured
- Registration now goes through approval queue — existing users are unaffected
- **Team ownership is via `team_owners` junction table** — never query `teams.user_id` directly (column exists but is legacy); always JOIN `team_owners` to find a user's teams; roster page uses `IsOwner` from handler (via `store.IsTeamOwner()`) for action buttons and "Propose Trade" visibility
- **`GetManagedTeams` is lightweight** — does NOT populate `.Players`; use `GetTeamWithRoster` when player data is needed (e.g., trade proposal page)
- **PostgreSQL COALESCE type matching** — `COALESCE(uuid_col, 'text')` fails; must cast: `COALESCE(uuid_col::TEXT, 'text')`
- **ISBP data lives in WP options table** — not on WP users' ACF fields (those are always 0); use the `fod-api-bridge.php` plugin to access via REST API
