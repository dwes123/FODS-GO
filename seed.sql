-- 1. Clear existing data just in case
TRUNCATE TABLE roster_entries, players, teams, leagues CASCADE;

-- 2. Create the 4 Moneyball Tiers (Leagues)
INSERT INTO leagues (id, name, league_type) VALUES
                                                ('11111111-1111-1111-1111-111111111111', 'MLB',    'Dynasty'),
                                                ('22222222-2222-2222-2222-222222222222', 'AAA',    'Dynasty'),
                                                ('33333333-3333-3333-3333-333333333333', 'AA',     'Dynasty'),
                                                ('44444444-4444-4444-4444-444444444444', 'High A', 'Dynasty');

-- 3. Create Teams in AAA
INSERT INTO teams (league_id, name, owner_name, email)
VALUES
    ('22222222-2222-2222-2222-222222222222', 'Bronx Bombers', 'Dan Wesdyk', 'dan@example.com'),
    ('22222222-2222-2222-2222-222222222222', 'Tacoma Rainiers', 'Scott Service', 'scott@example.com');

-- 4. Create a Player (Aaron Judge)
INSERT INTO players (id, mlb_id, first_name, last_name, position, mlb_team)
VALUES ('b5f3c2a1-8d4e-4b3a-9c1f-2e5d8a4b6c7d', '592450', 'Aaron', 'Judge', 'OF', 'NYY');

-- 5. Draft him to the Bronx Bombers
INSERT INTO roster_entries (team_id, player_id, status)
VALUES (
           (SELECT id FROM teams WHERE name = 'Bronx Bombers'),
           'b5f3c2a1-8d4e-4b3a-9c1f-2e5d8a4b6c7d',
           'ACTIVE'
       );