-- ==========================================
-- 1. CLEANUP DUPLICATES
-- ==========================================
DELETE FROM teams a USING teams b
WHERE a.id < b.id
  AND a.name = b.name;

-- ==========================================
-- 2. ADD SAFETY RULE
-- ==========================================
DO $$
    BEGIN
        IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'teams_name_key') THEN
            ALTER TABLE teams ADD CONSTRAINT teams_name_key UNIQUE (name);
        END IF;
    END $$;

-- ==========================================
-- 3. CREATE MISSING TEAMS (With Owner Name!)
-- ==========================================
INSERT INTO teams (id, name, league_id, owner_name) VALUES
                                                        (gen_random_uuid(), 'Las Vegas Aviators (AAA)', '22222222-2222-2222-2222-222222222222', 'League Office'),
                                                        (gen_random_uuid(), 'Sacramento River Cats (AAA)', '22222222-2222-2222-2222-222222222222', 'League Office'),
                                                        (gen_random_uuid(), 'Omaha Storm Chasers (AAA)', '22222222-2222-2222-2222-222222222222', 'League Office'),
                                                        (gen_random_uuid(), 'St. Paul Saints (AAA)', '22222222-2222-2222-2222-222222222222', 'League Office'),
                                                        (gen_random_uuid(), 'Gwinnett Stripers (AAA)', '22222222-2222-2222-2222-222222222222', 'League Office'),
                                                        (gen_random_uuid(), 'Buffalo Bisons (AAA)', '22222222-2222-2222-2222-222222222222', 'League Office'),
                                                        (gen_random_uuid(), 'Norfolk Tides (AAA)', '22222222-2222-2222-2222-222222222222', 'League Office'),
                                                        (gen_random_uuid(), 'Iowa Cubs (AAA)', '22222222-2222-2222-2222-222222222222', 'League Office'),
                                                        (gen_random_uuid(), 'New Hampshire Fisher Cats (AA)', '33333333-3333-3333-3333-333333333333', 'League Office'),
                                                        (gen_random_uuid(), 'Bowie Baysox (AA)', '33333333-3333-3333-3333-333333333333', 'League Office'),
                                                        (gen_random_uuid(), 'Springfield Cardinals (AA)', '33333333-3333-3333-3333-333333333333', 'League Office'),
                                                        (gen_random_uuid(), 'Frisco RoughRiders (AA)', '33333333-3333-3333-3333-333333333333', 'League Office'),
                                                        (gen_random_uuid(), 'Midland RockHounds (AA)', '33333333-3333-3333-3333-333333333333', 'League Office'),
                                                        (gen_random_uuid(), 'Erie SeaWolves (AA)', '33333333-3333-3333-3333-333333333333', 'League Office'),
                                                        (gen_random_uuid(), 'Akron RubberDucks (AA)', '33333333-3333-3333-3333-333333333333', 'League Office'),
                                                        (gen_random_uuid(), 'Portland Sea Dogs (AA)', '33333333-3333-3333-3333-333333333333', 'League Office')
ON CONFLICT (name) DO NOTHING;

-- ==========================================
-- 4. FORCE THE TRADES
-- ==========================================
-- AAA Manual Fixes
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Las Vegas Aviators (AAA)') WHERE raw_fantasy_team_id = 'LV';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Sacramento River Cats (AAA)') WHERE raw_fantasy_team_id = 'SAC';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Omaha Storm Chasers (AAA)') WHERE raw_fantasy_team_id = 'OMA';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'St. Paul Saints (AAA)') WHERE raw_fantasy_team_id = 'STP';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Gwinnett Stripers (AAA)') WHERE raw_fantasy_team_id IN ('GWI', 'ATL');

-- AA Manual Fixes
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'New Hampshire Fisher Cats (AA)') WHERE raw_fantasy_team_id = 'New Hampshire Fisher Cats';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Bowie Baysox (AA)') WHERE raw_fantasy_team_id = 'Chesapeake Baysox';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Springfield Cardinals (AA)') WHERE raw_fantasy_team_id = 'Springfield Cardinals';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Frisco RoughRiders (AA)') WHERE raw_fantasy_team_id = 'Frisco RoughRiders';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Midland RockHounds (AA)') WHERE raw_fantasy_team_id = 'Midland Rockhounds';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Erie SeaWolves (AA)') WHERE raw_fantasy_team_id = 'Erie SeaWolves';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Akron RubberDucks (AA)') WHERE raw_fantasy_team_id = 'Akron RubberDucks';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Portland Sea Dogs (AA)') WHERE raw_fantasy_team_id = 'Portland Sea Dogs';

-- SMART LINKER (Fixes JAC -> JAC (AAA), BUF -> BUF (AAA), etc.)
UPDATE players p
SET team_id = t.id
FROM teams t
WHERE p.team_id IS NULL
  AND p.raw_fantasy_team_id IS NOT NULL
  AND t.name LIKE p.raw_fantasy_team_id || ' (%';

-- Special Logic for NYM -> SYR (AAA) and others if needed
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Syracuse Mets (AAA)') WHERE raw_fantasy_team_id = 'NYM';

-- Check results
SELECT raw_fantasy_team_id, COUNT(*) as still_missing FROM players WHERE team_id IS NULL AND raw_fantasy_team_id != '' GROUP BY raw_fantasy_team_id ORDER BY still_missing DESC;