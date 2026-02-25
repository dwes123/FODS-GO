-- 018_fantasy_points.sql
-- Fantasy points system: scoring categories, daily player stats, processing log

-- Configurable scoring weights per stat
CREATE TABLE IF NOT EXISTS scoring_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stat_type TEXT NOT NULL,          -- 'pitching' or 'hitting'
    stat_key TEXT NOT NULL,           -- e.g. 'ip', 'k', 'er', 'qs'
    display_name TEXT NOT NULL,       -- e.g. 'Innings Pitched', 'Strikeouts'
    points NUMERIC(6,2) NOT NULL,     -- point value (can be negative)
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    UNIQUE(stat_type, stat_key)
);

-- One row per player per game per stat type
CREATE TABLE IF NOT EXISTS daily_player_stats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id UUID NOT NULL REFERENCES players(id),
    mlb_id TEXT,
    game_pk INT NOT NULL,
    game_date DATE NOT NULL,
    stat_type TEXT NOT NULL DEFAULT 'pitching',
    raw_stats JSONB NOT NULL DEFAULT '{}'::jsonb,
    fantasy_points NUMERIC(8,2) NOT NULL DEFAULT 0,
    team_id UUID REFERENCES teams(id),
    league_id UUID REFERENCES leagues(id),
    opponent TEXT,                     -- MLB team abbreviation
    UNIQUE(player_id, game_pk, stat_type)
);

CREATE INDEX IF NOT EXISTS idx_daily_player_stats_game_date ON daily_player_stats(game_date);
CREATE INDEX IF NOT EXISTS idx_daily_player_stats_player_id ON daily_player_stats(player_id);
CREATE INDEX IF NOT EXISTS idx_daily_player_stats_team_id ON daily_player_stats(team_id);
CREATE INDEX IF NOT EXISTS idx_daily_player_stats_league_id ON daily_player_stats(league_id);
CREATE INDEX IF NOT EXISTS idx_daily_player_stats_mlb_id ON daily_player_stats(mlb_id);

-- Tracks which dates have been processed
CREATE TABLE IF NOT EXISTS stats_processing_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_date DATE NOT NULL,
    stat_type TEXT NOT NULL DEFAULT 'pitching',
    games_processed INT NOT NULL DEFAULT 0,
    players_processed INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'completed',
    error_message TEXT,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(game_date, stat_type)
);

-- Seed pitching scoring categories
INSERT INTO scoring_categories (stat_type, stat_key, display_name, points, is_active) VALUES
    ('pitching', 'ip',  'Innings Pitched',           1.0,  TRUE),
    ('pitching', 'k',   'Strikeouts',                1.0,  TRUE),
    ('pitching', 'gs',  'Games Started',             1.0,  TRUE),
    ('pitching', 'sv',  'Saves',                     2.0,  TRUE),
    ('pitching', 'hld', 'Holds',                     1.0,  TRUE),
    ('pitching', 'cg',  'Complete Games',            6.0,  TRUE),
    ('pitching', 'sho', 'Shutouts',                  6.0,  TRUE),
    ('pitching', 'qs',  'Quality Starts',            7.0,  TRUE),
    ('pitching', 'nh',  'No-Hitters',                8.0,  TRUE),
    ('pitching', 'pg',  'Perfect Games',            10.0,  TRUE),
    ('pitching', 'irs', 'Inherited Runners Stranded', 1.0, TRUE),
    ('pitching', 'pko', 'Pickoffs',                  1.0,  TRUE),
    ('pitching', 'er',  'Earned Runs',              -1.0,  TRUE),
    ('pitching', 'hra', 'Home Runs Allowed',        -1.0,  TRUE),
    ('pitching', 'bb',  'Walks',                    -0.5,  TRUE),
    ('pitching', 'hb',  'Hit By Pitch',             -0.5,  TRUE),
    ('pitching', 'wp',  'Wild Pitches',             -0.5,  TRUE),
    ('pitching', 'bk',  'Balks',                    -1.0,  TRUE),
    ('pitching', 'bs',  'Blown Saves',              -2.0,  TRUE)
ON CONFLICT (stat_type, stat_key) DO NOTHING;

-- Seed hitting scoring categories (inactive, for future use)
INSERT INTO scoring_categories (stat_type, stat_key, display_name, points, is_active) VALUES
    ('hitting', 'h',   'Hits',               1.0,  FALSE),
    ('hitting', 'hr',  'Home Runs',          4.0,  FALSE),
    ('hitting', 'rbi', 'RBI',                1.0,  FALSE),
    ('hitting', 'r',   'Runs',               1.0,  FALSE),
    ('hitting', 'bb',  'Walks',              1.0,  FALSE),
    ('hitting', 'sb',  'Stolen Bases',       2.0,  FALSE),
    ('hitting', 'k',   'Strikeouts',        -0.5,  FALSE),
    ('hitting', 'cs',  'Caught Stealing',   -1.0,  FALSE)
ON CONFLICT (stat_type, stat_key) DO NOTHING;
