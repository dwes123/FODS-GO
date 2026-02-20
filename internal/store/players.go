package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PlayerSearchFilter struct {
	LeagueID string
	Position string
	Search   string
	IFAOnly  bool
	Limit    int
	Offset   int
}

func GetFreeAgents(db *pgxpool.Pool, filter PlayerSearchFilter) ([]RosterPlayer, error) {
	if filter.LeagueID == "" {
		return []RosterPlayer{}, nil
	}

	query := `
		SELECT id, first_name, last_name, position, mlb_team, COALESCE(fa_status, ''),
		       COALESCE(contract_2026, ''), COALESCE(is_international_free_agent, FALSE)
		FROM players
		WHERE (team_id IS NULL OR team_id = '00000000-0000-0000-0000-000000000000')
		AND league_id = $1
	`
	args := []interface{}{filter.LeagueID}
	argCount := 2

	if filter.Search != "" {
		query += fmt.Sprintf(" AND (first_name ILIKE $%d OR last_name ILIKE $%d)", argCount, argCount+1)
		args = append(args, "%"+filter.Search+"%", "%"+filter.Search+"%")
		argCount += 2
	}

	if filter.Position != "" {
		query += fmt.Sprintf(" AND position = $%d", argCount)
		args = append(args, filter.Position)
		argCount++
	}

	if filter.IFAOnly {
		query += " AND is_international_free_agent = TRUE"
	}

	query += " ORDER BY last_name ASC LIMIT 50"

	rows, err := db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []RosterPlayer
	for rows.Next() {
		var p RosterPlayer
		p.Contracts = make(map[int]string)
		var rawStatus, c26 string
		if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Position, &p.MLBTeam, &rawStatus, &c26, &p.IsIFA); err != nil {
			continue
		}
		p.Contracts[2026] = c26

		if rawStatus == "pending_bid" {
			p.Status = "Pending Bid"
		} else if rawStatus == "on waivers" {
			p.Status = "Waivers"
		} else {
			p.Status = "Available"
		}

		players = append(players, p)
	}

	return players, nil
}

func SearchAllPlayers(db *pgxpool.Pool, term string) ([]RosterPlayer, error) {
	rows, err := db.Query(context.Background(), `
		SELECT p.id, p.first_name, p.last_name, p.position, p.mlb_team, l.name
		FROM players p
		JOIN leagues l ON p.league_id = l.id
		WHERE p.first_name ILIKE $1 OR p.last_name ILIKE $1 
		ORDER BY l.name ASC, p.first_name ASC
		LIMIT 50
	`, "%"+term+"%")
	
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []RosterPlayer
	for rows.Next() {
		var p RosterPlayer
		if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Position, &p.MLBTeam, &p.LeagueName); err != nil {
			continue
		}
		players = append(players, p)
	}
	return players, nil
}

