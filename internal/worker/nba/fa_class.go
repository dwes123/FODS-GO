// Package nba contains NBA-side background workers (separate from the baseball worker package).
package nba

import (
	"context"
	"fmt"
	"time"

	nbastore "github.com/dwes123/fantasy-baseball-go/internal/store/nba"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartFAClassWorker recomputes players.fa_class from current contract data once per day.
// Runs once at startup (so a freshly-deployed instance has correct values immediately) then
// every 24 hours. Light enough to run on demand if needed — single UPDATE, no per-row work.
func StartFAClassWorker(ctx context.Context, nbaDB *pgxpool.Pool) {
	go func() {
		// Initial run on boot
		runOnce(nbaDB)

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("NBA FA-class worker stopped")
				return
			case <-ticker.C:
				runOnce(nbaDB)
			}
		}
	}()
}

func runOnce(nbaDB *pgxpool.Pool) {
	changed, err := nbastore.RecomputeAllFAClass(nbaDB)
	if err != nil {
		fmt.Printf("ERROR [NBA fa_class worker]: %v\n", err)
		return
	}
	if changed > 0 {
		fmt.Printf("NBA fa_class: %d players reclassified\n", changed)
	}
}
