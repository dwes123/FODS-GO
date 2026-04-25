-- Add fantrax_url to leagues
ALTER TABLE leagues ADD COLUMN IF NOT EXISTS fantrax_url TEXT;

-- Seed default URLs
UPDATE leagues SET fantrax_url = 'https://www.fantrax.com/fxea/general/getStandings?leagueId=w4wlt4b2mg5l9qja' WHERE slug = 'mlb';
UPDATE leagues SET fantrax_url = 'https://www.fantrax.com/fxea/general/getStandings?leagueId=m7qxa1w9mg5uk295' WHERE slug = 'aaa';
UPDATE leagues SET fantrax_url = 'https://www.fantrax.com/fxea/general/getStandings?leagueId=q0zuqdpdmg7xgmfg' WHERE slug = 'aa';
UPDATE leagues SET fantrax_url = 'https://www.fantrax.com/fxea/general/getStandings?leagueId=7zga7v6gmgz8sd3p' WHERE slug = 'high-a';
