package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
)

const UsersURL = "https://frontofficedynastysports.com/wp-json/wp/v2/users?per_page=100"

// Map specific League Names to your Postgres UUIDs
var LeagueMap = map[string]string{
	"MLB":    "11111111-1111-1111-1111-111111111111",
	"AAA":    "22222222-2222-2222-2222-222222222222",
	"AA":     "33333333-3333-3333-3333-333333333333",
	"High A": "44444444-4444-4444-4444-444444444444",
}

// Struct to catch the nested team data
type ManagedTeam struct {
	TeamID   string `json:"fantasy_team_id"` // "HOU"
	LeagueID string `json:"league_id"`       // "MLB"
}

type WPUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"` // "Astros - Gino"
	ACF  struct {
		// It comes as a list of teams
		ManagedTeams []ManagedTeam `json:"managed_teams"`
	} `json:"acf"`
}

func main() {
	pool := db.InitDB()
	defer pool.Close()

	fmt.Println("🚀 Unpacking Teams from Managers...")

	// 1. Fetch Users
	resp, err := http.Get(UsersURL)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var users []WPUser
	if err := json.Unmarshal(body, &users); err != nil {
		panic(err)
	}

	totalTeams := 0

	// 2. Loop through Users (Managers)
	for _, u := range users {
		ownerName := u.Name
		// Clean name: "Astros - Gino" -> "Gino"
		if strings.Contains(u.Name, " - ") {
			parts := strings.Split(u.Name, " - ")
			if len(parts) > 1 {
				ownerName = strings.TrimSpace(parts[1])
			}
		}

		// 3. Loop through THEIR Teams
		if len(u.ACF.ManagedTeams) == 0 {
			// Fallback: If list is empty, assume they own just one team named like their user
			// (Use AAA as default if not specified)
			// This handles the "48 users" who might not have filled out the ACF field yet
			continue
		}

		for _, mt := range u.ACF.ManagedTeams {
			// Find the UUID for "MLB", "AAA", etc.
			leagueUUID, exists := LeagueMap[mt.LeagueID]
			if !exists {
				// Default to AAA if unknown
				leagueUUID = LeagueMap["AAA"]
			}

			// Team Name (e.g., "Houston Astros (MLB)")
			// We construct a name since "HOU" is too short
			finalName := fmt.Sprintf("%s (%s)", mt.TeamID, mt.LeagueID)

			// Generate Slug (e.g., "houston-astros-mlb")
			slug := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(finalName, " ", "-"), "(", ""))
			slug = strings.ReplaceAll(slug, ")", "")

			// Insert the Team
			_, err := pool.Exec(context.Background(), `
				INSERT INTO teams (name, slug, owner_name, league_id, wp_id)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (id) DO NOTHING
			`, finalName, slug, ownerName, leagueUUID, u.ID)

			if err != nil {
				fmt.Printf("❌ Error saving %s: %v\n", finalName, err)
			} else {
				fmt.Printf("   ✨ Created: %-20s | Owner: %s | League: %s\n", finalName, ownerName, mt.LeagueID)
				totalTeams++
			}
		}
	}

	fmt.Printf("\n🎉 DONE! Unpacked %d teams from %d managers.\n", totalTeams, len(users))
}
