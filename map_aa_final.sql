-- ========================================================
-- FINAL AA MAPPING (SMART MATCH + TYPO FIXES)
-- League ID: 33333333-3333-3333-3333-333333333333
-- ========================================================

-- 1. SMART MATCH: Link players where the Name matches exactly
-- This fixes almost everyone (Altoona, Tulsa, etc.) instantly.
UPDATE players p
SET team_id = t.id
FROM teams t
WHERE p.raw_fantasy_team_id = t.name
  AND p.league_id = '33333333-3333-3333-3333-333333333333'
  AND p.team_id IS NULL;

-- 2. TYPO FIXES (Manual Mapping for the weird ones)

-- Fix "Chesapeake Baysox" -> Bowie Baysox
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Bowie Baysox')
WHERE raw_fantasy_team_id = 'Chesapeake Baysox'
  AND league_id = '33333333-3333-3333-3333-333333333333';

-- Fix "Columbus Clingstone" -> Columbus Clingstones (Missing 's')
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Columbus Clingstones')
WHERE raw_fantasy_team_id = 'Columbus Clingstone'
  AND league_id = '33333333-3333-3333-3333-333333333333';

-- Fix "Corpus Christi" -> Corpus Christi Hooks (Missing 'Hooks')
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Corpus Christi Hooks')
WHERE raw_fantasy_team_id = 'Corpus Christi'
  AND league_id = '33333333-3333-3333-3333-333333333333';

-- Fix "Midland Rockhounds" -> Midland RockHounds (Capital H case sensitivity)
UPDATE players SET team_id = (SELECT id FROM teams WHERE name = 'Midland RockHounds')
WHERE raw_fantasy_team_id = 'Midland Rockhounds'
  AND league_id = '33333333-3333-3333-3333-333333333333';

-- ========================================================
-- 3. FINAL VERIFICATION
-- ========================================================
-- The only remaining rows should be the blank ones (Free Agents)
SELECT raw_fantasy_team_id, COUNT(*) as missing_count
FROM players
WHERE league_id = '33333333-3333-3333-3333-333333333333'
  AND team_id IS NULL
GROUP BY raw_fantasy_team_id;