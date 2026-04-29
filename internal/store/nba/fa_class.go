package nba

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FA classification states. The fa_class column on players is one of these three values.
const (
	FAClassOwned     = "Owned"      // rostered with a guaranteed contract for upcoming year
	FAClassPending   = "Pending"    // rostered but contract is a cap hold / QO / UFA Year tag
	FAClassFreeAgent = "Free Agent" // not on any fantasy roster
)

// activeContractYear is the league season currently being negotiated. Static for now.
// When the league rolls over, bump this constant + redeploy.
const activeContractYear = 2026

// RecomputeAllFAClass updates every player's fa_class based on current contract data.
// Designed to be safe to call repeatedly. Returns the count of rows whose fa_class
// actually changed (useful for telemetry).
//
// Classification logic:
//   - team_id IS NULL                                              → 'Free Agent'
//   - contract_<active> IS NULL OR is a known transitional tag,    → 'Pending'
//     OR contract_annotations[<active>] contains 'UFA Year' /
//     'Qualifying Offer' / 'Qualifying Offer Extended'
//   - everything else                                              → 'Owned'
//
// All three values for the active year ARE source-of-truth. The daily worker calls this
// to keep the column in sync with any contract edits made by admins.
func RecomputeAllFAClass(nbaDB *pgxpool.Pool) (changed int64, err error) {
	ctx := context.Background()

	// One UPDATE for all three states. Mutually exclusive WHEN branches; first match wins.
	// We use the contract_<year> dynamic-column trick by hardcoding the active year into the SQL.
	col := fmt.Sprintf("contract_%d", activeContractYear)
	yearKey := fmt.Sprintf("%d", activeContractYear)

	stmt := fmt.Sprintf(`
		UPDATE players SET fa_class = sub.new_class
		FROM (
		    SELECT id,
		           CASE
		               WHEN team_id IS NULL THEN 'Free Agent'
		               WHEN %[1]s IS NULL OR %[1]s = '' THEN 'Pending'
		               WHEN %[1]s IN ('UFA Year', 'Qualifying Offer', 'Qualifying Offer Extended', 'G-League Contract') THEN 'Pending'
		               WHEN contract_annotations -> $1 ?| ARRAY['UFA Year', 'Qualifying Offer', 'Qualifying Offer Extended'] THEN 'Pending'
		               ELSE 'Owned'
		           END AS new_class
		    FROM players
		) sub
		WHERE players.id = sub.id AND players.fa_class IS DISTINCT FROM sub.new_class
	`, col)

	tag, err := nbaDB.Exec(ctx, stmt, yearKey)
	if err != nil {
		return 0, fmt.Errorf("RecomputeAllFAClass: %w", err)
	}
	return tag.RowsAffected(), nil
}
