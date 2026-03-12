-- Track where a player was before being placed on IL so we can restore them
ALTER TABLE players ADD COLUMN IF NOT EXISTS pre_il_status TEXT;

-- Backfill: players currently on IL who were on 10/15-day IL were on 40-man,
-- players on 60-day IL were off 40-man but we assume they came from 26-man
UPDATE players SET pre_il_status = '26' WHERE status_il IS NOT NULL AND pre_il_status IS NULL;
