-- Add unique constraint on players.nba_id so the sync_nba_players importer can use ON CONFLICT.
-- Partial index: NULL nba_id values (e.g., placeholder rows) are allowed to coexist.
-- Once the importer runs, every row will have a populated nba_id.

CREATE UNIQUE INDEX IF NOT EXISTS uniq_players_nba_id
    ON players(nba_id)
    WHERE nba_id IS NOT NULL;
