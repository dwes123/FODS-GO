-- 1. Nuke everything (start fresh)
DROP TABLE IF EXISTS roster_entries;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS leagues;

-- 2. Create Leagues
CREATE TABLE leagues (
                         id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                         name TEXT NOT NULL,
                         league_type TEXT NOT NULL,
                         created_at TIMESTAMP DEFAULT NOW()
);

-- 3. Create Teams (With the correct "owner_name" column)
CREATE TABLE teams (
                       id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       league_id UUID REFERENCES leagues(id) ON DELETE CASCADE,
                       name TEXT NOT NULL,
                       owner_name TEXT NOT NULL, -- This is the fix!
                       email TEXT,
                       created_at TIMESTAMP DEFAULT NOW()
);

-- 4. Create Players
CREATE TABLE players (
                         id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                         mlb_id TEXT UNIQUE,
                         first_name TEXT NOT NULL,
                         last_name TEXT NOT NULL,
                         position TEXT NOT NULL,
                         mlb_team TEXT,
                         stats JSONB DEFAULT '{}',
                         created_at TIMESTAMP DEFAULT NOW()
);

-- 5. Create Roster Entries
CREATE TABLE roster_entries (
                                id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                team_id UUID REFERENCES teams(id) ON DELETE CASCADE,
                                player_id UUID REFERENCES players(id) ON DELETE CASCADE,
                                status TEXT DEFAULT 'ACTIVE',
                                acquired_date TIMESTAMP DEFAULT NOW(),
                                UNIQUE(team_id, player_id)
);