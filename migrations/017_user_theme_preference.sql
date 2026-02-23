-- 017: Add theme_preference to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS theme_preference VARCHAR(10) NOT NULL DEFAULT 'light';
