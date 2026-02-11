-- ========================================================
-- CLEANUP SCRIPT: DELETE TEAMS WITH 0 PLAYERS
-- ========================================================

-- 1. Optional: See what we are about to delete (Safety Check)
-- This will list the names of the empty teams so you can verify they are the "Old" ones.
SELECT name as team_to_be_deleted, '0 Players' as status
FROM teams t
         LEFT JOIN players p ON t.id = p.team_id
GROUP BY t.id, t.name
HAVING COUNT(p.id) = 0
ORDER BY t.name;

-- 2. The Delete Command
-- This removes any team that is not referenced by a single player.
DELETE FROM teams
WHERE id NOT IN (
    SELECT DISTINCT team_id
    FROM players
    WHERE team_id IS NOT NULL
);

-- 3. Verify Final List
-- Show the remaining teams (The "New" ones) and their player counts
SELECT t.name, COUNT(p.id) as player_count
FROM teams t
         JOIN players p ON t.id = p.team_id
GROUP BY t.id, t.name
ORDER BY t.name;