package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func StartBidWorker(ctx context.Context, db *pgxpool.Pool) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Bid worker stopped")
				return
			case <-ticker.C:
				finalizeBids(db)
			}
		}
	}()
}

func finalizeBids(db *pgxpool.Pool) {
	ctx := context.Background()

	rows, err := db.Query(ctx, `
		SELECT id, first_name, last_name, pending_bid_team_id, pending_bid_years, pending_bid_aav,
		       COALESCE(bid_type, 'standard')
		FROM players
		WHERE fa_status = 'pending_bid' AND bid_end_time <= NOW()
	`)
	if err != nil {
		fmt.Printf("Worker Error: %v\n", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var pID, fName, lName, teamID, bidType string
		var years int
		var aav float64
		if err := rows.Scan(&pID, &fName, &lName, &teamID, &years, &aav, &bidType); err != nil {
			continue
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			continue
		}

		if bidType == "ifa" {
			// IFA signing: deduct from ISBP, no contract written
			_, err = tx.Exec(ctx, `
				UPDATE players SET
					team_id = $1,
					fa_status = 'rostered',
					status_40_man = TRUE,
					pending_bid_amount = NULL,
					pending_bid_team_id = NULL,
					is_international_free_agent = FALSE
				WHERE id = $2
			`, teamID, pID)
			if err != nil {
				tx.Rollback(ctx)
				continue
			}

			_, err = tx.Exec(ctx, `UPDATE teams SET isbp_balance = isbp_balance - $1 WHERE id = $2`, aav, teamID)
			if err != nil {
				tx.Rollback(ctx)
				fmt.Printf("❌ Worker: Failed to deduct ISBP for %s %s: %v\n", fName, lName, err)
				continue
			}
			fmt.Printf("💰 Worker: Deducted $%.0f ISBP from Team %s for IFA signing of %s %s\n", aav, teamID, fName, lName)
		} else {
			// Standard signing: write contract
			_, err = tx.Exec(ctx, `
				UPDATE players SET
					team_id = $1,
					fa_status = 'rostered',
					contract_2026 = $2,
					status_40_man = TRUE,
					pending_bid_amount = NULL,
					pending_bid_team_id = NULL
				WHERE id = $3
			`, teamID, fmt.Sprintf("%.0f", aav), pID)

			if err != nil {
				tx.Rollback(ctx)
				continue
			}
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO transactions (team_id, player_id, transaction_type, status)
			VALUES ($1, $2, 'Added Player', 'COMPLETED')
		`, teamID, pID)

		if err != nil {
			tx.Rollback(ctx)
			continue
		}

		tx.Commit(ctx)
		fmt.Printf("✅ Worker: %s %s signed by Team %s for %d years at %.0f AAV\n", fName, lName, teamID, years, aav)
	}
}