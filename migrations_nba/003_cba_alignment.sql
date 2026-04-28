-- Schema alignment with The Dynasty Association CBA.
-- Adds: real NBA team seeding (30 teams w/ conferences and divisions), agencies + member junction,
-- player agency_id / career games / G-League / IR flags, league_settings exception amounts,
-- configurable FA trade-restriction date, per-team G-League budget.

-- ============================================================
-- 1. Teams: conference + division + G-League budget
-- ============================================================
ALTER TABLE teams ADD COLUMN IF NOT EXISTS conference TEXT;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS division TEXT;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS g_league_budget NUMERIC(12,2) NOT NULL DEFAULT 100000;

-- ============================================================
-- 2. Seed all 30 real NBA franchises with conference/division.
--    Owners (team_owners.user_id) intentionally NOT seeded — admin assigns later.
-- ============================================================
INSERT INTO teams (id, league_id, name, abbreviation, conference, division)
VALUES
  -- East / Atlantic
  ('11111111-1111-1111-1111-aaaaaaaa0001', '55555555-5555-5555-5555-555555555555', 'Boston Celtics',         'BOS', 'East', 'Atlantic'),
  ('11111111-1111-1111-1111-aaaaaaaa0002', '55555555-5555-5555-5555-555555555555', 'Brooklyn Nets',          'BKN', 'East', 'Atlantic'),
  ('11111111-1111-1111-1111-aaaaaaaa0003', '55555555-5555-5555-5555-555555555555', 'New York Knicks',        'NYK', 'East', 'Atlantic'),
  ('11111111-1111-1111-1111-aaaaaaaa0004', '55555555-5555-5555-5555-555555555555', 'Philadelphia 76ers',     'PHI', 'East', 'Atlantic'),
  ('11111111-1111-1111-1111-aaaaaaaa0005', '55555555-5555-5555-5555-555555555555', 'Toronto Raptors',        'TOR', 'East', 'Atlantic'),
  -- East / Central
  ('11111111-1111-1111-1111-aaaaaaaa0006', '55555555-5555-5555-5555-555555555555', 'Chicago Bulls',          'CHI', 'East', 'Central'),
  ('11111111-1111-1111-1111-aaaaaaaa0007', '55555555-5555-5555-5555-555555555555', 'Cleveland Cavaliers',    'CLE', 'East', 'Central'),
  ('11111111-1111-1111-1111-aaaaaaaa0008', '55555555-5555-5555-5555-555555555555', 'Detroit Pistons',        'DET', 'East', 'Central'),
  ('11111111-1111-1111-1111-aaaaaaaa0009', '55555555-5555-5555-5555-555555555555', 'Indiana Pacers',         'IND', 'East', 'Central'),
  ('11111111-1111-1111-1111-aaaaaaaa0010', '55555555-5555-5555-5555-555555555555', 'Milwaukee Bucks',        'MIL', 'East', 'Central'),
  -- East / Southeast
  ('11111111-1111-1111-1111-aaaaaaaa0011', '55555555-5555-5555-5555-555555555555', 'Atlanta Hawks',          'ATL', 'East', 'Southeast'),
  ('11111111-1111-1111-1111-aaaaaaaa0012', '55555555-5555-5555-5555-555555555555', 'Charlotte Hornets',      'CHA', 'East', 'Southeast'),
  ('11111111-1111-1111-1111-aaaaaaaa0013', '55555555-5555-5555-5555-555555555555', 'Miami Heat',             'MIA', 'East', 'Southeast'),
  ('11111111-1111-1111-1111-aaaaaaaa0014', '55555555-5555-5555-5555-555555555555', 'Orlando Magic',          'ORL', 'East', 'Southeast'),
  ('11111111-1111-1111-1111-aaaaaaaa0015', '55555555-5555-5555-5555-555555555555', 'Washington Wizards',     'WAS', 'East', 'Southeast'),
  -- West / Northwest
  ('11111111-1111-1111-1111-aaaaaaaa0016', '55555555-5555-5555-5555-555555555555', 'Denver Nuggets',         'DEN', 'West', 'Northwest'),
  ('11111111-1111-1111-1111-aaaaaaaa0017', '55555555-5555-5555-5555-555555555555', 'Minnesota Timberwolves', 'MIN', 'West', 'Northwest'),
  ('11111111-1111-1111-1111-aaaaaaaa0018', '55555555-5555-5555-5555-555555555555', 'Oklahoma City Thunder',  'OKC', 'West', 'Northwest'),
  ('11111111-1111-1111-1111-aaaaaaaa0019', '55555555-5555-5555-5555-555555555555', 'Portland Trail Blazers', 'POR', 'West', 'Northwest'),
  ('11111111-1111-1111-1111-aaaaaaaa0020', '55555555-5555-5555-5555-555555555555', 'Utah Jazz',              'UTA', 'West', 'Northwest'),
  -- West / Pacific
  ('11111111-1111-1111-1111-aaaaaaaa0021', '55555555-5555-5555-5555-555555555555', 'Golden State Warriors',  'GSW', 'West', 'Pacific'),
  ('11111111-1111-1111-1111-aaaaaaaa0022', '55555555-5555-5555-5555-555555555555', 'LA Clippers',            'LAC', 'West', 'Pacific'),
  ('11111111-1111-1111-1111-aaaaaaaa0023', '55555555-5555-5555-5555-555555555555', 'Los Angeles Lakers',     'LAL', 'West', 'Pacific'),
  ('11111111-1111-1111-1111-aaaaaaaa0024', '55555555-5555-5555-5555-555555555555', 'Phoenix Suns',           'PHX', 'West', 'Pacific'),
  ('11111111-1111-1111-1111-aaaaaaaa0025', '55555555-5555-5555-5555-555555555555', 'Sacramento Kings',       'SAC', 'West', 'Pacific'),
  -- West / Southwest
  ('11111111-1111-1111-1111-aaaaaaaa0026', '55555555-5555-5555-5555-555555555555', 'Dallas Mavericks',       'DAL', 'West', 'Southwest'),
  ('11111111-1111-1111-1111-aaaaaaaa0027', '55555555-5555-5555-5555-555555555555', 'Houston Rockets',        'HOU', 'West', 'Southwest'),
  ('11111111-1111-1111-1111-aaaaaaaa0028', '55555555-5555-5555-5555-555555555555', 'Memphis Grizzlies',      'MEM', 'West', 'Southwest'),
  ('11111111-1111-1111-1111-aaaaaaaa0029', '55555555-5555-5555-5555-555555555555', 'New Orleans Pelicans',   'NOP', 'West', 'Southwest'),
  ('11111111-1111-1111-1111-aaaaaaaa0030', '55555555-5555-5555-5555-555555555555', 'San Antonio Spurs',      'SAS', 'West', 'Southwest')
