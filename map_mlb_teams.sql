-- 1. ENSURE ALL 30 MLB TEAMS EXIST (Official Full Names)
-- League ID: 11111111-1111-1111-1111-111111111111
INSERT INTO teams (id, name, league_id, owner_name) VALUES
                                                        (gen_random_uuid(), 'Arizona Diamondbacks', '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Atlanta Braves',       '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Baltimore Orioles',    '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Boston Red Sox',       '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Chicago Cubs',         '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Chicago White Sox',    '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Cincinnati Reds',      '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Cleveland Guardians',  '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Colorado Rockies',     '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Detroit Tigers',       '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Houston Astros',       '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Kansas City Royals',   '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Los Angeles Angels',   '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Los Angeles Dodgers',  '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Miami Marlins',        '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Milwaukee Brewers',    '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Minnesota Twins',      '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'New York Mets',        '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'New York Yankees',     '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Oakland Athletics',    '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Philadelphia Phillies','11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Pittsburgh Pirates',   '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'San Diego Padres',     '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'San Francisco Giants', '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Seattle Mariners',     '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'St. Louis Cardinals',  '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Tampa Bay Rays',       '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Texas Rangers',        '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Toronto Blue Jays',    '11111111-1111-1111-1111-111111111111', 'League Office'),
                                                        (gen_random_uuid(), 'Washington Nationals', '11111111-1111-1111-1111-111111111111', 'League Office')
ON CONFLICT (name) DO NOTHING;

-- 2. MAP RAW ABBREVIATIONS TO TEAM IDs
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Arizona Diamondbacks') WHERE raw_fantasy_team_id IN ('ARI', 'ARZ');
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Atlanta Braves')       WHERE raw_fantasy_team_id = 'ATL';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Baltimore Orioles')    WHERE raw_fantasy_team_id = 'BAL';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Boston Red Sox')       WHERE raw_fantasy_team_id = 'BOS';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Chicago Cubs')         WHERE raw_fantasy_team_id IN ('CHC', 'CHI');
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Chicago White Sox')    WHERE raw_fantasy_team_id IN ('CWS', 'CHW');
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Cincinnati Reds')      WHERE raw_fantasy_team_id = 'CIN';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Cleveland Guardians')  WHERE raw_fantasy_team_id IN ('CLE', 'CLE');
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Colorado Rockies')     WHERE raw_fantasy_team_id IN ('COL');
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Detroit Tigers')       WHERE raw_fantasy_team_id = 'DET';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Houston Astros')       WHERE raw_fantasy_team_id = 'HOU';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Kansas City Royals')   WHERE raw_fantasy_team_id = 'KC';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Los Angeles Angels')   WHERE raw_fantasy_team_id IN ('LAA', 'ANA');
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Los Angeles Dodgers')  WHERE raw_fantasy_team_id = 'LAD';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Miami Marlins')        WHERE raw_fantasy_team_id IN ('MIA', 'FLO');
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Milwaukee Brewers')    WHERE raw_fantasy_team_id = 'MIL';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Minnesota Twins')      WHERE raw_fantasy_team_id = 'MIN';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'New York Mets')        WHERE raw_fantasy_team_id = 'NYM';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'New York Yankees')     WHERE raw_fantasy_team_id = 'NYY';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Oakland Athletics')    WHERE raw_fantasy_team_id = 'OAK';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Philadelphia Phillies') WHERE raw_fantasy_team_id = 'PHI';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Pittsburgh Pirates')   WHERE raw_fantasy_team_id = 'PIT';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'San Diego Padres')     WHERE raw_fantasy_team_id = 'SD';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'San Francisco Giants') WHERE raw_fantasy_team_id = 'SF';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Seattle Mariners')     WHERE raw_fantasy_team_id = 'SEA';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'St. Louis Cardinals')  WHERE raw_fantasy_team_id = 'STL';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Tampa Bay Rays')       WHERE raw_fantasy_team_id = 'TB';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Texas Rangers')        WHERE raw_fantasy_team_id = 'TEX';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Toronto Blue Jays')    WHERE raw_fantasy_team_id = 'TOR';
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Washington Nationals') WHERE raw_fantasy_team_id IN ('WSH', 'WAS');

-- 3. FINAL VERIFICATION (Should be Empty)
SELECT raw_fantasy_team_id, COUNT(*) as still_missing FROM players WHERE team_id IS NULL AND raw_fantasy_team_id != '' GROUP BY raw_fantasy_team_id;