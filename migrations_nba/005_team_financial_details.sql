-- Adds the per-team financial header fields from each team tab in the
-- Dynasty Association xlsx (rows 8-15: Paid Up To, Need Billing, Cap Level,
-- Free Cap Space, MLE/TPMLE choice, BAE eligibility + usage, trade
-- restrictions). These are commissioner-curated values driving cap-compliance
-- workflows, not derived from contract math.

ALTER TABLE teams ADD COLUMN IF NOT EXISTS paid_up_to NUMERIC(14,2);
ALTER TABLE teams ADD COLUMN IF NOT EXISTS need_billing BOOLEAN;

-- Categorical: 'Below Salary Floor' / 'Below Soft Cap' / 'Above Soft Cap'
-- / 'Above Luxury Tax' / 'Above Apron #1' / 'Above Apron #2'
ALTER TABLE teams ADD COLUMN IF NOT EXISTS cap_level TEXT;

-- "Free Cap Space?" YES/NO from the sheet — separate from the numeric
-- cap_space column (which can be negative when over the cap).
ALTER TABLE teams ADD COLUMN IF NOT EXISTS free_cap_space_yn BOOLEAN;

-- Which mid-level exception is available given the cap level: 'MLE' or 'TPMLE' or 'NONE'
ALTER TABLE teams ADD COLUMN IF NOT EXISTS exception_type TEXT;

-- Mid-level exception usage (sum across split signings) and remaining headroom
ALTER TABLE teams ADD COLUMN IF NOT EXISTS mle_used NUMERIC(14,2);
ALTER TABLE teams ADD COLUMN IF NOT EXISTS mle_remaining NUMERIC(14,2);

-- Bi-Annual Exception
ALTER TABLE teams ADD COLUMN IF NOT EXISTS bae_available BOOLEAN;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS bae_eligible_year TEXT;
ALTER TABLE teams ADD COLUMN IF NOT EXISTS bae_used NUMERIC(14,2);
ALTER TABLE teams ADD COLUMN IF NOT EXISTS bae_remaining NUMERIC(14,2);

-- Free-text from the sheet, e.g. "Accept 125% + $100k of Outgoing Salary"
ALTER TABLE teams ADD COLUMN IF NOT EXISTS trade_restriction TEXT;
