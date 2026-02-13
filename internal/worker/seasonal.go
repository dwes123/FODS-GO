package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StartSeasonalWorker runs hourly checks for seasonal resets.
// - Nov 1: Reset option_years_used for all players (Feature 12)
// - Oct 15: Clear IL statuses for all players (Feature 13)
func StartSeasonalWorker(db *pgxpool.Pool) {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			checkSeasonalTasks(db)
		}
	}()
}

func checkSeasonalTasks(db *pgxpool.Pool) {
	ctx := context.Background()
	now := time.Now()
	year := now.Year()
	month := now.Month()
	day := now.Day()

	// Nov 1: Reset option years used
	if month == 11 && day == 1 {
		key := fmt.Sprintf("seasonal_option_reset_%d", year)
		if !hasRunThisYear(db, ctx, key) {
			result, err := db.Exec(ctx, `UPDATE players SET option_years_used = 0 WHERE option_years_used > 0`)
			if err != nil {
				fmt.Printf("Seasonal Worker Error (option reset): %v\n", err)
				return
			}
			markAsRun(db, ctx, key)
			fmt.Printf("Seasonal Worker: Reset option_years_used for %d players\n", result.RowsAffected())
		}
	}

	// Oct 15: Clear IL statuses
	if month == 10 && day == 15 {
		key := fmt.Sprintf("seasonal_il_clear_%d", year)
		if !hasRunThisYear(db, ctx, key) {
			result, err := db.Exec(ctx, `UPDATE players SET status_il = NULL, il_start_date = NULL WHERE status_il IS NOT NULL`)
			if err != nil {
				fmt.Printf("Seasonal Worker Error (IL clear): %v\n", err)
				return
			}
			markAsRun(db, ctx, key)

			// Log batch transaction
			db.Exec(ctx, `
				INSERT INTO transactions (transaction_type, status, summary)
				VALUES ('SEASONAL', 'COMPLETED', $1)
			`, fmt.Sprintf("End of season IL clear — %d players activated", result.RowsAffected()))

			fmt.Printf("Seasonal Worker: Cleared IL for %d players\n", result.RowsAffected())
		}
	}
}

func hasRunThisYear(db *pgxpool.Pool, ctx context.Context, key string) bool {
	var count int
	err := db.QueryRow(ctx, `SELECT value FROM system_counters WHERE key = $1`, key).Scan(&count)
	return err == nil && count > 0
}

func markAsRun(db *pgxpool.Pool, ctx context.Context, key string) {
	db.Exec(ctx, `INSERT INTO system_counters (key, value) VALUES ($1, 1) ON CONFLICT (key) DO UPDATE SET value = 1`, key)
}
