package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// parseContractAmount parses contract TEXT values like "$1,000,000", "1000000", "1000000(TO)" into float64.
func parseContractAmount(val string) float64 {
	val = strings.ReplaceAll(val, "$", "")
	val = strings.ReplaceAll(val, ",", "")
	val = strings.Split(val, "(")[0] // strip (TO) suffix
	val = strings.TrimSpace(val)
	f, _ := strconv.ParseFloat(val, 64)
	return f
}

type ArbitrationPlayer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	TeamID        string `json:"team_id"`
	TeamName      string `json:"team_name"`
	LeagueID      string `json:"league_id"`
	CurrentStatus string `json:"current_status"` // e.g. "ARB 1"
	PendingStatus string `json:"pending_status"` // 'PENDING' if a request exists
}

type PendingAction struct {
	ID           string  `json:"id"`
	PlayerID     string  `json:"player_id"`
	PlayerName   string  `json:"player_name"`
	TeamName     string  `json:"team_name"`
	ActionType   string  `json:"action_type"`
	TargetYear   int     `json:"target_year"`
	SalaryAmount float64 `json:"salary_amount"`
	Summary      string  `json:"summary"`
	Status       string  `json:"status"`
}

func GetArbitrationEligiblePlayers(db *pgxpool.Pool, teamID string, year int) ([]ArbitrationPlayer, error) {
	ctx := context.Background()
	contractCol := fmt.Sprintf("contract_%d", year)

	query := fmt.Sprintf(`
		SELECT p.id, p.first_name || ' ' || p.last_name, p.team_id, t.name, p.league_id, p.%s,
		       COALESCE(pa.status, '')
		FROM players p
		JOIN teams t ON p.team_id = t.id
		LEFT JOIN pending_actions pa ON p.id = pa.player_id 
		     AND pa.action_type = 'ARBITRATION' 
		     AND pa.target_year = $2 
		     AND pa.status = 'PENDING'
		WHERE p.team_id = $1 AND p.%s ILIKE '%%ARB%%'
		ORDER BY p.last_name
	`, contractCol, contractCol)

	rows, err := db.Query(ctx, query, teamID, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []ArbitrationPlayer
	for rows.Next() {
		var p ArbitrationPlayer
		if err := rows.Scan(&p.ID, &p.Name, &p.TeamID, &p.TeamName, &p.LeagueID, &p.CurrentStatus, &p.PendingStatus); err != nil {
			continue
		}
		players = append(players, p)
	}
	return players, nil
}

func SubmitArbitrationDecision(db *pgxpool.Pool, playerID, teamID, leagueID string, year int, amount float64, decline bool) error {
	ctx := context.Background()
	
	if decline {
		tx, err := db.Begin(ctx)
		if err != nil { return err }
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `
			UPDATE players SET team_id = NULL, fa_status = 'available', status_40_man = FALSE, status_26_man = FALSE
			WHERE id = $1
		`, playerID)
		if err != nil { return err }

		_, err = tx.Exec(ctx, `
			INSERT INTO transactions (team_id, player_id, transaction_type, status)
			VALUES ($1, $2, 'Dropped Player', 'COMPLETED')
		`, teamID, playerID)
		if err != nil { return err }

		return tx.Commit(ctx)
	}

	_, err := db.Exec(ctx, `
		INSERT INTO pending_actions (player_id, team_id, league_id, action_type, target_year, salary_amount, status)
		VALUES ($1, $2, $3, 'ARBITRATION', $4, $5, 'PENDING')
	`, playerID, teamID, leagueID, year, amount)

	return err
}

func SubmitExtension(db *pgxpool.Pool, playerID, teamID, leagueID string, salaries map[string]float64) error {
	ctx := context.Background()
	
	salariesJSON, err := json.Marshal(salaries)
	if err != nil { return err }

	_, err = db.Exec(ctx, `
		INSERT INTO pending_actions (player_id, team_id, league_id, action_type, multi_year_contract, status)
		VALUES ($1, $2, $3, 'EXTENSION', $4, 'PENDING')
	`, playerID, teamID, leagueID, salariesJSON)

	return err
}

func GetAllPendingActions(db *pgxpool.Pool) ([]PendingAction, error) {
	return GetPendingActionsForLeagues(db, nil) // All
}

