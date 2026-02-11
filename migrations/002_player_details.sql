-- Update players table with detailed roster, contract, and transaction fields
ALTER TABLE players 
-- Roster Status Flags
ADD COLUMN IF NOT EXISTS status_40_man BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS status_26_man BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS status_il TEXT, -- e.g., '60-Day IL'
ADD COLUMN IF NOT EXISTS il_start_date TIMESTAMP,
ADD COLUMN IF NOT EXISTS dfa_only BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS rule_5_eligibility_year INTEGER,
ADD COLUMN IF NOT EXISTS option_years_used INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS depth_rank INTEGER DEFAULT 999,

-- Trade Block
ADD COLUMN IF NOT EXISTS on_trade_block BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS trade_block_notes TEXT,

-- Free Agency & Waivers Status
ADD COLUMN IF NOT EXISTS fa_status TEXT DEFAULT 'rostered', -- 'available', 'pending_bid', 'rostered', 'on waivers'
ADD COLUMN IF NOT EXISTS waiver_end_time TIMESTAMP,
ADD COLUMN IF NOT EXISTS waiving_team_id UUID, -- Links to teams(id)
ADD COLUMN IF NOT EXISTS dfa_clear_action TEXT, -- 'outright', 'release'

-- Active Bidding State
ADD COLUMN IF NOT EXISTS bid_type TEXT, -- 'standard', 'milb', 'isbp'
ADD COLUMN IF NOT EXISTS pending_bid_amount NUMERIC,
ADD COLUMN IF NOT EXISTS pending_bid_years INTEGER,
ADD COLUMN IF NOT EXISTS pending_bid_aav NUMERIC,
ADD COLUMN IF NOT EXISTS pending_bid_team_id UUID,
ADD COLUMN IF NOT EXISTS pending_bid_manager_id UUID,
ADD COLUMN IF NOT EXISTS bid_start_time TIMESTAMP,
ADD COLUMN IF NOT EXISTS bid_end_time TIMESTAMP,
ADD COLUMN IF NOT EXISTS milb_qualifying_stat TEXT, -- For MiLB bids

-- Player Attributes
ADD COLUMN IF NOT EXISTS is_international_free_agent BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS has_been_restructured BOOLEAN DEFAULT FALSE,

-- JSONB Storage for complex ACF repeaters
ADD COLUMN IF NOT EXISTS roster_moves_log JSONB DEFAULT '[]'::jsonb,
ADD COLUMN IF NOT EXISTS bid_history JSONB DEFAULT '[]'::jsonb,
ADD COLUMN IF NOT EXISTS contract_option_years JSONB DEFAULT '[]'::jsonb, -- Array of years [2027, 2028]

-- Contract Salaries (TEXT to support 'TC', 'ARB', etc.)
ADD COLUMN IF NOT EXISTS contract_2026 TEXT,
ADD COLUMN IF NOT EXISTS contract_2027 TEXT,
ADD COLUMN IF NOT EXISTS contract_2028 TEXT,
ADD COLUMN IF NOT EXISTS contract_2029 TEXT,
ADD COLUMN IF NOT EXISTS contract_2030 TEXT,
ADD COLUMN IF NOT EXISTS contract_2031 TEXT,
ADD COLUMN IF NOT EXISTS contract_2032 TEXT,
ADD COLUMN IF NOT EXISTS contract_2033 TEXT,
ADD COLUMN IF NOT EXISTS contract_2034 TEXT,
ADD COLUMN IF NOT EXISTS contract_2035 TEXT,
ADD COLUMN IF NOT EXISTS contract_2036 TEXT,
ADD COLUMN IF NOT EXISTS contract_2037 TEXT,
ADD COLUMN IF NOT EXISTS contract_2038 TEXT,
ADD COLUMN IF NOT EXISTS contract_2039 TEXT,
ADD COLUMN IF NOT EXISTS contract_2040 TEXT;
