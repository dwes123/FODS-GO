-- Initial schema for fantasy_basketball_db.
-- Note: users, sessions, and league_roles live in fantasy_db (the baseball DB).
-- Tables here reference user_id as a soft FK (UUID without REFERENCES) — app-layer maintains integrity.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ============================================================
-- Leagues (single NBA league row will be seeded)
-- ============================================================
CREATE TABLE IF NOT EXISTS leagues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    abbreviation TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Seed the single NBA league with a fixed UUID for app-side referencing
INSERT INTO leagues (id, name, abbreviation)
VALUES ('55555555-5555-5555-5555-555555555555', 'NBA', 'NBA')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- Teams
-- ============================================================
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    abbreviation TEXT,
    owner_name TEXT,
    cap_space NUMERIC(14,2) DEFAULT 0,
    luxury_tax_balance NUMERIC(14,2) DEFAULT 0,
    trade_exception_balance NUMERIC(14,2) DEFAULT 0,
    fantrax_url TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_teams_league ON teams(league_id);

-- ============================================================
-- Team owners (junction; soft FK to users in baseball DB)
-- ============================================================
CREATE TABLE IF NOT EXISTS team_owners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,           -- soft FK to fantasy_db.users.id
    is_primary BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(team_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_team_owners_user ON team_owners(user_id);

-- ============================================================
-- Players
-- ============================================================
CREATE TABLE IF NOT EXISTS players (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nba_id INTEGER,                   -- stats.nba.com player ID (analog of mlb_id, non-unique across leagues if we ever add more)
    league_id UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id UUID REFERENCES teams(id) ON DELETE SET NULL,

    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    position TEXT,                    -- 'PG', 'SG', 'SF', 'PF', 'C', or comma list 'PG,SG'
    jersey_number TEXT,
    height_inches INTEGER,
    weight_lbs INTEGER,
    age INTEGER,
    college TEXT,
    draft_year INTEGER,
    draft_pick INTEGER,
    real_life_team TEXT,              -- NBA real-life team abbreviation, e.g. 'LAL'

    -- Contract columns (TEXT — values can be dollar strings or named tags)
    contract_2026 TEXT,
    contract_2027 TEXT,
    contract_2028 TEXT,
    contract_2029 TEXT,
    contract_2030 TEXT,
    contract_2031 TEXT,
    contract_2032 TEXT,
    contract_2033 TEXT,
    contract_2034 TEXT,
    contract_2035 TEXT,
    contract_2036 TEXT,
    contract_2037 TEXT,
    contract_2038 TEXT,
    contract_2039 TEXT,
    contract_2040 TEXT,

    -- Per-year contract annotations (e.g., '12/1 Trade Restriction', 'DPE Designation')
    -- JSONB shape: { "2026": ["12/1 Trade Restriction"], "2027": ["DPE Designation"] }
    contract_annotations JSONB DEFAULT '{}'::jsonb,

    -- Roster status
    on_two_way BOOLEAN DEFAULT FALSE,
    on_active_roster BOOLEAN DEFAULT TRUE,
    injury_status TEXT,               -- 'OUT', 'GTD', 'DAY-TO-DAY', 'OUT-FOR-SEASON', NULL
    fa_status TEXT,                   -- 'FA', 'pending_bid', 'on_waivers', NULL
    on_trade_block BOOLEAN DEFAULT FALSE,

    -- Bidding
    bid_end_time TIMESTAMP,
    waiver_end_time TIMESTAMP,
    current_bid_user_id UUID,         -- soft FK to fantasy_db.users.id
    current_bid_team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    bid_history JSONB DEFAULT '[]'::jsonb,

    -- Misc
    roster_moves_log JSONB DEFAULT '[]'::jsonb,
    contract_option_years JSONB DEFAULT '{}'::jsonb,

    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_players_team ON players(team_id);
CREATE INDEX IF NOT EXISTS idx_players_league ON players(league_id);
CREATE INDEX IF NOT EXISTS idx_players_nba_id ON players(nba_id);
CREATE INDEX IF NOT EXISTS idx_players_fa_status ON players(fa_status);
CREATE INDEX IF NOT EXISTS idx_players_name ON players(last_name, first_name);

-- ============================================================
-- League settings (per league per year)
-- ============================================================
CREATE TABLE IF NOT EXISTS league_settings (
    league_id UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    salary_cap NUMERIC(14,2),
    luxury_tax_limit NUMERIC(14,2),
    apron_first NUMERIC(14,2),
    apron_second NUMERIC(14,2),
    roster_standard_limit INTEGER NOT NULL DEFAULT 15,
    roster_two_way_limit INTEGER NOT NULL DEFAULT 3,
    PRIMARY KEY (league_id, year)
);

-- ============================================================
-- Key dates (deadlines, windows)
-- ============================================================
CREATE TABLE IF NOT EXISTS key_dates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    date_type TEXT NOT NULL,          -- 'opening_day', 'trade_deadline', 'option_deadline', etc.
    event_date DATE NOT NULL,
    label TEXT,
    UNIQUE(league_id, date_type, event_date)
);

CREATE INDEX IF NOT EXISTS idx_key_dates_league ON key_dates(league_id);

-- ============================================================
-- Transactions / activity feed
-- ============================================================
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    player_id UUID REFERENCES players(id) ON DELETE SET NULL,
    user_id UUID,                     -- soft FK to fantasy_db.users.id
    transaction_type TEXT NOT NULL,   -- 'Added Player', 'Dropped Player', 'Roster Move', 'Trade'
    description TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_transactions_league ON transactions(league_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created ON transactions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transactions_team ON transactions(team_id);

-- ============================================================
-- Pending actions (admin approval queue)
-- ============================================================
CREATE TABLE IF NOT EXISTS pending_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID REFERENCES leagues(id) ON DELETE CASCADE,
    action_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'approved', 'rejected'
    requested_by UUID,                -- soft FK to users
    processed_by UUID,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_pending_actions_status ON pending_actions(status);

-- ============================================================
-- Bug reports
-- ============================================================
CREATE TABLE IF NOT EXISTS bug_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID,                     -- soft FK to users
    title TEXT,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Agent audit log (for the NBA AI commissioner agent — Slice 5)
-- ============================================================
CREATE TABLE IF NOT EXISTS agent_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID,                     -- soft FK to users
    tool_name TEXT NOT NULL,
    tool_args JSONB,
    tool_result JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_audit_user ON agent_audit_log(user_id, created_at DESC);

-- ============================================================
-- Default league_settings row for the current year
-- ============================================================
INSERT INTO league_settings (league_id, year, salary_cap, luxury_tax_limit, apron_first, apron_second, roster_standard_limit, roster_two_way_limit)
VALUES ('55555555-5555-5555-5555-555555555555', 2026, 154647000, 187895000, 195945000, 207824000, 15, 3)
ON CONFLICT (league_id, year) DO NOTHING;
