-- Migration 013: Schema changes for feature batch (18 features)
-- Run this before deploying any Phase 1+ code changes.

-- ============================================================
-- 1. Transactions table additions
-- ============================================================

-- Fantrax sync tracking (Feature #6)
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS fantrax_processed BOOLEAN DEFAULT FALSE;

-- Summary column (used by existing Go code but never formally migrated)
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS summary TEXT;

-- ============================================================
-- 2. Players table additions
-- ============================================================

-- FOD unique identifier (Feature #10)
ALTER TABLE players ADD COLUMN IF NOT EXISTS fod_id TEXT UNIQUE;

-- ============================================================
-- 3. Registration requests table (Feature #9)
-- ============================================================

CREATE TABLE IF NOT EXISTS registration_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    status TEXT DEFAULT 'pending',   -- 'pending', 'approved', 'denied'
    reviewed_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ
);

-- ============================================================
-- 4. League dates table (Features #7, #16)
--    Stores trade deadlines, opening days, etc. per league/year
-- ============================================================

CREATE TABLE IF NOT EXISTS league_dates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID REFERENCES leagues(id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    date_type TEXT NOT NULL,          -- 'trade_deadline', 'opening_day'
    event_date DATE NOT NULL,
    UNIQUE(league_id, year, date_type)
);

-- ============================================================
-- 5. System counters table (Feature #10)
--    Atomic counters for FOD ID generation, seasonal task tracking
-- ============================================================

CREATE TABLE IF NOT EXISTS system_counters (
    key TEXT PRIMARY KEY,
    value INTEGER DEFAULT 0
);

INSERT INTO system_counters (key, value) VALUES ('fod_id_counter', 10000) ON CONFLICT DO NOTHING;
