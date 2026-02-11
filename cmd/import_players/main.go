package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
)

// 1. Define the Structures
type ACFData struct {
	FantasyTeamID string `json:"fantasy_team_id"`
	Position      string `json:"position"`
	MLBTeam       string `json:"mlb_team"`
	League        string `json:"league"`
	LeagueID      string `json:"league_id"`
}

type WPTitle struct {
	Rendered string `json:"rendered"`
}

type WPPlayer struct {
	ID    int     `json:"id"`
	Title WPTitle `json:"title"`
	ACF   ACFData `json:"acf"`
}

func main() {
	database := db.InitDB()
	defer database.Close()

	fmt.Println("✅ Connected to Moneyball Database")
	fmt.Println("🚀 Starting AA Import (Looping all pages)...")

	page := 1
	totalImported := 0
	totalSkipped := 0

	for {
		url := "https://frontofficedynastysports.com/wp-json/wp/v2/playerdata?per_page=100&page=" + strconv.Itoa(page)
		fmt.Printf("📡 Fetching Page %d... ", page)

		resp, err := http.Get(url)
		if err != nil {
			log.Printf("❌ Error fetching page %d: %v\n", page, err)
			break
		}

		if resp.StatusCode >= 400 {
			fmt.Println("🏁 End of list reached.")
			resp.Body.Close()
			break
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}

		var wpPlayers []WPPlayer
		if err := json.Unmarshal(body, &wpPlayers); err != nil {
			fmt.Println("🏁 End of list reached (JSON parse).")
			break
		}

		if len(wpPlayers) == 0 {
			fmt.Println("🏁 No more players found.")
			break
		}

		fmt.Printf("Found %d players. Importing...\n", len(wpPlayers))

		for _, wpPlayer := range wpPlayers {

			// ============================================================
			// 🛑 THE FILTER: TARGET "AA"
			// ============================================================
			isTarget := false
			if wpPlayer.ACF.League == "AA" {
				isTarget = true
			}
			if wpPlayer.ACF.LeagueID == "AA" {
				isTarget = true
			}

			if !isTarget {
				totalSkipped++
				continue
			}

			// ============================================================
			// 🚀 IMPORT
			// ============================================================
			_, err := database.Exec(context.Background(), `
				INSERT INTO players (
					wp_id, 
					first_name, 
					last_name, 
					position, 
					mlb_team, 
					league_id, 
					raw_fantasy_team_id
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (wp_id) DO UPDATE SET
					mlb_team = EXCLUDED.mlb_team,
					raw_fantasy_team_id = EXCLUDED.raw_fantasy_team_id,
					position = EXCLUDED.position,
                    league_id = EXCLUDED.league_id;
			`,
				wpPlayer.ID,
				wpPlayer.Title.Rendered,
				"",
				wpPlayer.ACF.Position,
				wpPlayer.ACF.MLBTeam,
				"33333333-3333-3333-3333-333333333333", // <--- NEW UUID FOR AA
				wpPlayer.ACF.FantasyTeamID,
			)

			if err != nil {
				fmt.Printf("❌ Failed: %s (%v)\n", wpPlayer.Title.Rendered, err)
			} else {
				totalImported++
			}
		}
		page++
	}

	fmt.Println("------------------------------------------------")
	fmt.Printf("✅ FINAL REPORT (AA): Imported: %d | Skipped: %d\n", totalImported, totalSkipped)
}
