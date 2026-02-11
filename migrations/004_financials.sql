-- Add balances to teams
ALTER TABLE teams 
ADD COLUMN IF NOT EXISTS isbp_balance NUMERIC DEFAULT 5000000,
ADD COLUMN IF NOT EXISTS milb_balance NUMERIC DEFAULT 25000000;

-- Create Dead Cap Penalties table
CREATE TABLE IF NOT EXISTS dead_cap_penalties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
    player_id UUID REFERENCES players(id) ON DELETE SET NULL, -- Can be null if player is deleted from DB entirely
    amount NUMERIC NOT NULL,
    year INTEGER NOT NULL,
    note TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