ON CONFLICT (id) DO NOTHING;

-- Backfill conference/division on any pre-existing rows that were created before this migration
UPDATE teams t
   SET conference = src.conference,
       division   = src.division
  FROM (VALUES
    ('Boston Celtics','East','Atlantic'),('Brooklyn Nets','East','Atlantic'),('New York Knicks','East','Atlantic'),
    ('Philadelphia 76ers','East','Atlantic'),('Toronto Raptors','East','Atlantic'),
    ('Chicago Bulls','East','Central'),('Cleveland Cavaliers','East','Central'),('Detroit Pistons','East','Central'),
    ('Indiana Pacers','East','Central'),('Milwaukee Bucks','East','Central'),
    ('Atlanta Hawks','East','Southeast'),('Charlotte Hornets','East','Southeast'),('Miami Heat','East','Southeast'),
    ('Orlando Magic','East','Southeast'),('Washington Wizards','East','Southeast'),
    ('Denver Nuggets','West','Northwest'),('Minnesota Timberwolves','West','Northwest'),
    ('Oklahoma City Thunder','West','Northwest'),('Portland Trail Blazers','West','Northwest'),('Utah Jazz','West','Northwest'),
    ('Golden State Warriors','West','Pacific'),('LA Clippers','West','Pacific'),('Los Angeles Lakers','West','Pacific'),
    ('Phoenix Suns','West','Pacific'),('Sacramento Kings','West','Pacific'),
    ('Dallas Mavericks','West','Southwest'),('Houston Rockets','West','Southwest'),('Memphis Grizzlies','West','Southwest'),
    ('New Orleans Pelicans','West','Southwest'),('San Antonio Spurs','West','Southwest')
  ) AS src(name, conference, division)
 WHERE t.name = src.name
   AND t.league_id = '55555555-5555-5555-5555-555555555555'
   AND (t.conference IS NULL OR t.division IS NULL);

