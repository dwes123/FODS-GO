-- Counter proposals: add parent_trade_id to link counter trades to their originals
ALTER TABLE trades ADD COLUMN IF NOT EXISTS parent_trade_id UUID REFERENCES trades(id);
CREATE INDEX IF NOT EXISTS idx_trades_parent_trade_id ON trades(parent_trade_id);
