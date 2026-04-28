// Package nba is the data-access layer for the basketball fantasy DB (fantasy_basketball_db).
// It mirrors the structure of internal/store but with NBA-specific schema.
//
// Pool conventions: every function takes nbaDB *pgxpool.Pool — that's the basketball pool only.
// User/auth lookups happen against the baseball pool elsewhere; nothing here queries that.
package nba

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Player is the canonical NBA player record. Fields mirror the players table in migrations_nba/001_init.sql.
type Player struct {
	ID            string
	NBAID         *int   // stats.nba.com player id (nullable until import)
	LeagueID      string
	TeamID        *string
	TeamName      string

	FirstName     string
	LastName      string
	Position      string // 'PG', 'SG', 'SF', 'PF', 'C', or comma list 'PG,SG'
	JerseyNumber  string
	HeightInches  *int
	WeightLbs     *int
	Age           *int
	College       string
	DraftYear     *int
	DraftPick     *int
	RealLifeTeam  string

	// Contracts: keyed by year (2026 → "$30000000" or "Team Option" or "UFA Year").
	Contracts map[int]string

	// Per-year tags stored alongside dollar amounts (e.g., "12/1 Trade Restriction").
	// Keyed by year as a string.
	Annotations map[string][]string

	OnTwoWay        bool
	OnActiveRoster  bool
	InjuryStatus    string
	FAStatus        string
	OnTradeBlock    bool

	BidEndTime    *time.Time
	WaiverEndTime *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// FreeAgentFilter shapes a free-agent search.
type FreeAgentFilter struct {
	Search   string // matches first or last name (ILIKE)
	Position string // exact match against players.position
	Limit    int
	Offset   int
}

const allContractYearsSelect = `
	COALESCE(p.contract_2026, ''), COALESCE(p.contract_2027, ''), COALESCE(p.contract_2028, ''),
	COALESCE(p.contract_2029, ''), COALESCE(p.contract_2030, ''), COALESCE(p.contract_2031, ''),
	COALESCE(p.contract_2032, ''), COALESCE(p.contract_2033, ''), COALESCE(p.contract_2034, ''),
	COALESCE(p.contract_2035, ''), COALESCE(p.contract_2036, ''), COALESCE(p.contract_2037, ''),
	COALESCE(p.contract_2038, ''), COALESCE(p.contract_2039, ''), COALESCE(p.contract_2040, '')
`

func scanContracts(p *Player, c [15]string) {
	if p.Contracts == nil {
		p.Contracts = make(map[int]string)
	}
	for i, v := range c {
		p.Contracts[2026+i] = v
	}
}

// GetFreeAgents returns NBA free agents (team_id IS NULL) matching the filter.
func GetFreeAgents(nbaDB *pgxpool.Pool, filter FreeAgentFilter) ([]Player, error) {
	query := `
		SELECT p.id, p.nba_id,
		       p.first_name, p.last_name, COALESCE(p.position, ''), COALESCE(p.real_life_team, ''),
		       COALESCE(p.fa_status, ''), COALESCE(p.injury_status, ''),
		       p.on_trade_block, p.on_two_way, p.on_active_roster,
		       ` + allContractYearsSelect + `
		FROM players p
		WHERE p.team_id IS NULL
	`
	args := []interface{}{}
	argN := 1

	if filter.Search != "" {
		query += fmt.Sprintf(" AND (p.first_name ILIKE $%d OR p.last_name ILIKE $%d)", argN, argN+1)
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
		argN += 2
	}
	if filter.Position != "" {
		// Exact match OR comma-list contains (handles "PG,SG")
		query += fmt.Sprintf(" AND (p.position = $%d OR p.position LIKE $%d OR p.position LIKE $%d OR p.position LIKE $%d)", argN, argN+1, argN+2, argN+3)
		args = append(args, filter.Position, filter.Position+",%", "%,"+filter.Position, "%,"+filter.Position+",%")
		argN += 4
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query += fmt.Sprintf(" ORDER BY p.last_name ASC, p.first_name ASC LIMIT $%d OFFSET $%d", argN, argN+1)
	args = append(args, limit, filter.Offset)

	rows, err := nbaDB.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Player
	for rows.Next() {
		var p Player
		var contracts [15]string
		if err := rows.Scan(
			&p.ID, &p.NBAID,
			&p.FirstName, &p.LastName, &p.Position, &p.RealLifeTeam,
			&p.FAStatus, &p.InjuryStatus,
			&p.OnTradeBlock, &p.OnTwoWay, &p.OnActiveRoster,
			&contracts[0], &contracts[1], &contracts[2], &contracts[3], &contracts[4],
			&contracts[5], &contracts[6], &contracts[7], &contracts[8], &contracts[9],
			&contracts[10], &contracts[11], &contracts[12], &contracts[13], &contracts[14],
		); err != nil {
			continue
		}
		scanContracts(&p, contracts)
		out = append(out, p)
	}
	return out, nil
}

// CountFreeAgents returns the total free-agent count matching the filter (for pagination).
func CountFreeAgents(nbaDB *pgxpool.Pool, filter FreeAgentFilter) (int, error) {
	query := `SELECT COUNT(*) FROM players p WHERE p.team_id IS NULL`
	args := []interface{}{}
	argN := 1

	if filter.Search != "" {
		query += fmt.Sprintf(" AND (p.first_name ILIKE $%d OR p.last_name ILIKE $%d)", argN, argN+1)
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
		argN += 2
	}
	if filter.Position != "" {
		query += fmt.Sprintf(" AND (p.position = $%d OR p.position LIKE $%d OR p.position LIKE $%d OR p.position LIKE $%d)", argN, argN+1, argN+2, argN+3)
		args = append(args, filter.Position, filter.Position+",%", "%,"+filter.Position, "%,"+filter.Position+",%")
	}

	var n int
	err := nbaDB.QueryRow(context.Background(), query, args...).Scan(&n)
	return n, err
}

// GetPlayerByID returns the full player record (including contracts and annotations) by UUID.
func GetPlayerByID(nbaDB *pgxpool.Pool, id string) (*Player, error) {
	query := `
		SELECT p.id, p.nba_id, p.league_id, p.team_id,
		       p.first_name, p.last_name, COALESCE(p.position, ''), COALESCE(p.jersey_number, ''),
		       p.height_inches, p.weight_lbs, p.age,
		       COALESCE(p.college, ''), p.draft_year, p.draft_pick, COALESCE(p.real_life_team, ''),
		       COALESCE(p.fa_status, ''), COALESCE(p.injury_status, ''),
		       p.on_trade_block, p.on_two_way, p.on_active_roster,
		       p.bid_end_time, p.waiver_end_time,
		       COALESCE(p.contract_annotations, '{}'::jsonb),
		       COALESCE(t.name, ''),
		       p.created_at, p.updated_at,
		       ` + allContractYearsSelect + `
		FROM players p
		LEFT JOIN teams t ON p.team_id = t.id
		WHERE p.id = $1
	`
	var p Player
	var contracts [15]string
	var annotationsJSON []byte
	err := nbaDB.QueryRow(context.Background(), query, id).Scan(
		&p.ID, &p.NBAID, &p.LeagueID, &p.TeamID,
		&p.FirstName, &p.LastName, &p.Position, &p.JerseyNumber,
		&p.HeightInches, &p.WeightLbs, &p.Age,
		&p.College, &p.DraftYear, &p.DraftPick, &p.RealLifeTeam,
		&p.FAStatus, &p.InjuryStatus,
		&p.OnTradeBlock, &p.OnTwoWay, &p.OnActiveRoster,
		&p.BidEndTime, &p.WaiverEndTime,
		&annotationsJSON,
		&p.TeamName,
		&p.CreatedAt, &p.UpdatedAt,
		&contracts[0], &contracts[1], &contracts[2], &contracts[3], &contracts[4],
		&contracts[5], &contracts[6], &contracts[7], &contracts[8], &contracts[9],
		&contracts[10], &contracts[11], &contracts[12], &contracts[13], &contracts[14],
	)
	if err != nil {
		return nil, err
	}
	scanContracts(&p, contracts)

	if len(annotationsJSON) > 0 {
		_ = json.Unmarshal(annotationsJSON, &p.Annotations)
	}
	return &p, nil
}

// GetTradeBlockPlayers returns all players currently flagged on the trade block, with their team name.
func GetTradeBlockPlayers(nbaDB *pgxpool.Pool) ([]Player, error) {
	query := `
		SELECT p.id, p.nba_id,
		       p.first_name, p.last_name, COALESCE(p.position, ''), COALESCE(p.real_life_team, ''),
		       COALESCE(p.fa_status, ''), COALESCE(p.injury_status, ''),
		       p.on_trade_block, p.on_two_way, p.on_active_roster,
		       COALESCE(t.name, ''),
		       ` + allContractYearsSelect + `
		FROM players p
		LEFT JOIN teams t ON p.team_id = t.id
		WHERE p.on_trade_block = TRUE
		ORDER BY t.name ASC, p.last_name ASC
	`
	rows, err := nbaDB.Query(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Player
	for rows.Next() {
		var p Player
		var contracts [15]string
		if err := rows.Scan(
			&p.ID, &p.NBAID,
			&p.FirstName, &p.LastName, &p.Position, &p.RealLifeTeam,
			&p.FAStatus, &p.InjuryStatus,
			&p.OnTradeBlock, &p.OnTwoWay, &p.OnActiveRoster,
			&p.TeamName,
			&contracts[0], &contracts[1], &contracts[2], &contracts[3], &contracts[4],
			&contracts[5], &contracts[6], &contracts[7], &contracts[8], &contracts[9],
			&contracts[10], &contracts[11], &contracts[12], &contracts[13], &contracts[14],
		); err != nil {
			continue
		}
		scanContracts(&p, contracts)
		out = append(out, p)
	}
	return out, nil
}

// GetTeamRoster returns all players assigned to the given team, ordered for display.
// Sort order: active two-way last, then by position, then by last name.
func GetTeamRoster(nbaDB *pgxpool.Pool, teamID string) ([]Player, error) {
	query := `
		SELECT p.id, p.nba_id,
		       p.first_name, p.last_name, COALESCE(p.position, ''), COALESCE(p.real_life_team, ''),
		       COALESCE(p.fa_status, ''), COALESCE(p.injury_status, ''),
		       p.on_trade_block, p.on_two_way, p.on_active_roster,
		       ` + allContractYearsSelect + `
		FROM players p
		WHERE p.team_id = $1
		ORDER BY p.on_two_way ASC, p.position ASC, p.last_name ASC
	`
	rows, err := nbaDB.Query(context.Background(), query, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Player
	for rows.Next() {
		var p Player
		var contracts [15]string
		if err := rows.Scan(
			&p.ID, &p.NBAID,
			&p.FirstName, &p.LastName, &p.Position, &p.RealLifeTeam,
			&p.FAStatus, &p.InjuryStatus,
			&p.OnTradeBlock, &p.OnTwoWay, &p.OnActiveRoster,
			&contracts[0], &contracts[1], &contracts[2], &contracts[3], &contracts[4],
			&contracts[5], &contracts[6], &contracts[7], &contracts[8], &contracts[9],
			&contracts[10], &contracts[11], &contracts[12], &contracts[13], &contracts[14],
		); err != nil {
			continue
		}
		scanContracts(&p, contracts)
		out = append(out, p)
	}
	return out, nil
}

// FullName returns "First Last" for display.
func (p *Player) FullName() string {
	return strings.TrimSpace(p.FirstName + " " + p.LastName)
}
