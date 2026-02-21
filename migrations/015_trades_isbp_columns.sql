-- Add ISBP transfer columns and league_id to trades table
-- Already applied to production and staging on 2026-02-21

ALTER TABLE trades ADD COLUMN IF NOT EXISTS isbp_offered NUMERIC(12,2) DEFAULT 0;
ALTER TABLE trades ADD COLUMN IF NOT EXISTS isbp_requested NUMERIC(12,2) DEFAULT 0;
ALTER TABLE trades ADD COLUMN IF NOT EXISTS league_id UUID REFERENCES leagues(id);
