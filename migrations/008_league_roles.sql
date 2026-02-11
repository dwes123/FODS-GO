-- Table for league-specific roles
CREATE TABLE IF NOT EXISTS league_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    league_id UUID REFERENCES leagues(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'commissioner', -- 'commissioner'
    UNIQUE(user_id, league_id)
);

-- Example: Make Dan a commissioner for MLB and AAA
INSERT INTO league_roles (user_id, league_id, role)
SELECT u.id, l.id, 'commissioner'
FROM users u, leagues l
WHERE u.username = 'Dan' AND l.name IN ('MLB', 'AAA')
ON CONFLICT DO NOTHING;
