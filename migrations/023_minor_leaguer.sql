-- Migration 023: Add minor leaguer badge column
ALTER TABLE players ADD COLUMN IF NOT EXISTS is_minor_leaguer BOOLEAN NOT NULL DEFAULT FALSE;
