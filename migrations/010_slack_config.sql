-- Table for Slack configurations per league
CREATE TABLE IF NOT EXISTS league_integrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    league_id UUID REFERENCES leagues(id) ON DELETE CASCADE,
    slack_bot_token TEXT,
    slack_channel_trade_block TEXT,
    slack_channel_transactions TEXT,
    UNIQUE(league_id)
);

-- Seed with current MLB/AAA IDs
INSERT INTO league_integrations (league_id)
SELECT id FROM leagues
ON CONFLICT DO NOTHING;
