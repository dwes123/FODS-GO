-- Users table to replace WordPress users
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username TEXT UNIQUE NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL, -- Will use bcrypt
    role TEXT DEFAULT 'user', -- 'admin', 'commish', 'user'
    created_at TIMESTAMP DEFAULT NOW()
);

-- Sessions table for generic cookie-based auth
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    token TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Link Teams to Users (One user can own multiple teams)
ALTER TABLE teams ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users(id) ON DELETE SET NULL;

-- Transaction Types Enum
CREATE TYPE transaction_type AS ENUM ('ADD', 'DROP', 'TRADE', 'COMMISSIONER');

-- Transactions Log
CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID REFERENCES leagues(id),
    team_id UUID REFERENCES teams(id),
    player_id UUID REFERENCES players(id),
    transaction_type transaction_type NOT NULL,
    related_transaction_id UUID, -- For linking an Add to a Drop
    status TEXT DEFAULT 'PENDING', -- 'PENDING', 'APPROVED', 'REJECTED', 'COMPLETED'
    created_at TIMESTAMP DEFAULT NOW(),
    processed_at TIMESTAMP
);

-- Trade Offers
CREATE TABLE IF NOT EXISTS trades (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    proposing_team_id UUID REFERENCES teams(id),
    receiving_team_id UUID REFERENCES teams(id),
    status TEXT DEFAULT 'PROPOSED', -- 'PROPOSED', 'ACCEPTED', 'REJECTED', 'CANCELLED', 'VETOED', 'PROCESSED'
    created_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP
);

-- Trade Assets (Players/Picks moving in a trade)
CREATE TABLE IF NOT EXISTS trade_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trade_id UUID REFERENCES trades(id) ON DELETE CASCADE,
    sender_team_id UUID REFERENCES teams(id),
    player_id UUID REFERENCES players(id), -- Null if it's a draft pick
    draft_pick_year INTEGER,
    draft_pick_round INTEGER,
    draft_pick_original_owner_id UUID REFERENCES teams(id) -- To track whose pick it was
);
