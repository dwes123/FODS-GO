package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WPWaiverClaim struct {
	ClaimingTeamID string `json:"claiming_team_id"`
	ClaimTimestamp string `json:"claim_timestamp"`
}

type ACFDataWaivers struct {
	FaStatus            string          `json:"fa_status"`
	WaiverEndTime       *string         `json:"waiver_end_time"`
	WaivingTeamID       string          `json:"waiving_team_id"`
	PendingWaiverClaims []WPWaiverClaim `json:"pending_waiver_claims"`
	LeagueID            string          `json:"league_id"`
}

type WPPlayerWaivers struct {
	ID  int            `json:"id"`
	ACF ACFDataWaivers `json:"acf"`
}

func main() {
	dbUrl := "postgres://admin:password123@localhost:5433/fantasy_db"
	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer db.Close()

	leagueMap := map[string]string{
		"MLB":    "11111111-1111-1111-1111-111111111111",
		"AAA":    "22222222-2222-2222-2222-222222222222",
		"AA":     "33333333-3333-3333-3333-333333333333",
		"High A": "44444444-4444-4444-4444-444444444444",
	}

	fmt.Println("Caching teams...")
	teamLookup := make(map[string]string)
	tRows, _ := db.Query(context.Background(), "SELECT id, abbreviation, league_id FROM teams WHERE abbreviation IS NOT NULL")
	for tRows.Next() {
		var id, abbr, lID string
		tRows.Scan(&id, &abbr, &lID)
		lKey := ""
		for k, v := range leagueMap {
			if v == lID {
				lKey = k
				break
			}
		}
		if lKey != "" {
			teamLookup[lKey+"_"+abbr] = id
		}
	}
	tRows.Close()

	fmt.Println("Syncing Waivers and Claims from Old Site...")

	page := 1
	for {
		url := "https://frontofficedynastysports.com/wp-json/wp/v2/playerdata?per_page=100&page=" + strconv.Itoa(page)
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode >= 400 {
			break
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var wpPlayers []WPPlayerWaivers
		json.Unmarshal(body, &wpPlayers)
		if len(wpPlayers) == 0 {
			break
		}

		for _, p := range wpPlayers {
			if p.ACF.FaStatus != "on waivers" {
				continue
			}

			lID, ok := leagueMap[p.ACF.LeagueID]
			if !ok {
				continue
			}

			var playerUUID string
			err := db.QueryRow(context.Background(), "SELECT id FROM players WHERE wp_id = $1", p.ID).Scan(&playerUUID)
			if err != nil {
				continue
			}

			var endTime *time.Time
			if p.ACF.WaiverEndTime != nil && *p.ACF.WaiverEndTime != "" {
				t, err := time.Parse("2006-01-02 15:04:05", *p.ACF.WaiverEndTime)
				if err == nil {
					endTime = &t
				}
			}

			waivingTeamUUID := teamLookup[p.ACF.LeagueID+"_"+p.ACF.WaivingTeamID]

			_, err = db.Exec(context.Background(), `
				UPDATE players SET 
					fa_status = 'on waivers',
					waiver_end_time = $1,
					waiving_team_id = $2
				WHERE id = $3
			`, endTime, waivingTeamUUID, playerUUID)

			if len(p.ACF.PendingWaiverClaims) > 0 {
				db.Exec(context.Background(), "DELETE FROM waiver_claims WHERE player_id = $1 AND status = 'pending'", playerUUID)
				for _, c := range p.ACF.PendingWaiverClaims {
					claimingTeamUUID := teamLookup[p.ACF.LeagueID+"_"+c.ClaimingTeamID]
					if claimingTeamUUID == "" {
						continue
					}

					ts, _ := time.Parse("2006-01-02 15:04:05", c.ClaimTimestamp)
					if ts.IsZero() {
						ts = time.Now()
					}

					db.Exec(context.Background(), `
						INSERT INTO waiver_claims (league_id, team_id, player_id, status, created_at)
						VALUES ($1, $2, $3, 'pending', $4)
					`, lID, claimingTeamUUID, playerUUID, ts)
				}
			}
			fmt.Printf("Synced waivers/claims for player WP_ID: %d\n", p.ID)
		}
		page++
	}

	fmt.Println("Waiver sync complete.")
}