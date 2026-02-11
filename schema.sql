-- Leagues table
CREATE TABLE IF NOT EXISTS leagues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Global Players table (shared across leagues)
CREATE TABLE IF NOT EXISTS players (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mlb_id INTEGER UNIQUE, -- ID from MLB API
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    position TEXT,
    team_abbr TEXT, -- MLB Team (e.g., NYY, LAD)
    status TEXT, -- Active, IL, etc.
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Teams table (league-specific)
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    owner_id UUID, -- Placeholder for future auth integration
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    budget NUMERIC(12, 2) DEFAULT 0.00,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(league_id, slug)
);

-- Roster Entries (Linking Players to Teams within a League)
CREATE TABLE IF NOT EXISTS roster_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    contract_salary NUMERIC(12, 2) DEFAULT 0.00,
    contract_years INTEGER DEFAULT 1,
    acquisition_date DATE DEFAULT CURRENT_DATE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    -- A player can only be on one team per league
    UNIQUE(league_id, player_id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_teams_league ON teams(league_id);
CREATE INDEX IF NOT EXISTS idx_roster_entries_team ON roster_entries(team_id);
CREATE INDEX IF NOT EXISTS idx_roster_entries_league_player ON roster_entries(league_id, player_id);
