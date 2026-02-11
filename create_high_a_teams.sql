-- ========================================================
-- CREATE HIGH-A TEAMS (South Atlantic & Midwest Leagues)
-- League ID: 44444444-4444-4444-4444-444444444444
-- ========================================================
INSERT INTO teams (id, name, league_id, owner_name) VALUES
-- SOUTH ATLANTIC LEAGUE (SAL)
(gen_random_uuid(), 'Aberdeen IronBirds',      '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Asheville Tourists',      '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Bowling Green Hot Rods',  '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Brooklyn Cyclones',       '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Greensboro Grasshoppers', '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Greenville Drive',        '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Hickory Crawdads',        '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Hub City Spartanburgers', '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Hudson Valley Renegades', '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Jersey Shore BlueClaws',  '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Rome Emperors',           '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Wilmington Blue Rocks',   '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Winston-Salem Dash',      '44444444-4444-4444-4444-444444444444', 'League Office'),

-- MIDWEST LEAGUE (MWL)
(gen_random_uuid(), 'Beloit Sky Carp',         '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Cedar Rapids Kernels',    '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Dayton Dragons',          '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Fort Wayne TinCaps',      '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Great Lakes Loons',       '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Lake County Captains',    '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Lansing Lugnuts',         '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Peoria Chiefs',           '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Quad Cities River Bandits','44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'South Bend Cubs',         '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'West Michigan Whitecaps', '44444444-4444-4444-4444-444444444444', 'League Office'),
(gen_random_uuid(), 'Wisconsin Timber Rattlers','44444444-4444-4444-4444-444444444444', 'League Office')
ON CONFLICT (name) DO NOTHING;