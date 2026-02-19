package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
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
	database := db.InitDB()
	defer database.Close()

	leagueMap := map[string]string{
		"MLB":    "11111111-1111-1111-1111-111111111111",
		"AAA":    "22222222-2222-2222-2222-222222222222",
		"AA":     "33333333-3333-3333-3333-333333333333",
		"High A": "44444444-4444-4444-4444-444444444444",
	}

	fmt.Println("Caching teams...")
	teamLookup := make(map[string]string)
	tRows, _ := database.Query(context.Background(), "SELECT id, abbreviation, league_id FROM teams WHERE abbreviation IS NOT NULL")
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

	// Step 1: Collect Go DB players currently on waivers (to detect stale ones later)
	staleWaivers := make(map[int]string) // wp_id -> player UUID
	sRows, _ := database.Query(context.Background(),
		"SELECT id, wp_id FROM players WHERE fa_status = 'on waivers' AND wp_id IS NOT NULL")
	for sRows.Next() {
		var uuid string
		var wpID int
		sRows.Scan(&uuid, &wpID)
		staleWaivers[wpID] = uuid
	}
	sRows.Close()
	fmt.Printf("Found %d players currently on waivers in Go DB\n", len(staleWaivers))

	// Step 2: Sync from WP API — update confirmed waivers, track which are still active
	confirmedWPIDs := make(map[int]bool)
	synced := 0

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
			// Also sync fa_status for players that Go thinks are on waivers
			if _, isStale := staleWaivers[p.ID]; isStale && p.ACF.FaStatus != "on waivers" {
				// Player is on waivers in Go but NOT on WP — fix their status
				playerUUID := staleWaivers[p.ID]
				newStatus := p.ACF.FaStatus
				if newStatus == "" {
					newStatus = "available"
				}
				database.Exec(context.Background(), `
					UPDATE players SET fa_status = $1, waiver_end_time = NULL, waiving_team_id = NULL
					WHERE id = $2
				`, newStatus, playerUUID)
				fmt.Printf("Cleared stale waiver: WP_ID %d -> fa_status='%s'\n", p.ID, newStatus)
				confirmedWPIDs[p.ID] = true
				continue
			}

			if p.ACF.FaStatus != "on waivers" {
				continue
			}

			lID, ok := leagueMap[p.ACF.LeagueID]
			if !ok {
				continue
			}

			var playerUUID string
			err := database.QueryRow(context.Background(), "SELECT id FROM players WHERE wp_id = $1", p.ID).Scan(&playerUUID)
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

			_, err = database.Exec(context.Background(), `
				UPDATE players SET
					fa_status = 'on waivers',
					waiver_end_time = $1,
					waiving_team_id = $2
				WHERE id = $3
			`, endTime, waivingTeamUUID, playerUUID)

			if len(p.ACF.PendingWaiverClaims) > 0 {
				database.Exec(context.Background(), "DELETE FROM waiver_claims WHERE player_id = $1", playerUUID)
				for _, c := range p.ACF.PendingWaiverClaims {
					claimingTeamUUID := teamLookup[p.ACF.LeagueID+"_"+c.ClaimingTeamID]
					if claimingTeamUUID == "" {
						continue
					}

					ts, _ := time.Parse("2006-01-02 15:04:05", c.ClaimTimestamp)
					if ts.IsZero() {
						ts = time.Now()
					}

					database.Exec(context.Background(), `
						INSERT INTO waiver_claims (league_id, team_id, player_id, claim_priority, created_at)
						VALUES ($1, $2, $3, 0, $4)
					`, lID, claimingTeamUUID, playerUUID, ts)
				}
			}

			confirmedWPIDs[p.ID] = true
			synced++
			fmt.Printf("Synced waivers/claims for player WP_ID: %d\n", p.ID)
		}

		if page%50 == 0 {
			fmt.Printf("  ...page %d\n", page)
		}
		page++
	}

	fmt.Printf("\nWaiver sync complete: %d players synced, %d pages scanned\n", synced, page-1)

	// Step 3: Any Go DB waiver players whose wp_id wasn't encountered at all — clear them
	cleared := 0
	for wpID, uuid := range staleWaivers {
		if !confirmedWPIDs[wpID] {
			database.Exec(context.Background(), `
				UPDATE players SET fa_status = 'available', waiver_end_time = NULL, waiving_team_id = NULL
				WHERE id = $1
			`, uuid)
			fmt.Printf("Cleared orphan waiver (WP_ID not found): WP_ID %d\n", wpID)
			cleared++
		}
	}
	if cleared > 0 {
		fmt.Printf("Cleared %d orphan waiver entries\n", cleared)
	}
}
