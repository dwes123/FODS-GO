package worker

import (
	"context"
	"fmt"
	"time"

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
		SELECT id, first_name, last_name, league_id, waiving_team_id
		FROM players
		WHERE fa_status = 'on waivers' AND waiver_end_time <= NOW()
	`)
	if err != nil {
		fmt.Printf("Waiver Worker Error: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var pID, fName, lName, lID, waivingTeamID string
		if err := rows.Scan(&pID, &fName, &lName, &lID, &waivingTeamID); err != nil {
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
					waiver_end_time = NULL
				WHERE id = $2
			`, winningTeamID, pID)

			if err == nil {
				tx.Exec(ctx, "UPDATE waiver_claims SET status = 'processed' WHERE player_id = $1 AND team_id = $2", pID, winningTeamID)
				tx.Exec(ctx, "UPDATE waiver_claims SET status = 'invalid' WHERE player_id = $1 AND status = 'pending'", pID)
			}
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE players SET
					team_id = NULL,
					fa_status = 'available',
					waiving_team_id = NULL,
					waiver_end_time = NULL
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
		} else {
			fmt.Printf("Waiver Worker: %s %s cleared waivers\n", fName, lName)
		}
	}
}