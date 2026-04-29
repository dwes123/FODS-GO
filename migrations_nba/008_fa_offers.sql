-- Free-agent offer flow: owner sends an offer to a player's Agency, Agent
-- accepts / rejects / counters, RFAs trigger a 48hr match window for the
-- bird-rights team. Counter chain is supported via parent_offer_id.

CREATE TABLE IF NOT EXISTS fa_offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Offer subject
    player_id UUID NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    agency_id UUID REFERENCES agencies(id) ON DELETE SET NULL,

    -- Parties
    offering_team_id     UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    bird_rights_team_id  UUID REFERENCES teams(id) ON DELETE SET NULL,

    -- Terms
    years INTEGER NOT NULL CHECK (years BETWEEN 1 AND 5),
    starting_salary NUMERIC(14,2) NOT NULL CHECK (starting_salary > 0),
    raise_pct NUMERIC(5,2) NOT NULL DEFAULT 0 CHECK (raise_pct BETWEEN 0 AND 8),
    exception_used TEXT NOT NULL CHECK (exception_used IN
        ('Bird Rights','Cap Space','MLE','TPMLE','BAE','Min','Sign-and-Trade','Other')),
    notes TEXT,

    -- Counter chain
    parent_offer_id UUID REFERENCES fa_offers(id) ON DELETE SET NULL,

    -- State machine
    -- pending_agent  : owner submitted, agent has not acted
    -- agent_rejected : agent declined; terminal
    -- agent_countered: agent counter-proposed; this row is now the historical original,
    --                  a new fa_offers row exists with parent_offer_id pointing here
    -- pending_team   : agent's counter is awaiting the team's accept/reject
    -- awaiting_match : agent accepted; bird-rights team has 48hr to match (RFA only)
    -- matched        : bird-rights team matched; player stays; offer terms applied; terminal
    -- walked         : bird-rights team let walk; player goes to offering team; finalize follows; terminal
    -- finalized      : contract written to player; team_id assigned; terminal
    -- withdrawn      : offering team withdrew; terminal
    status TEXT NOT NULL DEFAULT 'pending_agent' CHECK (status IN
        ('pending_agent','agent_rejected','agent_countered','pending_team',
         'awaiting_match','matched','walked','finalized','withdrawn')),

    -- Audit-light columns; full event log lives in fa_offer_events
    submitted_by UUID NOT NULL,                   -- user_id of GM who submitted
    decided_by   UUID,                            -- last actor (agent or bird-rights GM)
    submitted_at TIMESTAMP NOT NULL DEFAULT NOW(),
    decided_at   TIMESTAMP,
    match_window_opens_at  TIMESTAMP,
    match_window_closes_at TIMESTAMP,
    finalized_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_fa_offers_player ON fa_offers(player_id);
CREATE INDEX IF NOT EXISTS idx_fa_offers_agency_status ON fa_offers(agency_id, status);
CREATE INDEX IF NOT EXISTS idx_fa_offers_offering_team ON fa_offers(offering_team_id);
CREATE INDEX IF NOT EXISTS idx_fa_offers_bird_rights ON fa_offers(bird_rights_team_id);
CREATE INDEX IF NOT EXISTS idx_fa_offers_match_window ON fa_offers(match_window_closes_at) WHERE status = 'awaiting_match';

CREATE TABLE IF NOT EXISTS fa_offer_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    offer_id UUID NOT NULL REFERENCES fa_offers(id) ON DELETE CASCADE,
    actor_user_id UUID NOT NULL,
    action TEXT NOT NULL CHECK (action IN
        ('submitted','accepted','rejected','countered','team_accepted_counter','team_rejected_counter',
         'matched','walked','finalized','withdrawn','match_window_expired')),
    notes TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_fa_offer_events_offer ON fa_offer_events(offer_id, created_at);
