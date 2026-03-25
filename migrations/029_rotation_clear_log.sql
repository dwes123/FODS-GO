-- Audit log for when users clear their pitching rotations
CREATE TABLE IF NOT EXISTS rotation_clear_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id),
    league_id UUID NOT NULL REFERENCES leagues(id),
    user_id UUID NOT NULL REFERENCES users(id),
    week_identifier TEXT NOT NULL,
    cleared_rotations JSONB DEFAULT '[]'::jsonb,   -- snapshot of deleted rotation entries
    cleared_banked_starts JSONB DEFAULT '[]'::jsonb, -- snapshot of deleted/cleared banked starts
    created_at TIMESTAMP DEFAULT NOW()
);
