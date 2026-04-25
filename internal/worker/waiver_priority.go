package worker

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/fantrax"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartWaiverPriorityWorker keeps teams.current_waiver_priority fresh.
// Runs once at startup, then daily at ~12:30 AM Pacific (just after the
// fantrax-fetch cache rolls over at midnight PT).
func StartWaiverPriorityWorker(ctx context.Context, db *pgxpool.Pool) {
	go func() {
		if err := RecomputeAllWaiverPriorities(ctx, db); err != nil {
			fmt.Printf("Waiver Priority Worker [startup]: %v\n", err)
		}
	}()

	ticker := time.NewTicker(15 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Waiver priority worker stopped")
				return
			case <-ticker.C:
				recomputeIfDue(ctx, db)
			}
		}
	}()
}

func recomputeIfDue(ctx context.Context, db *pgxpool.Pool) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PT", -8*3600)
	}
	now := time.Now().In(loc)
	if now.Hour() != 0 || now.Minute() < 30 {
		return
	}
	key := fmt.Sprintf("waiver_priority_recompute_%s", now.Format("2006-01-02"))
	if hasRunThisYear(db, ctx, key) {
		return
	}
	if err := RecomputeAllWaiverPriorities(ctx, db); err != nil {
		fmt.Printf("Waiver Priority Worker [scheduled]: %v\n", err)
		return
	}
	markAsRun(db, ctx, key)
}

// RecomputeAllWaiverPriorities fetches fresh standings for each linked league
// and writes 1..N to teams.current_waiver_priority. 1 = worst-standing team
// (picks first); N = best team. Tiebreaker: lower totalPointsFor wins the
// lower (better) priority number.
func RecomputeAllWaiverPriorities(ctx context.Context, db *pgxpool.Pool) error {
	rows, err := db.Query(ctx, `
		SELECT id::TEXT, name, fantrax_url
		FROM leagues
		WHERE fantrax_url IS NOT NULL AND fantrax_url <> ''
	`)
	if err != nil {
		return err
	}
	type lg struct{ id, name, url string }
	var leagues []lg
	for rows.Next() {
		var l lg
		if err := rows.Scan(&l.id, &l.name, &l.url); err == nil {
			leagues = append(leagues, l)
		}
	}
	rows.Close()

	for _, l := range leagues {
		// Force fresh fetch — daily recompute must not reuse yesterday's cache.
		fantrax.Invalidate(l.url)
		standings, err := fantrax.Fetch(l.url)
		if err != nil {
			fmt.Printf("Waiver Priority Worker [%s]: fetch failed: %v\n", l.name, err)
			continue
		}
		if len(standings) == 0 {
			fmt.Printf("Waiver Priority Worker [%s]: empty standings\n", l.name)
			continue
		}

		// Higher rank number = worse standing = picks first. Tiebreak by lower
		// totalPointsFor (worse offense = picks earlier among tied teams).
		sort.SliceStable(standings, func(i, j int) bool {
			if standings[i].Rank != standings[j].Rank {
				return standings[i].Rank > standings[j].Rank
			}
			return standings[i].TotalPointsFor < standings[j].TotalPointsFor
		})

		tx, err := db.Begin(ctx)
		if err != nil {
			fmt.Printf("Waiver Priority Worker [%s]: tx begin: %v\n", l.name, err)
			continue
		}
		if _, err := tx.Exec(ctx, "UPDATE teams SET current_waiver_priority = NULL WHERE league_id = $1", l.id); err != nil {
			tx.Rollback(ctx)
			fmt.Printf("Waiver Priority Worker [%s]: clear: %v\n", l.name, err)
			continue
		}
		var unmatched []string
		for i, s := range standings {
			tag, err := tx.Exec(ctx,
				"UPDATE teams SET current_waiver_priority = $1 WHERE league_id = $2 AND fantrax_team_id = $3",
				i+1, l.id, s.TeamID)
			if err != nil {
				fmt.Printf("Waiver Priority Worker [%s]: update %s: %v\n", l.name, s.TeamName, err)
				continue
			}
			if tag.RowsAffected() == 0 {
				unmatched = append(unmatched, fmt.Sprintf("%s (%s)", s.TeamName, s.TeamID))
			}
		}
		if err := tx.Commit(ctx); err != nil {
			fmt.Printf("Waiver Priority Worker [%s]: commit: %v\n", l.name, err)
			continue
		}
		if len(unmatched) > 0 {
			fmt.Printf("Waiver Priority Worker [%s]: %d unmatched fantrax_team_ids: %v\n", l.name, len(unmatched), unmatched)
		}
		fmt.Printf("Waiver Priority Worker [%s]: assigned priorities 1..%d\n", l.name, len(standings))
	}
	return nil
}
