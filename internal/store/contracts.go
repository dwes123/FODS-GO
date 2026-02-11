package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OptionPlayer struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	TeamID     string  `json:"team_id"`
	TeamName   string  `json:"team_name"`
	LeagueID   string  `json:"league_id"`
	Year       int     `json:"year"`
	SalaryText string  `json:"salary_text"`
	Salary     float64 `json:"salary"`
	Buyout     float64 `json:"buyout"`
}

func GetPlayersWithOptions(db *pgxpool.Pool, teamID string, year int) ([]OptionPlayer, error) {
	col := fmt.Sprintf("contract_%d", year)
	query := fmt.Sprintf(`
		SELECT p.id, p.first_name || ' ' || p.last_name, p.team_id, t.name, p.league_id, %s
		FROM players p
		JOIN teams t ON p.team_id = t.id
		WHERE %s LIKE '%%(TO)%%'
	`, col, col)

	var args []interface{}
	if teamID != "" {
		query += " AND p.team_id = $1"
		args = append(args, teamID)
	}

	rows, err := db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []OptionPlayer
	for rows.Next() {
		var p OptionPlayer
		p.Year = year
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID, &p.TeamName, &p.LeagueID, &p.SalaryText); err != nil {
			continue
		}

		// Parse Salary
		clean := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(p.SalaryText, "(TO)", ""), "$", ""), ",", "")
		p.Salary, _ = strconv.ParseFloat(strings.TrimSpace(clean), 64)
		p.Buyout = p.Salary * 0.30

		players = append(players, p)
	}
	return players, nil
}

func ProcessOptionDecision(db *pgxpool.Pool, playerID string, year int, action string) error {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	// Get Current Data
	var pName, teamID, leagueID, salaryText string
	col := fmt.Sprintf("contract_%d", year)
	err = tx.QueryRow(ctx, fmt.Sprintf("SELECT first_name || ' ' || last_name, team_id, league_id, %s FROM players WHERE id = $1", col), playerID).Scan(&pName, &teamID, &leagueID, &salaryText)
	if err != nil { return err }

	clean := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(salaryText, "(TO)", ""), "$", ""), ",", "")
	salary, _ := strconv.ParseFloat(strings.TrimSpace(clean), 64)

	var summary string
	if action == "exercise" {
		// Just strip the (TO)
		newVal := fmt.Sprintf("%d", int64(salary))
		_, err = tx.Exec(ctx, fmt.Sprintf("UPDATE players SET %s = $1 WHERE id = $2", col), newVal, playerID)
		summary = fmt.Sprintf("%s EXERCISED the %d Team Option for %s ($%s).", teamID, year, pName, newVal)
	} else {
		// Decline: 30% buyout
		buyout := salary * 0.30
		buyoutVal := fmt.Sprintf("%d", int64(buyout))
		
		// 1. Current year becomes buyout (as Dead Cap)
		// Note: We'll set the contract value to the buyout, then drop the player.
		// The legacy site seems to keep the buyout in the contract field for the remainder of the year.
		
		// 2. Clear all years
		updateQuery := fmt.Sprintf(`
			UPDATE players SET 
				%s = $1,
				contract_2026 = CASE WHEN %d = 2026 THEN $1 ELSE '' END,
				contract_2027 = CASE WHEN %d = 2027 THEN $1 ELSE '' END,
				contract_2028 = CASE WHEN %d = 2028 THEN $1 ELSE '' END,
				contract_2029 = CASE WHEN %d = 2029 THEN $1 ELSE '' END,
				contract_2030 = CASE WHEN %d = 2030 THEN $1 ELSE '' END,
				contract_2031 = '', contract_2032 = '', contract_2033 = '', contract_2034 = '', contract_2035 = '',
				contract_2036 = '', contract_2037 = '', contract_2038 = '', contract_2039 = '', contract_2040 = '',
				team_id = NULL,
				status_40_man = false,
				status_26_man = false,
				fa_status = 'available'
			WHERE id = $2
		`, col, year, year, year, year, year)
		
		_, err = tx.Exec(ctx, updateQuery, buyoutVal, playerID)
		summary = fmt.Sprintf("%s DECLINED the %d Team Option for %s. Buyout: $%s. Player is now a Free Agent.", teamID, year, pName, buyoutVal)
	}

	if err != nil { return err }

	// Log Activity
	_, err = tx.Exec(ctx, `
		INSERT INTO activity_log (league_id, primary_team_id, transaction_type, summary)
		VALUES ($1, $2, 'Team Option', $3)
	`, leagueID, teamID, summary)

	if err != nil { return err }

	return tx.Commit(ctx)
}
