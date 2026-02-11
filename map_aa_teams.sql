-- ========================================================
-- 1. CREATE AA TEAMS (Eastern, Southern, Texas Leagues)
-- League ID: 33333333-3333-3333-3333-333333333333
-- ========================================================
INSERT INTO teams (id, name, league_id, owner_name) VALUES
-- EASTERN LEAGUE
(gen_random_uuid(), 'Akron RubberDucks',       '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Altoona Curve',           '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Binghamton Rumble Ponies','33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Bowie Baysox',            '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Erie SeaWolves',          '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Harrisburg Senators',     '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Hartford Yard Goats',     '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'New Hampshire Fisher Cats','33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Portland Sea Dogs',       '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Reading Fightin Phils',   '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Richmond Flying Squirrels','33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Somerset Patriots',       '33333333-3333-3333-3333-333333333333', 'League Office'),

-- SOUTHERN LEAGUE
(gen_random_uuid(), 'Biloxi Shuckers',         '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Birmingham Barons',       '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Chattanooga Lookouts',    '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Columbus Clingstones',    '33333333-3333-3333-3333-333333333333', 'League Office'), -- Replaces Mississippi Braves
(gen_random_uuid(), 'Knoxville Smokies',       '33333333-3333-3333-3333-333333333333', 'League Office'), -- Formerly Tennessee Smokies
(gen_random_uuid(), 'Montgomery Biscuits',     '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Pensacola Blue Wahoos',   '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Rocket City Trash Pandas','33333333-3333-3333-3333-333333333333', 'League Office'),

-- TEXAS LEAGUE
(gen_random_uuid(), 'Amarillo Sod Poodles',    '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Arkansas Travelers',      '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Corpus Christi Hooks',    '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Frisco RoughRiders',      '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Midland RockHounds',      '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Northwest Arkansas Naturals','33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'San Antonio Missions',    '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Springfield Cardinals',   '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Tulsa Drillers',          '33333333-3333-3333-3333-333333333333', 'League Office'),
(gen_random_uuid(), 'Wichita Wind Surge',      '33333333-3333-3333-3333-333333333333', 'League Office')
ON CONFLICT (name) DO NOTHING;

-- ========================================================
-- 2. MAP CODES (RESTRICTED TO AA LEAGUE PLAYERS ONLY)
-- ========================================================

-- Eastern League
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Akron RubberDucks')       WHERE raw_fantasy_team_id = 'AKR' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Altoona Curve')           WHERE raw_fantasy_team_id = 'ALT' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Binghamton Rumble Ponies') WHERE raw_fantasy_team_id = 'BIN' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Bowie Baysox')            WHERE raw_fantasy_team_id = 'BOW' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Erie SeaWolves')          WHERE raw_fantasy_team_id = 'ERI' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Harrisburg Senators')     WHERE raw_fantasy_team_id = 'HBG' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Hartford Yard Goats')     WHERE raw_fantasy_team_id = 'HFD' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'New Hampshire Fisher Cats') WHERE raw_fantasy_team_id = 'NH'  AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Portland Sea Dogs')       WHERE raw_fantasy_team_id = 'POR' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Reading Fightin Phils')   WHERE raw_fantasy_team_id = 'REA' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Richmond Flying Squirrels') WHERE raw_fantasy_team_id = 'RIC' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Somerset Patriots')       WHERE raw_fantasy_team_id = 'SOM' AND league_id = '33333333-3333-3333-3333-333333333333';

-- Southern League
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Biloxi Shuckers')         WHERE raw_fantasy_team_id = 'BIL' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Birmingham Barons')       WHERE raw_fantasy_team_id = 'BIR' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Chattanooga Lookouts')    WHERE raw_fantasy_team_id = 'CHA' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Columbus Clingstones')    WHERE raw_fantasy_team_id = 'CCS' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Knoxville Smokies')       WHERE raw_fantasy_team_id IN ('KNO', 'TEN') AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Montgomery Biscuits')     WHERE raw_fantasy_team_id = 'MTG' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Pensacola Blue Wahoos')   WHERE raw_fantasy_team_id = 'PEN' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Rocket City Trash Pandas') WHERE raw_fantasy_team_id = 'RCT' AND league_id = '33333333-3333-3333-3333-333333333333';

-- Texas League
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Amarillo Sod Poodles')    WHERE raw_fantasy_team_id = 'AMA' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Arkansas Travelers')      WHERE raw_fantasy_team_id = 'ARK' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Corpus Christi Hooks')    WHERE raw_fantasy_team_id = 'CC'  AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Frisco RoughRiders')      WHERE raw_fantasy_team_id = 'FRI' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Midland RockHounds')      WHERE raw_fantasy_team_id = 'MID' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Northwest Arkansas Naturals') WHERE raw_fantasy_team_id = 'NWA' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'San Antonio Missions')    WHERE raw_fantasy_team_id = 'SA'  AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Springfield Cardinals')   WHERE raw_fantasy_team_id = 'SPR' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Tulsa Drillers')          WHERE raw_fantasy_team_id = 'TUL' AND league_id = '33333333-3333-3333-3333-333333333333';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Wichita Wind Surge')      WHERE raw_fantasy_team_id = 'WIC' AND league_id = '33333333-3333-3333-3333-333333333333';

-- ========================================================
-- 3. FINAL VERIFICATION (Check for unmapped AA players)
-- ========================================================
SELECT raw_fantasy_team_id, COUNT(*) as missing_count
FROM players
WHERE league_id = '33333333-3333-3333-3333-333333333333'
  AND team_id IS NULL
GROUP BY raw_fantasy_team_id;