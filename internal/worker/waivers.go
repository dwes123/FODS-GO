package worker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func StartWaiverWorker(db *pgxpool.Pool) {
	ticker := time.NewTicker(2 * time.Minute)
	go func() {
		for range ticker.C {
			processExpiredWaivers(db)
		}
	}()
}

func processExpiredWaivers(db *pgxpool.Pool) {
	ctx := context.Background()

	rows, err := db.Query(ctx, `
		SELECT id, first_name, last_name, league_id, waiving_team_id, COALESCE(dfa_clear_action, 'release')
		FROM players
		WHERE fa_status = 'on waivers' AND waiver_end_time <= NOW()
	`)
	if err != nil {
		fmt.Printf("Waiver Worker Error: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var pID, fName, lName, lID, waivingTeamID, clearAction string
		if err := rows.Scan(&pID, &fName, &lName, &lID, &waivingTeamID, &clearAction); err != nil {
			continue
		}

		var winningTeamID string
		err = db.QueryRow(ctx, `
			SELECT team_id FROM waiver_claims
			WHERE player_id = $1 AND status = 'pending'
			ORDER BY created_at ASC LIMIT 1
		`, pID).Scan(&winningTeamID)

		tx, err := db.Begin(ctx)
		if err != nil {
			continue
		}

		if winningTeamID != "" {
			_, err = tx.Exec(ctx, `
				UPDATE players SET
					team_id = $1,
					fa_status = 'rostered',
					status_40_man = TRUE,
					waiving_team_id = NULL,
					waiver_end_time = NULL,
					dfa_clear_action = NULL
				WHERE id = $2
			`, winningTeamID, pID)

			if err == nil {
				tx.Exec(ctx, "UPDATE waiver_claims SET status = 'processed' WHERE player_id = $1 AND team_id = $2", pID, winningTeamID)
				tx.Exec(ctx, "UPDATE waiver_claims SET status = 'invalid' WHERE player_id = $1 AND status = 'pending'", pID)
			}
		} else if clearAction == "minors" {
			// Send to minors: keep on team but off 40-man
			_, err = tx.Exec(ctx, `
				UPDATE players SET
					fa_status = 'rostered',
					status_40_man = FALSE,
					status_26_man = FALSE,
					waiving_team_id = NULL,
					waiver_end_time = NULL,
					dfa_clear_action = NULL
				WHERE id = $1
			`, pID)
		} else {
			// Release: calculate dead cap and release to free agency
			currentYear := time.Now().Year()
			applyDFADeadCap(tx, ctx, pID, waivingTeamID, currentYear)

			_, err = tx.Exec(ctx, `
				UPDATE players SET
					team_id = NULL,
					fa_status = 'available',
					waiving_team_id = NULL,
					waiver_end_time = NULL,
					dfa_clear_action = NULL,
					contract_2026 = NULL, contract_2027 = NULL, contract_2028 = NULL,
					contract_2029 = NULL, contract_2030 = NULL, contract_2031 = NULL,
					contract_2032 = NULL, contract_2033 = NULL, contract_2034 = NULL,
					contract_2035 = NULL, contract_2036 = NULL, contract_2037 = NULL,
					contract_2038 = NULL, contract_2039 = NULL, contract_2040 = NULL
				WHERE id = $1
			`, pID)
		}

		if err != nil {
			tx.Rollback(ctx)
			continue
		}
		tx.Commit(ctx)

		if winningTeamID != "" {
			fmt.Printf("Waiver Worker: %s %s claimed by Team %s\n", fName, lName, winningTeamID)
		} else if clearAction == "minors" {
			fmt.Printf("Waiver Worker: %s %s cleared waivers — sent to minors\n", fName, lName)
		} else {
			fmt.Printf("Waiver Worker: %s %s cleared waivers — released with dead cap\n", fName, lName)
		}
	}
}

// applyDFADeadCap calculates and inserts dead cap penalties for a DFA release.
// Current year: 75% of salary. Future years: 50% of salary.
func applyDFADeadCap(tx pgx.Tx, ctx context.Context, playerID, teamID string, currentYear int) {
	contractCols := []int{2026, 2027, 2028, 2029, 2030, 2031, 2032, 2033, 2034, 2035, 2036, 2037, 2038, 2039, 2040}

	for _, year := range contractCols {
		if year < currentYear {
			continue
		}
		var contractStr string
		col := fmt.Sprintf("contract_%d", year)
		err := tx.QueryRow(ctx, fmt.Sprintf("SELECT COALESCE(%s, '') FROM players WHERE id = $1", col), playerID).Scan(&contractStr)
		if err != nil || contractStr == "" {
			continue
		}

		// Skip non-dollar values like "TC", "ARB", "UFA"
		if !strings.Contains(contractStr, "$") && !strings.ContainsAny(contractStr, "0123456789") {
			continue
		}

		cleanStr := strings.ReplaceAll(strings.ReplaceAll(contractStr, "$", ""), ",", "")
		salary, _ := strconv.ParseFloat(cleanStr, 64)
		if salary <= 0 {
			continue
		}

		pct := 0.50 // future years
		if year == currentYear {
			pct = 0.75
		}
		deadCap := salary * pct

		tx.Exec(ctx, `
			INSERT INTO dead_cap_penalties (team_id, player_id, amount, year, note)
			VALUES ($1, $2, $3, $4, $5)
		`, teamID, playerID, deadCap, year, fmt.Sprintf("DFA Release (%.0f%%)", pct*100))
	}
}