package nba

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Team is the canonical NBA fantasy team record.
type Team struct {
	ID                    string
	LeagueID              string
	Name                  string
	Abbreviation          string
	Conference            string  // "East" / "West"
	Division              string  // "Atlantic" / "Central" / "Southeast" / "Northwest" / "Pacific" / "Southwest"
	OwnerName             string  // free-text fallback when no team_owners row exists
	CapSpace              float64
	LuxuryTaxBalance      float64
	TradeExceptionBalance float64
	GLeagueBudget         float64
	FantraxURL            string

	// Per-team financial header (rows 8-15 of each team tab in the xlsx).
	// Curated by commissioners; drives cap-compliance workflows.
	PaidUpTo         *float64
	NeedBilling      *bool
	CapLevel         string  // "Below Salary Floor" / "Below Soft Cap" / "Above Soft Cap" / "Above Luxury Tax" / "Above Apron #1" / "Above Apron #2"
	FreeCapSpaceYN   *bool   // distinct from numeric CapSpace — the YES/NO flag from the sheet
	ExceptionType    string  // "MLE" / "TPMLE" / "NONE"
	MLEUsed          *float64
	MLERemaining     *float64
	BAEAvailable     *bool
	BAEEligibleYear  string
	BAEUsed          *float64
	BAERemaining     *float64
	TradeRestriction string

	// Populated by GetTeamWithRoster only.
	Roster []Player
}

// ListTeams returns every NBA team in the league, ordered by name.
func ListTeams(nbaDB *pgxpool.Pool) ([]Team, error) {
	rows, err := nbaDB.Query(context.Background(), `
		SELECT id, league_id, name, COALESCE(abbreviation, ''), COALESCE(conference, ''), COALESCE(division, ''),
		       COALESCE(owner_name, ''),
		       COALESCE(cap_space, 0), COALESCE(luxury_tax_balance, 0), COALESCE(trade_exception_balance, 0),
		       COALESCE(g_league_budget, 0),
		       COALESCE(fantrax_url, ''),
		       paid_up_to, need_billing, COALESCE(cap_level, ''), free_cap_space_yn,
		       COALESCE(exception_type, ''), mle_used, mle_remaining,
		       bae_available, COALESCE(bae_eligible_year, ''), bae_used, bae_remaining,
		       COALESCE(trade_restriction, '')
		FROM teams
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.LeagueID, &t.Name, &t.Abbreviation, &t.Conference, &t.Division, &t.OwnerName,
			&t.CapSpace, &t.LuxuryTaxBalance, &t.TradeExceptionBalance, &t.GLeagueBudget, &t.FantraxURL,
			&t.PaidUpTo, &t.NeedBilling, &t.CapLevel, &t.FreeCapSpaceYN,
			&t.ExceptionType, &t.MLEUsed, &t.MLERemaining,
			&t.BAEAvailable, &t.BAEEligibleYear, &t.BAEUsed, &t.BAERemaining,
			&t.TradeRestriction); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// GetTeamByID returns one team without its roster (cheap; use GetTeamWithRoster when players are needed).
func GetTeamByID(nbaDB *pgxpool.Pool, id string) (*Team, error) {
	var t Team
	err := nbaDB.QueryRow(context.Background(), `
		SELECT id, league_id, name, COALESCE(abbreviation, ''), COALESCE(conference, ''), COALESCE(division, ''),
		       COALESCE(owner_name, ''),
		       COALESCE(cap_space, 0), COALESCE(luxury_tax_balance, 0), COALESCE(trade_exception_balance, 0),
		       COALESCE(g_league_budget, 0),
		       COALESCE(fantrax_url, ''),
		       paid_up_to, need_billing, COALESCE(cap_level, ''), free_cap_space_yn,
		       COALESCE(exception_type, ''), mle_used, mle_remaining,
		       bae_available, COALESCE(bae_eligible_year, ''), bae_used, bae_remaining,
		       COALESCE(trade_restriction, '')
		FROM teams WHERE id = $1
	`, id).Scan(&t.ID, &t.LeagueID, &t.Name, &t.Abbreviation, &t.Conference, &t.Division, &t.OwnerName,
		&t.CapSpace, &t.LuxuryTaxBalance, &t.TradeExceptionBalance, &t.GLeagueBudget, &t.FantraxURL,
		&t.PaidUpTo, &t.NeedBilling, &t.CapLevel, &t.FreeCapSpaceYN,
		&t.ExceptionType, &t.MLEUsed, &t.MLERemaining,
		&t.BAEAvailable, &t.BAEEligibleYear, &t.BAEUsed, &t.BAERemaining,
		&t.TradeRestriction)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTeamWithRoster returns a team plus its full roster in one logical call.
// Two queries (no nested rows iteration) to avoid the connection-pool deadlock noted in CLAUDE.md.
func GetTeamWithRoster(nbaDB *pgxpool.Pool, id string) (*Team, error) {
	t, err := GetTeamByID(nbaDB, id)
	if err != nil {
		return nil, err
	}
	roster, err := GetTeamRoster(nbaDB, id)
	if err != nil {
		return nil, err
	}
	t.Roster = roster
	return t, nil
}

// GetManagedNBATeams returns all NBA teams owned by the given user (via team_owners junction).
// user_id is a soft FK to fantasy_db.users.id — no JOIN possible, app-layer maintains integrity.
func GetManagedNBATeams(nbaDB *pgxpool.Pool, userID string) ([]Team, error) {
	rows, err := nbaDB.Query(context.Background(), `
		SELECT t.id, t.league_id, t.name, COALESCE(t.abbreviation, ''), COALESCE(t.conference, ''), COALESCE(t.division, ''),
		       COALESCE(t.owner_name, ''),
		       COALESCE(t.cap_space, 0), COALESCE(t.luxury_tax_balance, 0), COALESCE(t.trade_exception_balance, 0),
		       COALESCE(t.g_league_budget, 0),
		       COALESCE(t.fantrax_url, ''),
		       t.paid_up_to, t.need_billing, COALESCE(t.cap_level, ''), t.free_cap_space_yn,
		       COALESCE(t.exception_type, ''), t.mle_used, t.mle_remaining,
		       t.bae_available, COALESCE(t.bae_eligible_year, ''), t.bae_used, t.bae_remaining,
		       COALESCE(t.trade_restriction, '')
		FROM teams t
		JOIN team_owners o ON o.team_id = t.id
		WHERE o.user_id = $1
		ORDER BY t.name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(&t.ID, &t.LeagueID, &t.Name, &t.Abbreviation, &t.Conference, &t.Division, &t.OwnerName,
			&t.CapSpace, &t.LuxuryTaxBalance, &t.TradeExceptionBalance, &t.GLeagueBudget, &t.FantraxURL,
			&t.PaidUpTo, &t.NeedBilling, &t.CapLevel, &t.FreeCapSpaceYN,
			&t.ExceptionType, &t.MLEUsed, &t.MLERemaining,
			&t.BAEAvailable, &t.BAEEligibleYear, &t.BAEUsed, &t.BAERemaining,
			&t.TradeRestriction); err != nil {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

// IsTeamOwner returns true iff the given user owns the team via team_owners.
func IsTeamOwner(nbaDB *pgxpool.Pool, teamID, userID string) (bool, error) {
	var n int
	err := nbaDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM team_owners WHERE team_id = $1 AND user_id = $2`,
		teamID, userID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
