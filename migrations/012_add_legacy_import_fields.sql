-- Add legacy import fields to teams table
ALTER TABLE teams ADD COLUMN IF NOT EXISTS owner_name TEXT;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS wp_id INTEGER;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS abbreviation TEXT;
