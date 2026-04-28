-- Add sport column to league_roles so commissioners can be scoped per sport.
-- Existing baseball league roles default to 'mlb'. NBA roles will be inserted with sport='nba'.

ALTER TABLE league_roles
    ADD COLUMN IF NOT EXISTS sport TEXT NOT NULL DEFAULT 'mlb';

-- Backfill (no-op if defaults already applied, but explicit for clarity)
UPDATE league_roles SET sport = 'mlb' WHERE sport IS NULL OR sport = '';

-- Helpful index for sport-scoped lookups
CREATE INDEX IF NOT EXISTS idx_league_roles_user_sport ON league_roles(user_id, sport);
