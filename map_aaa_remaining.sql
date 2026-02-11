-- ========================================================
-- FIX REMAINING AAA MAPPINGS
-- LEAGUE ID: 22222222-2222-2222-2222-222222222222
-- ========================================================

-- ALB -> Albuquerque Isotopes
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Albuquerque Isotopes')
WHERE raw_fantasy_team_id = 'ALB' AND league_id = '22222222-2222-2222-2222-222222222222';

-- CHA -> Charlotte Knights (Was CLT)
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Charlotte Knights')
WHERE raw_fantasy_team_id = 'CHA' AND league_id = '22222222-2222-2222-2222-222222222222';

-- EP -> El Paso Chihuahuas (Was ELP)
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'El Paso Chihuahuas')
WHERE raw_fantasy_team_id = 'EP' AND league_id = '22222222-2222-2222-2222-222222222222';

-- Iow -> Iowa Cubs (Was IOW/IWA)
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Iowa Cubs')
WHERE raw_fantasy_team_id = 'Iow' AND league_id = '22222222-2222-2222-2222-222222222222';

-- JAC -> Jacksonville Jumbo Shrimp (Was JAX)
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Jacksonville Jumbo Shrimp')
WHERE raw_fantasy_team_id = 'JAC' AND league_id = '22222222-2222-2222-2222-222222222222';

-- NSH -> Nashville Sounds (Was NAS)
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Nashville Sounds')
WHERE raw_fantasy_team_id = 'NSH' AND league_id = '22222222-2222-2222-2222-222222222222';

-- RENO -> Reno Aces (Was RNO)
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Reno Aces')
WHERE raw_fantasy_team_id = 'RENO' AND league_id = '22222222-2222-2222-2222-222222222222';

-- SLC -> Salt Lake Bees (Was SL)
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Salt Lake Bees')
WHERE raw_fantasy_team_id = 'SLC' AND league_id = '22222222-2222-2222-2222-222222222222';

-- ========================================================
-- FINAL CHECK
-- ========================================================
-- The only rows left should be the blank ones (Free Agents)
SELECT raw_fantasy_team_id, COUNT(*) as missing_count
FROM players
WHERE league_id = '22222222-2222-2222-2222-222222222222'
  AND team_id IS NULL
GROUP BY raw_fantasy_team_id;