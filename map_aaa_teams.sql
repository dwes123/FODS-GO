-- ========================================================
-- 1. ENSURE AAA TEAMS EXIST
-- League ID: 22222222-2222-2222-2222-222222222222
-- ========================================================
INSERT INTO teams (id, name, league_id, owner_name) VALUES
-- PACIFIC COAST LEAGUE
(gen_random_uuid(), 'Las Vegas Aviators',      '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Reno Aces',               '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Sacramento River Cats',   '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Salt Lake Bees',          '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Tacoma Rainiers',         '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Albuquerque Isotopes',    '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'El Paso Chihuahuas',      '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Oklahoma City Comets',    '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Round Rock Express',      '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Sugar Land Space Cowboys','22222222-2222-2222-2222-222222222222', 'League Office'),

-- INTERNATIONAL LEAGUE
(gen_random_uuid(), 'Buffalo Bisons',          '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Charlotte Knights',       '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Columbus Clippers',       '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Durham Bulls',            '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Gwinnett Stripers',       '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Indianapolis Indians',    '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Iowa Cubs',               '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Jacksonville Jumbo Shrimp','22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Lehigh Valley IronPigs',  '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Louisville Bats',         '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Memphis Redbirds',        '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Nashville Sounds',        '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Norfolk Tides',           '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Omaha Storm Chasers',     '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Rochester Red Wings',     '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Scranton/Wilkes-Barre RailRiders', '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'St. Paul Saints',         '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Syracuse Mets',           '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Toledo Mud Hens',         '22222222-2222-2222-2222-222222222222', 'League Office'),
(gen_random_uuid(), 'Worcester Red Sox',       '22222222-2222-2222-2222-222222222222', 'League Office')
ON CONFLICT (name) DO NOTHING;

-- ========================================================
-- 2. MAP CODES (RESTRICTED TO AAA LEAGUE PLAYERS ONLY)
-- ========================================================

-- The Big Fix: We add "AND league_id = '2222...'" to everything.
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Columbus Clippers')
WHERE raw_fantasy_team_id = 'COL' AND league_id = '22222222-2222-2222-2222-222222222222';

UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Buffalo Bisons')
WHERE raw_fantasy_team_id = 'BUF' AND league_id = '22222222-2222-2222-2222-222222222222';

-- Pacific Coast League Mappings
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Las Vegas Aviators')      WHERE raw_fantasy_team_id = 'LV'  AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Sacramento River Cats')   WHERE raw_fantasy_team_id = 'SAC' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Salt Lake Bees')          WHERE raw_fantasy_team_id = 'SL'  AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Tacoma Rainiers')         WHERE raw_fantasy_team_id = 'TAC' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Reno Aces')              WHERE raw_fantasy_team_id = 'RNO' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Albuquerque Isotopes')    WHERE raw_fantasy_team_id = 'ABQ' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'El Paso Chihuahuas')      WHERE raw_fantasy_team_id = 'ELP' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Oklahoma City Comets')    WHERE raw_fantasy_team_id IN ('OKC', 'OKL') AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Round Rock Express')      WHERE raw_fantasy_team_id IN ('RR', 'RRE') AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Sugar Land Space Cowboys') WHERE raw_fantasy_team_id IN ('SUG', 'SLD') AND league_id = '22222222-2222-2222-2222-222222222222';

-- International League Mappings
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Charlotte Knights')       WHERE raw_fantasy_team_id = 'CLT' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Durham Bulls')            WHERE raw_fantasy_team_id = 'DUR' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Gwinnett Stripers')       WHERE raw_fantasy_team_id IN ('GWN', 'GWI') AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Indianapolis Indians')    WHERE raw_fantasy_team_id = 'IND' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Iowa Cubs')               WHERE raw_fantasy_team_id IN ('IOW', 'IWA') AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Jacksonville Jumbo Shrimp') WHERE raw_fantasy_team_id = 'JAX' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Lehigh Valley IronPigs')  WHERE raw_fantasy_team_id IN ('LHV', 'LEH') AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Louisville Bats')         WHERE raw_fantasy_team_id = 'LOU' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Memphis Redbirds')        WHERE raw_fantasy_team_id = 'MEM' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Nashville Sounds')        WHERE raw_fantasy_team_id = 'NAS' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Norfolk Tides')           WHERE raw_fantasy_team_id = 'NOR' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Omaha Storm Chasers')     WHERE raw_fantasy_team_id = 'OMA' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Rochester Red Wings')     WHERE raw_fantasy_team_id = 'ROC' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Scranton/Wilkes-Barre RailRiders') WHERE raw_fantasy_team_id IN ('SWB', 'SW') AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'St. Paul Saints')         WHERE raw_fantasy_team_id = 'STP' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Syracuse Mets')           WHERE raw_fantasy_team_id = 'SYR' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Toledo Mud Hens')         WHERE raw_fantasy_team_id = 'TOL' AND league_id = '22222222-2222-2222-2222-222222222222';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Worcester Red Sox')       WHERE raw_fantasy_team_id IN ('WOR', 'WOO') AND league_id = '22222222-2222-2222-2222-222222222222';

-- ========================================================
-- 3. FINAL VERIFICATION (Check for unmapped AAA players)
-- ========================================================
SELECT raw_fantasy_team_id, COUNT(*) as missing_count
FROM players
WHERE league_id = '22222222-2222-2222-2222-222222222222'
  AND team_id IS NULL
GROUP BY raw_fantasy_team_id;