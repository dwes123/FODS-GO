-- Table to store pending arbitration and extension requests
CREATE TABLE IF NOT EXISTS pending_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id UUID REFERENCES players(id),
    team_id UUID REFERENCES teams(id),
    league_id UUID REFERENCES leagues(id),
    action_type TEXT NOT NULL, -- 'ARBITRATION', 'EXTENSION'
    target_year INTEGER,
    salary_amount NUMERIC,
    multi_year_contract JSONB, -- For extensions: { "2027": 5000000, "2028": 6000000 }
    status TEXT DEFAULT 'PENDING', -- 'PENDING', 'APPROVED', 'REJECTED'
    created_at TIMESTAMP DEFAULT NOW()
);
