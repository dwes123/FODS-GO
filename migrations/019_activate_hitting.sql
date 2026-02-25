-- 019_activate_hitting.sql
-- Activate the 8 hitting scoring categories seeded in migration 018.
UPDATE scoring_categories SET is_active = TRUE WHERE stat_type = 'hitting';
