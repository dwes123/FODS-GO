-- Player Add Requests: user-submitted requests for new players
CREATE TABLE IF NOT EXISTS player_add_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    position TEXT NOT NULL,
    mlb_team TEXT NOT NULL,
    league_id UUID NOT NULL REFERENCES leagues(id),
    is_ifa BOOLEAN DEFAULT FALSE,
    notes TEXT,
    submitted_by UUID NOT NULL REFERENCES users(id),
    status TEXT DEFAULT 'PENDING',
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
