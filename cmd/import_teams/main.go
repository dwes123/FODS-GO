package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

var FullNameMap = map[string]string{
	"ARI": "Arizona Diamondbacks", "ARZ": "Arizona Diamondbacks",
	"ATL": "Atlanta Braves",
	"BAL": "Baltimore Orioles",
	"BOS": "Boston Red Sox",
	"CHC": "Chicago Cubs", "CHI": "Chicago Cubs",
	"CWS": "Chicago White Sox", "CHW": "Chicago White Sox",
	"CIN": "Cincinnati Reds",
	"CLE": "Cleveland Guardians",
	"COL": "Colorado Rockies",
	"DET": "Detroit Tigers",
	"HOU": "Houston Astros",
	"KC":  "Kansas City Royals",
	"LAA": "Los Angeles Angels", "ANA": "Los Angeles Angels",
	"LAD": "Los Angeles Dodgers",
	"MIA": "Miami Marlins", "FLO": "Miami Marlins",
	"MIL": "Milwaukee Brewers",
	"MIN": "Minnesota Twins",
	"NYM": "New York Mets",
	"NYY": "New York Yankees",
	"OAK": "Oakland Athletics", "ATH": "Oakland Athletics",
	"PHI": "Philadelphia Phillies",
	"PIT": "Pittsburgh Pirates",
	"SD":  "San Diego Padres",
	"SF":  "San Francisco Giants",
	"SEA": "Seattle Mariners",
	"STL": "St. Louis Cardinals",
	"TB":  "Tampa Bay Rays",
	"TEX": "Texas Rangers",
	"TOR": "Toronto Blue Jays",
	"WSH": "Washington Nationals", "WAS": "Washington Nationals",
}

// Struct to catch the nested team data
type ManagedTeam struct {
	TeamID      string `json:"fantasy_team_id"` // "HOU"
	LeagueID    string `json:"league_id"`       // "MLB"
	ISBPBalance any    `json:"isbp_balance"`
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
				leagueUUID = LeagueMap["AAA"]
			}

			// Determine Name
			baseName := mt.TeamID
			if full, ok := FullNameMap[mt.TeamID]; ok {
				baseName = full
			}
			
			// Only append (League) if it's NOT a full MLB name (optional preference)
			// For now, let's keep consistent format: "Name (League)"
			// But for MLB, usually we just want "New York Yankees"
			finalName := baseName
			if mt.LeagueID != "MLB" {
				finalName = fmt.Sprintf("%s (%s)", baseName, mt.LeagueID)
			} else {
				// For MLB, we want just the name "New York Yankees", not "New York Yankees (MLB)"
				finalName = baseName
			}

			// Generate Slug (e.g., "houston-astros-mlb")
			slug := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(finalName, " ", "-"), "(", ""))
			slug = strings.ReplaceAll(slug, ")", "")

			// Parse ISBP
			var isbp float64
			switch v := mt.ISBPBalance.(type) {
			case float64: isbp = v
			case string: 
				clean := strings.ReplaceAll(strings.ReplaceAll(v, "$", ""), ",", "")
				isbp, _ = strconv.ParseFloat(clean, 64)
			}

			// Insert/Update the Team
			_, err := pool.Exec(context.Background(), `
				INSERT INTO teams (name, slug, owner_name, league_id, wp_id, isbp_balance)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (league_id, slug) DO UPDATE SET 
					isbp_balance = EXCLUDED.isbp_balance,
					wp_id = EXCLUDED.wp_id
			`, finalName, slug, ownerName, leagueUUID, u.ID, isbp)

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
