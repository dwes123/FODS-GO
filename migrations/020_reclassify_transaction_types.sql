-- Reclassify transaction types into clean categories:
-- "Added Player", "Dropped Player", "Roster Move", "Trade"

-- ADD -> Added Player
UPDATE transactions SET transaction_type = 'Added Player' WHERE UPPER(TRIM(transaction_type)) = 'ADD';

-- DROP -> Dropped Player
UPDATE transactions SET transaction_type = 'Dropped Player' WHERE UPPER(TRIM(transaction_type)) = 'DROP';

-- TRADE -> Trade
UPDATE transactions SET transaction_type = 'Trade' WHERE UPPER(TRIM(transaction_type)) = 'TRADE';

-- ROSTER -> Roster Move
UPDATE transactions SET transaction_type = 'Roster Move' WHERE UPPER(TRIM(transaction_type)) = 'ROSTER';

-- WAIVER -> Added Player (waiver claims are player additions)
UPDATE transactions SET transaction_type = 'Added Player' WHERE UPPER(TRIM(transaction_type)) = 'WAIVER';

-- COMMISSIONER: reclassify based on summary content
-- Bids/signings -> Added Player
UPDATE transactions SET transaction_type = 'Added Player'
WHERE UPPER(TRIM(transaction_type)) = 'COMMISSIONER'
  AND (summary ILIKE '%signed%' OR summary ILIKE '%Free Agent Bid%' OR summary ILIKE '%claimed%');

-- DFA/released -> Dropped Player
UPDATE transactions SET transaction_type = 'Dropped Player'
WHERE UPPER(TRIM(transaction_type)) = 'COMMISSIONER'
  AND (summary ILIKE '%dfa%' OR summary ILIKE '%released%' OR summary ILIKE '%waived%' OR summary ILIKE '%dropped%');

-- Everything else in COMMISSIONER (roster moves, arbitration, promotions, etc.) -> Roster Move
UPDATE transactions SET transaction_type = 'Roster Move'
WHERE UPPER(TRIM(transaction_type)) = 'COMMISSIONER';

-- Catch any remaining oddball types
UPDATE transactions SET transaction_type = 'Roster Move'
WHERE transaction_type NOT IN ('Added Player', 'Dropped Player', 'Trade', 'Roster Move');