func GetPlayerByID(db *pgxpool.Pool, id string) (*RosterPlayer, error) {
	var p RosterPlayer
	p.Contracts = make(map[int]string)
	var rawStatus string
	var teamID *string
	var movesLogRaw []byte
	contracts := make([]string, 15)

	query := `
		SELECT p.id, p.first_name, p.last_name, p.position, p.mlb_team, p.fa_status,
		       p.status_40_man, p.status_26_man, COALESCE(p.status_il, ''), p.option_years_used,
		       p.team_id, p.league_id, l.name as league_name,
		       COALESCE(p.rule_5_eligibility_year, 0),
		       COALESCE(p.roster_moves_log, '[]'::jsonb),
		       COALESCE(p.is_international_free_agent, FALSE),
		       COALESCE(p.contract_2026, ''), COALESCE(p.contract_2027, ''), COALESCE(p.contract_2028, ''),
		       COALESCE(p.contract_2029, ''), COALESCE(p.contract_2030, ''), COALESCE(p.contract_2031, ''),
		       COALESCE(p.contract_2032, ''), COALESCE(p.contract_2033, ''), COALESCE(p.contract_2034, ''),
		       COALESCE(p.contract_2035, ''), COALESCE(p.contract_2036, ''), COALESCE(p.contract_2037, ''),
		       COALESCE(p.contract_2038, ''), COALESCE(p.contract_2039, ''), COALESCE(p.contract_2040, '')
		FROM players p
		JOIN leagues l ON p.league_id = l.id
		WHERE p.id = $1
	`

	dest := []interface{}{
		&p.ID, &p.FirstName, &p.LastName, &p.Position, &p.MLBTeam, &rawStatus,
		&p.Status40Man, &p.Status26Man, &p.StatusIL, &p.OptionYears,
		&teamID, &p.LeagueID, &p.LeagueName,
		&p.Rule5Year, &movesLogRaw, &p.IsIFA,
	}
	for i := range contracts {
		dest = append(dest, &contracts[i])
	}

	err := db.QueryRow(context.Background(), query, id).Scan(dest...)
	if err != nil {
		return nil, err
	}

	// Parse roster moves log JSONB
	if len(movesLogRaw) > 0 {
		json.Unmarshal(movesLogRaw, &p.RosterMovesLog)
	}

	for i, year := range []int{2026, 2027, 2028, 2029, 2030, 2031, 2032, 2033, 2034, 2035, 2036, 2037, 2038, 2039, 2040} {
		p.Contracts[year] = contracts[i]
	}

	isOnTeam := teamID != nil && *teamID != "00000000-0000-0000-0000-000000000000"

	if p.StatusIL != "" {
		p.Status = p.StatusIL
	} else if p.Status26Man {
		p.Status = "Active (26-Man)"
	} else if p.Status40Man {
		p.Status = "40-Man (Minors)"
	} else if isOnTeam {
		p.Status = "Minors (Non-40)"
	} else {
		if rawStatus == "pending_bid" {
			p.Status = "Pending Bid"
		} else if rawStatus == "on waivers" {
			p.Status = "Waivers"
		} else {
			p.Status = "Available"
		}
	}

	return &p, nil
}

// AppendRosterMove adds a move entry to a player's roster_moves_log JSONB column.
func AppendRosterMove(db *pgxpool.Pool, playerID, teamID, moveType string) {
	entry := fmt.Sprintf(`[{"type":"%s","date":"%s","team_id":"%s"}]`,
		moveType, time.Now().Format("2006-01-02"), teamID)
	db.Exec(context.Background(),
		`UPDATE players SET roster_moves_log = COALESCE(roster_moves_log, '[]'::jsonb) || $1::jsonb WHERE id = $2`,
		entry, playerID)
}

type TradeBlockPlayer struct {
	PlayerID        string
	PlayerName      string
	Position        string
	MLBTeam         string
	TeamID          string
	TeamName        string
	LeagueID        string
	LeagueName      string
	TradeBlockNotes string
	Contract2026    string
	Contract2027    string
	Contract2028    string
}

func GetTradeBlockPlayers(db *pgxpool.Pool, leagueID string) ([]TradeBlockPlayer, error) {
	ctx := context.Background()

	query := `
		SELECT p.id, p.first_name || ' ' || p.last_name, p.position, COALESCE(p.mlb_team, ''),
		       p.team_id, t.name, p.league_id, l.name,
		       COALESCE(p.trade_block_notes, ''),
		       COALESCE(p.contract_2026, ''), COALESCE(p.contract_2027, ''), COALESCE(p.contract_2028, '')
		FROM players p
		JOIN teams t ON p.team_id = t.id
		JOIN leagues l ON p.league_id = l.id
		WHERE p.on_trade_block = TRUE
	`
	args := []interface{}{}
	if leagueID != "" {
		query += " AND p.league_id = $1"
		args = append(args, leagueID)
	}
	query += " ORDER BY t.name ASC, p.last_name ASC"

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []TradeBlockPlayer
	for rows.Next() {
		var p TradeBlockPlayer
		if err := rows.Scan(&p.PlayerID, &p.PlayerName, &p.Position, &p.MLBTeam,
			&p.TeamID, &p.TeamName, &p.LeagueID, &p.LeagueName,
			&p.TradeBlockNotes, &p.Contract2026, &p.Contract2027, &p.Contract2028); err != nil {
			continue
		}
		players = append(players, p)
	}
	return players, nil
}