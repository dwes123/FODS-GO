package nba

import (
	"context"
	"fmt"
	"time"

	nbastore "github.com/dwes123/fantasy-baseball-go/internal/store/nba"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartMatchWindowWorker auto-finalizes RFA offers whose 48-hour match window has
// closed without action. Runs every 30 minutes; lightweight (single SELECT + per-row tx).
func StartMatchWindowWorker(ctx context.Context, nbaDB *pgxpool.Pool) {
	go func() {
		// Initial run on boot in case the server was down when windows expired
		expireOnce(nbaDB)

		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("NBA match-window worker stopped")
				return
			case <-ticker.C:
				expireOnce(nbaDB)
			}
		}
	}()
}

func expireOnce(nbaDB *pgxpool.Pool) {
	count, err := nbastore.ExpireOpenMatchWindows(nbaDB)
	if err != nil {
		fmt.Printf("ERROR [NBA match-window worker]: %v\n", err)
		return
	}
	if count > 0 {
		fmt.Printf("NBA match-window: %d offers auto-finalized after window closed\n", count)
	}
}
