package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
)

type WPUserOwnedTeam struct {
	LeagueID      string `json:"league_id"`
	FantasyTeamID string `json:"fantasy_team_id"` 
}

type WPUserACF struct {
	ManagedTeams []WPUserOwnedTeam `json:"managed_teams"`
}

type WPUserWithACF struct {
	ID       int       `json:"id"`
	Username string    `json:"username"`
	Name     string    `json:"name"`
	ACF      WPUserACF `json:"acf"`
}

func main() {
	database := db.InitDB()
	defer database.Close()

	// Hardcoded credentials for this specific task
	wpUser := "djwes487@gmail.com"
	wpPass := "ab4H TPEh vyrc 9lOL T91Z Zt5L"
	auth := base64.StdEncoding.EncodeToString([]byte(wpUser + ":" + wpPass))

	leagueMap := map[string]string{
		"MLB":    "11111111-1111-1111-1111-111111111111",
		"AAA":    "22222222-2222-2222-2222-222222222222",
		"AA":     "33333333-3333-3333-3333-333333333333",
		"High A": "44444444-4444-4444-4444-444444444444",
	}

	fmt.Println("🚀 Starting Precise Team Ownership Sync...")

	page := 1
	totalLinked := 0

	for {
		url := "https://frontofficedynastysports.com/wp-json/wp/v2/users?context=edit&per_page=100&page=" + strconv.Itoa(page)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Basic "+auth)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode >= 400 {
			break
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var wpUsers []WPUserWithACF
		json.Unmarshal(body, &wpUsers)

		if len(wpUsers) == 0 {
			break
		}

		for _, wpu := range wpUsers {
			var goUserID string
			// 1. Find User
			database.QueryRow(context.Background(), 
				"SELECT id FROM users WHERE wp_id = $1 OR username = $2", 
				wpu.ID, wpu.Username).Scan(&goUserID)

			if goUserID == "" {
				continue
			}

			// 2. Loop through their assigned teams from ACF
			for _, t := range wpu.ACF.ManagedTeams {
				lUUID, ok := leagueMap[t.LeagueID]
				if !ok { continue }

				teamIdentifier := strings.TrimSpace(t.FantasyTeamID)
				if teamIdentifier == "" { continue }

				// 3. Find the Team ID
				var teamID string

				// Strategy A: Direct abbreviation match (most reliable)
				database.QueryRow(context.Background(), `
					SELECT id FROM teams
					WHERE abbreviation = $1 AND league_id = $2
					LIMIT 1
				`, teamIdentifier, lUUID).Scan(&teamID)

				// Strategy B: Fuzzy name match
				if teamID == "" {
					database.QueryRow(context.Background(), `
						SELECT id FROM teams
						WHERE league_id = $1 AND (name ILIKE '%' || $2 || '%')
						LIMIT 1
					`, lUUID, teamIdentifier).Scan(&teamID)
				}

				// Strategy C: Player-based lookup (works if players already synced)
				if teamID == "" {
					database.QueryRow(context.Background(), `
						SELECT team_id FROM players
						WHERE raw_fantasy_team_id = $1 AND league_id = $2 AND team_id IS NOT NULL
						LIMIT 1
					`, teamIdentifier, lUUID).Scan(&teamID)
				}

				if teamID != "" {
					// 4. Insert into team_owners
					_, err := database.Exec(context.Background(), `
						INSERT INTO team_owners (team_id, user_id)
						VALUES ($1, $2)
						ON CONFLICT DO NOTHING
					`, teamID, goUserID)

					if err == nil {
						fmt.Printf("✅ Linked %s to %s (Team: %s)\n", wpu.Username, teamIdentifier, teamID)
						
						// Update legacy columns too just in case
						database.Exec(context.Background(), "UPDATE teams SET owner_name = $1 WHERE id = $2", wpu.Name, teamID)
						
						totalLinked++
					}
				}
			}
		}
		page++
		fmt.Printf("Processed Page %d...\n", page-1)
	}

	fmt.Printf("\n🏆 DONE! Linked %d teams to owners.\n", totalLinked)
}
