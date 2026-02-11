-- Store league settings like Luxury Tax limits
CREATE TABLE IF NOT EXISTS league_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID REFERENCES leagues(id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    luxury_tax_limit NUMERIC DEFAULT 0,
    UNIQUE(league_id, year)
);

-- Seed with some example limits for 2026
INSERT INTO league_settings (league_id, year, luxury_tax_limit)
SELECT id, 2026, 250000000 FROM leagues
ON CONFLICT DO NOTHING;
