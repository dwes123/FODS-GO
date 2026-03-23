-- Dedicated table for tracking banked pitching starts and their usage
CREATE TABLE IF NOT EXISTS banked_starts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id),
    league_id UUID NOT NULL REFERENCES leagues(id),
    pitcher_id UUID NOT NULL REFERENCES players(id),
    banked_week TEXT NOT NULL,        -- "YYYY-WW" when the start was banked
    banked_day INT NOT NULL,          -- 0=Mon, 6=Sun
    banked_date DATE NOT NULL,        -- actual calendar date
    fantasy_points NUMERIC(8,2),      -- populated lazily from daily_player_stats
    used_week TEXT,                   -- NULL = available; "YYYY-WW" when used
    used_day INT,                     -- day of week it was used on
    used_date DATE,                   -- actual date it was used on
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(team_id, pitcher_id, banked_week, banked_day)
);

CREATE INDEX IF NOT EXISTS idx_banked_starts_team_week ON banked_starts(team_id, banked_week);
CREATE INDEX IF NOT EXISTS idx_banked_starts_available ON banked_starts(team_id) WHERE used_week IS NULL;
