// Package sport contains shared constants and helpers for sport-specific logic.
// nba.go holds NBA-specific values: positions, contract tags, roster limits, league IDs.
package sport

import "strings"

// LeagueID is the fixed UUID for the single NBA league. Mirrors the seed in migrations_nba/001_init.sql.
const NBALeagueID = "55555555-5555-5555-5555-555555555555"

// Sport identifiers used in templates' .Sport field and league_roles.sport column.
const (
	SportMLB = "mlb"
	SportNBA = "nba"
)

// NBA position codes. A player's `position` column may hold a single code or a comma list (e.g., "PG,SG").
const (
	PositionPG = "PG"
	PositionSG = "SG"
	PositionSF = "SF"
	PositionPF = "PF"
	PositionC  = "C"
)

// NBAPositions is the canonical ordered list of NBA positions for UI tabs.
var NBAPositions = []string{PositionPG, PositionSG, PositionSF, PositionPF, PositionC}

// NBA contract tags. Stored as the value of a contract_YYYY column when not a dollar amount.
const (
	ContractTeamOption              = "Team Option"
	ContractPlayerOption            = "Player Option"
	ContractQualifyingOffer         = "Qualifying Offer"
	ContractQualifyingOfferExtended = "Qualifying Offer Extended"
	ContractUFAYear                 = "UFA Year"
)

// NBA contract annotations. Stored in the players.contract_annotations JSONB column,
// keyed by year, alongside a dollar amount in contract_YYYY.
//
// Example:
//
//	contract_2026 = "$30000000"
//	contract_annotations = {"2026": ["12/1 Trade Restriction", "DPE Designation"]}
const (
	AnnotationTwelveOneTradeRestriction = "12/1 Trade Restriction"
	AnnotationDPEDesignation            = "DPE Designation"
)

// NBARosterLimits — defaults; overridable via league_settings table.
const (
	NBAStandardRosterLimit = 15
	NBATwoWayLimit         = 3
)

// IsContractTag reports whether v is a recognized non-dollar contract tag.
// Used to decide between dollar-amount parsing vs tag handling when reading a contract_YYYY column.
func IsContractTag(v string) bool {
	switch strings.TrimSpace(v) {
	case ContractTeamOption,
		ContractPlayerOption,
		ContractQualifyingOffer,
		ContractQualifyingOfferExtended,
		ContractUFAYear:
		return true
	}
	return false
}

// IsContractAnnotation reports whether v is a recognized contract-year annotation
// (i.e., a flag stored alongside a dollar amount, not in place of one).
func IsContractAnnotation(v string) bool {
	switch strings.TrimSpace(v) {
	case AnnotationTwelveOneTradeRestriction, AnnotationDPEDesignation:
		return true
	}
	return false
}

// NormalizePositions splits and trims a comma-separated NBA position string.
// "PG, SG" -> ["PG", "SG"].
func NormalizePositions(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(strings.ToUpper(p))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
