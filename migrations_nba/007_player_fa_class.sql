-- fa_class is the canonical FA-state field used by free-agency workflows.
-- Values:
--   'Owned'      — player is on a fantasy roster with a guaranteed contract for the upcoming year
--   'Pending'    — player is on a fantasy roster but their contract for the upcoming year is a
--                  cap hold (UFA Year), Qualifying Offer (RFA), or otherwise transitional. The
--                  bird-rights team has matching/extension rights but the player is otherwise
--                  available to negotiate elsewhere through their Agent.
--   'Free Agent' — player has no fantasy team (team_id IS NULL).
--
-- Derived from contract_2026 + contract_annotations by a daily worker (see
-- internal/worker/nba/fa_class.go). NOT manually edited — every overnight run
-- re-syncs from the source-of-truth contract data.

ALTER TABLE players ADD COLUMN IF NOT EXISTS fa_class TEXT NOT NULL DEFAULT 'Owned'
    CHECK (fa_class IN ('Owned', 'Pending', 'Free Agent'));

CREATE INDEX IF NOT EXISTS idx_players_fa_class ON players(fa_class);