func GetPendingActionsForLeagues(db *pgxpool.Pool, leagueIDs []string) ([]PendingAction, error) {
	query := `
		SELECT pa.id, COALESCE(pa.player_id::TEXT, ''), COALESCE(p.first_name || ' ' || p.last_name, ''), t.name,
		       pa.action_type, COALESCE(pa.target_year, 0), COALESCE(pa.salary_amount, 0), COALESCE(pa.summary, ''), pa.status
		FROM pending_actions pa
		LEFT JOIN players p ON pa.player_id = p.id
		JOIN teams t ON pa.team_id = t.id
		WHERE pa.status = 'PENDING'
	`
	var args []interface{}
	if leagueIDs != nil {
		query += " AND pa.league_id = ANY($1)"
		args = append(args, leagueIDs)
	}
	query += " ORDER BY pa.created_at ASC"

	rows, err := db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []PendingAction
	for rows.Next() {
		var a PendingAction
		if err := rows.Scan(&a.ID, &a.PlayerID, &a.PlayerName, &a.TeamName, &a.ActionType, &a.TargetYear, &a.SalaryAmount, &a.Summary, &a.Status); err != nil {
			continue
		}
		actions = append(actions, a)
	}
	return actions, nil
}

func ProcessAction(db *pgxpool.Pool, actionID, status string) error {
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)

	// 1. Get action details
	var pID *string
	var aType string
	var year *int
	var amount *float64
	var multiYear []byte
	err = tx.QueryRow(ctx, `SELECT player_id, action_type, target_year, salary_amount, multi_year_contract FROM pending_actions WHERE id = $1`, actionID).Scan(&pID, &aType, &year, &amount, &multiYear)
	if err != nil { return err }

	if status == "APPROVED" {
		if aType == "ARBITRATION" && pID != nil && year != nil && amount != nil {
			// Update Player Record
			contractCol := fmt.Sprintf("contract_%d", *year)
			_, err = tx.Exec(ctx, fmt.Sprintf("UPDATE players SET %s = $1 WHERE id = $2", contractCol), fmt.Sprintf("%.2f", *amount), *pID)
			if err != nil { return err }
		} else if aType == "EXTENSION" && pID != nil {
			var salaries map[string]float64
			json.Unmarshal(multiYear, &salaries)
			for yr, amt := range salaries {
				contractCol := fmt.Sprintf("contract_%s", yr)
				_, err = tx.Exec(ctx, fmt.Sprintf("UPDATE players SET %s = $1 WHERE id = $2", contractCol), fmt.Sprintf("%.2f", amt), *pID)
				if err != nil { return err }
			}
		} else if aType == "RESTRUCTURE" && pID != nil && len(multiYear) > 0 {
			var data map[string]string
			json.Unmarshal(multiYear, &data)
			fromYear := data["from_year"]
			toYear := data["to_year"]
			moveAmount, _ := strconv.ParseFloat(data["amount"], 64)

			if fromYear != "" && toYear != "" && moveAmount > 0 {
				fromCol := fmt.Sprintf("contract_%s", fromYear)
				toCol := fmt.Sprintf("contract_%s", toYear)

				// Read current values
				var fromVal, toVal string
				err = tx.QueryRow(ctx, fmt.Sprintf("SELECT COALESCE(%s, '0'), COALESCE(%s, '0') FROM players WHERE id = $1", fromCol, toCol), *pID).Scan(&fromVal, &toVal)
				if err != nil { return err }

				fromAmount := parseContractAmount(fromVal)
				toAmount := parseContractAmount(toVal)

				newFrom := fromAmount - moveAmount
				newTo := toAmount + moveAmount

				_, err = tx.Exec(ctx, fmt.Sprintf("UPDATE players SET %s = $1, %s = $2 WHERE id = $3", fromCol, toCol),
					fmt.Sprintf("%.2f", newFrom), fmt.Sprintf("%.2f", newTo), *pID)
				if err != nil { return err }
			}
		}
	}

	// 2. Update Action Status
	_, err = tx.Exec(ctx, "UPDATE pending_actions SET status = $1 WHERE id = $2", status, actionID)
	if err != nil { return err }

	return tx.Commit(ctx)
}