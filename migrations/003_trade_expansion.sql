-- Add ISBP to trades table
ALTER TABLE trades 
ADD COLUMN IF NOT EXISTS isbp_offered INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS isbp_requested INTEGER DEFAULT 0;

-- Add retention flag to trade_items
ALTER TABLE trade_items
ADD COLUMN IF NOT EXISTS retain_salary BOOLEAN DEFAULT FALSE;
