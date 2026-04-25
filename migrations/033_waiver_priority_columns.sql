-- Sturdy Fantrax mapping for teams (replaces fragile name-string match)
ALTER TABLE teams ADD COLUMN IF NOT EXISTS fantrax_team_id TEXT;

-- Daily-recomputed waiver priority. 1 = worst team in standings (picks first),
-- N = best team (picks last). Recomputed at midnight PT by worker/waiver_priority.go.
ALTER TABLE teams ADD COLUMN IF NOT EXISTS current_waiver_priority INTEGER;

CREATE INDEX IF NOT EXISTS teams_fantrax_team_id_idx ON teams (fantrax_team_id);
