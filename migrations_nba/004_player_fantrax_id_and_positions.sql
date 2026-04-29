-- Adds fantrax_id to players (the canonical ID format from the Dynasty Association
-- spreadsheet, e.g., '*03e75*') and a partial unique index for upsert-by-fantrax-id.
-- Also documents that players.position uses single-letter G/F/C codes (or comma lists like 'G,F'),
-- matching the Fantrax position vocabulary the league uses for G/F/C/FLEX lineup slots.

ALTER TABLE players ADD COLUMN IF NOT EXISTS fantrax_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_players_fantrax_id
    ON players(fantrax_id)
    WHERE fantrax_id IS NOT NULL;
