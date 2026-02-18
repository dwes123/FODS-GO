-- 014_business_rules.sql
-- Add configurable roster limits & settings to league_settings

ALTER TABLE league_settings
    ADD COLUMN IF NOT EXISTS roster_26_man_limit INTEGER DEFAULT 26,
    ADD COLUMN IF NOT EXISTS roster_40_man_limit INTEGER DEFAULT 40,
    ADD COLUMN IF NOT EXISTS sp_26_man_limit INTEGER DEFAULT 6;

-- league_dates already supports arbitrary date_type values via UpsertLeagueDate.
-- New date types we'll use: 'extension_deadline', 'ifa_window_open', 'ifa_window_close',
-- 'milb_fa_window_open', 'milb_fa_window_close', 'option_deadline', 'roster_expansion_start',
-- 'roster_expansion_end'
-- No DDL needed — these are just string values in the existing league_dates table.
