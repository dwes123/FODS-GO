-- Add a column to store the old WordPress User ID
ALTER TABLE teams ADD COLUMN IF NOT EXISTS wp_id INT;

-- Ensure the "AAA" League exists
INSERT INTO leagues (id, name, league_type)
VALUES ('22222222-2222-2222-2222-222222222222', 'AAA', 'Dynasty')
ON CONFLICT (id) DO NOTHING;