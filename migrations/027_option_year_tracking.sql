-- Track options properly: 3 lifetime years, 5 sends per year
ALTER TABLE players ADD COLUMN IF NOT EXISTS options_this_season INT NOT NULL DEFAULT 0;
ALTER TABLE players ADD COLUMN IF NOT EXISTS option_year_logged INT NOT NULL DEFAULT 0;
