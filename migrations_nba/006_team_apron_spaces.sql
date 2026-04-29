-- Adds proper apron #1 / apron #2 space columns. Earlier migrations conflated
-- "Apron #1 Space" (room below the apron threshold) with the existing
-- trade_exception_balance column (which represents an actual Traded Player
-- Exception balance — a different concept entirely). Splitting them apart.

ALTER TABLE teams ADD COLUMN IF NOT EXISTS apron1_space NUMERIC(14,2);
ALTER TABLE teams ADD COLUMN IF NOT EXISTS apron2_space NUMERIC(14,2);

-- Migrate the data that was incorrectly stored in trade_exception_balance
-- (which was actually apron #1 space all along). Leave trade_exception_balance
-- intact for now — it'll be re-populated by the importer to NULL on next run,
-- and re-purposed for actual TPE tracking later.
UPDATE teams
   SET apron1_space = trade_exception_balance
 WHERE apron1_space IS NULL
   AND trade_exception_balance IS NOT NULL
   AND league_id = '55555555-5555-5555-5555-555555555555';
