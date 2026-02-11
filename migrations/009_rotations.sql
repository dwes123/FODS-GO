-- Table for weekly pitching rotations
CREATE TABLE IF NOT EXISTS rotations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID REFERENCES leagues(id),
    team_id UUID REFERENCES teams(id),
    week_identifier TEXT NOT NULL, -- e.g., '2026-06'
    day_of_week TEXT NOT NULL, -- 'monday', 'tuesday', etc.
    
    pitcher_1_id UUID REFERENCES players(id),
    pitcher_1_date DATE,
    
    pitcher_2_id UUID REFERENCES players(id),
    pitcher_2_date DATE,
    
    banked_starters JSONB DEFAULT '[]'::jsonb, -- [{ "id": "uuid", "date": "2026-02-10" }]
    
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(team_id, week_identifier, day_of_week)
);
