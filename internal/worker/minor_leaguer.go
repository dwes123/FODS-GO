package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StartMinorLeaguerWorker checks career stats once per month and tags players with limited MLB experience.
// Ticks daily, uses system_counters keyed by year-month to run actual work once per month.
func StartMinorLeaguerWorker(ctx context.Context, db *pgxpool.Pool) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Minor leaguer worker stopped")
				return
			case <-ticker.C:
				key := fmt.Sprintf("minor_leaguer_check_%d_%d", time.Now().Year(), time.Now().Month())
				if hasRunThisYear(db, context.Background(), key) {
					continue
				}
				fmt.Println("Minor leaguer worker: starting monthly career stats check")
				ProcessMinorLeaguerCheck(ctx, db)
				markAsRun(db, context.Background(), key)
				fmt.Println("Minor leaguer worker: finished monthly career stats check")
			}
		}
	}()
}

// ProcessMinorLeaguerCheck fetches career stats from MLB API and updates is_minor_leaguer.
// Players without mlb_id are automatically marked as minor leaguers.
// Exported so it can be triggered from the admin refresh endpoint.
func ProcessMinorLeaguerCheck(ctx context.Context, db *pgxpool.Pool) {
	dbCtx := context.Background()

	// Step 1: Mark all players without mlb_id as minor leaguers (rostered or not)
	// If the MLB ID populator couldn't find them, they're almost certainly minor leaguers
	res, err := db.Exec(dbCtx, `UPDATE players SET is_minor_leaguer = TRUE
		WHERE (mlb_id IS NULL OR mlb_id = 0)
		AND is_minor_leaguer = FALSE`)
	if err != nil {
		fmt.Printf("ERROR [MinorLeaguerWorker]: bulk mark no-mlb-id: %v\n", err)
	} else {
		fmt.Printf("Minor leaguer worker: marked %d players without MLB ID as minor leaguers\n", res.RowsAffected())
	}

	// Step 2: Check career stats for players with mlb_id
	rows, err := db.Query(dbCtx, `SELECT DISTINCT mlb_id FROM players WHERE mlb_id IS NOT NULL AND mlb_id > 0`)
	if err != nil {
		fmt.Printf("ERROR [MinorLeaguerWorker]: query mlb_ids: %v\n", err)
		return
	}

	var mlbIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		mlbIDs = append(mlbIDs, id)
	}
	rows.Close()

	fmt.Printf("Minor leaguer worker: checking %d unique MLB IDs\n", len(mlbIDs))

	// Process in batches of 50
	batchSize := 50
	updated := 0
	for i := 0; i < len(mlbIDs); i += batchSize {
		select {
		case <-ctx.Done():
			fmt.Println("Minor leaguer worker: cancelled")
			return
		default:
		}

		end := i + batchSize
		if end > len(mlbIDs) {
			end = len(mlbIDs)
		}
		batch := mlbIDs[i:end]

		results, err := fetchCareerStats(batch)
		if err != nil {
			fmt.Printf("ERROR [MinorLeaguerWorker]: batch %d-%d API call: %v\n", i, end, err)
			time.Sleep(1 * time.Second)
			continue
		}

		for mlbID, isMinor := range results {
			_, err := db.Exec(dbCtx,
				`UPDATE players SET is_minor_leaguer = $1 WHERE mlb_id = $2`,
				isMinor, mlbID)
			if err != nil {
				fmt.Printf("ERROR [MinorLeaguerWorker]: update mlb_id %d: %v\n", mlbID, err)
				continue
			}
			updated++
		}

		// Rate limit between batches
		if end < len(mlbIDs) {
			time.Sleep(1 * time.Second)
		}
	}

	fmt.Printf("Minor leaguer worker: updated %d MLB IDs\n", updated)
}

// fetchCareerStats calls MLB Stats API for a batch of player IDs and returns minor leaguer status.
// A player is a "minor leaguer" if career IP <= 50 AND career AB <= 130.
func fetchCareerStats(mlbIDs []int) (map[int]bool, error) {
	ids := make([]string, len(mlbIDs))
	for i, id := range mlbIDs {
		ids[i] = strconv.Itoa(id)
	}

	url := fmt.Sprintf(
		"https://statsapi.mlb.com/api/v1/people?personIds=%s&hydrate=stats(group=[hitting,pitching],type=[career])",
		strings.Join(ids, ","),
	)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp struct {
		People []struct {
			ID    int `json:"id"`
			Stats []struct {
				Group struct {
					DisplayName string `json:"displayName"`
				} `json:"group"`
				Splits []struct {
					Stat json.RawMessage `json:"stat"`
				} `json:"splits"`
			} `json:"stats"`
		} `json:"people"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse API response: %w", err)
	}

	results := make(map[int]bool)

	for _, person := range apiResp.People {
		var careerIP float64
		var careerAB float64
		hasPitching := false
		hasHitting := false

		for _, statGroup := range person.Stats {
			if len(statGroup.Splits) == 0 {
				continue
			}
			raw := statGroup.Splits[0].Stat

			switch statGroup.Group.DisplayName {
			case "pitching":
				hasPitching = true
				var ps struct {
					InningsPitched string `json:"inningsPitched"`
				}
				if err := json.Unmarshal(raw, &ps); err == nil {
					careerIP = parseInningsPitched(ps.InningsPitched)
				}
			case "hitting":
				hasHitting = true
				var hs struct {
					AtBats json.Number `json:"atBats"`
				}
				if err := json.Unmarshal(raw, &hs); err == nil {
					careerAB, _ = hs.AtBats.Float64()
				}
			}
		}

		// Minor leaguer if: career IP <= 50 AND career AB <= 130
		// Players with no stats in either category count as 0 for that category
		_ = hasPitching
		_ = hasHitting
		isMinor := careerIP <= 50 && careerAB <= 130
		results[person.ID] = isMinor
	}

	return results, nil
}