-- ============================================================
-- 3. Agencies — two NBA Sports Agencies represent all players.
-- ============================================================
CREATE TABLE IF NOT EXISTS agencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    contracts_signed INTEGER NOT NULL DEFAULT 0,
    total_value_signed NUMERIC(14,2) NOT NULL DEFAULT 0,
    championships_won INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Seed two placeholder agencies (rename via admin tools).
INSERT INTO agencies (id, name) VALUES
    ('a9e0c111-0000-0000-0000-000000000001', 'Agency One'),
    ('a9e0c222-0000-0000-0000-000000000002', 'Agency Two')
ON CONFLICT (id) DO NOTHING;

-- Agency membership = which users act as agents for which agency.
-- user_id is a soft FK to fantasy_db.users.id (cross-DB) — app maintains integrity.
CREATE TABLE IF NOT EXISTS agency_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agency_id UUID NOT NULL REFERENCES agencies(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(agency_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_agency_members_user ON agency_members(user_id);

-- ============================================================
-- 4. Players — agency assignment, career games tracking, G-League / IR flags
-- ============================================================
ALTER TABLE players ADD COLUMN IF NOT EXISTS agency_id UUID REFERENCES agencies(id) ON DELETE SET NULL;
ALTER TABLE players ADD COLUMN IF NOT EXISTS career_games_played INTEGER;
ALTER TABLE players ADD COLUMN IF NOT EXISTS on_g_league BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE players ADD COLUMN IF NOT EXISTS on_ir BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_players_agency ON players(agency_id);
CREATE INDEX IF NOT EXISTS idx_players_g_league ON players(on_g_league) WHERE on_g_league;
CREATE INDEX IF NOT EXISTS idx_players_ir ON players(on_ir) WHERE on_ir;

-- ============================================================
-- 5. League settings — exception amounts + configurable FA trade restriction date
-- ============================================================
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS mle_amount NUMERIC(12,2);
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS tpmle_amount NUMERIC(12,2);
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS bae_amount NUMERIC(12,2);
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS min_salary_amount NUMERIC(12,2);

-- Configurable FA trade-restriction date (month-day). Default "12-01" per user's preference.
-- CBA PDF says November 15; user explicitly chose 12/1 on 2026-04-27 with the option to change later.
ALTER TABLE league_settings ADD COLUMN IF NOT EXISTS fa_trade_restriction_month_day TEXT NOT NULL DEFAULT '12-01';

-- Backfill 2026 with placeholder amounts (real NBA 2025-26 figures, user can update via admin).
UPDATE league_settings SET
    mle_amount        = COALESCE(mle_amount, 14100000),
    tpmle_amount      = COALESCE(tpmle_amount, 5500000),
    bae_amount        = COALESCE(bae_amount, 4900000),
    min_salary_amount = COALESCE(min_salary_amount, 1300000)
WHERE league_id = '55555555-5555-5555-5555-555555555555' AND year = 2026;
