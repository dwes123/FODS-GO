-- Table for league-specific key dates
CREATE TABLE IF NOT EXISTS key_dates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID REFERENCES leagues(id) ON DELETE CASCADE,
    event_date TEXT NOT NULL,
    event_name TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);
