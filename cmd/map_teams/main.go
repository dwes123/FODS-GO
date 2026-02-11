package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

const UsersURL = "https://frontofficedynastysports.com/wp-json/wp/v2/users?per_page=100"

var LeagueMap = map[string]string{
	"MLB":    "11111111-1111-1111-1111-111111111111",
	"AAA":    "22222222-2222-2222-2222-222222222222",
	"AA":     "33333333-3333-3333-3333-333333333333",
	"High A": "44444444-4444-4444-4444-444444444444",
}

type ManagedTeam struct {
	TeamID   string `json:"fantasy_team_id"`
	LeagueID string `json:"league_id"`
}

type WPUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	ACF  struct {
		ManagedTeams []ManagedTeam `json:"managed_teams"`
	} `json:"acf"`
}

func main() {
	dbUrl := "postgres://admin:password123@localhost:5433/fantasy_db"
	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	fmt.Println("Fetching Users to map Abbreviations...")
	resp, err := http.Get(UsersURL)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var users []WPUser
	json.Unmarshal(body, &users)

	mapped := 0
	for _, u := range users {
		for _, mt := range u.ACF.ManagedTeams {
			leagueUUID := LeagueMap[mt.LeagueID]
			if leagueUUID == "" { continue }

			_, err := db.Exec(context.Background(), `
				UPDATE teams 
				SET abbreviation = $1 
				WHERE wp_id = $2 AND league_id = $3
			`, mt.TeamID, u.ID, leagueUUID)

			if err == nil {
				mapped++
				fmt.Printf("Mapped %s (%s) -> User %d\n", mt.TeamID, mt.LeagueID, u.ID)
			}
		}
	}
	
	// Manual fixes
	db.Exec(context.Background(), "UPDATE teams SET abbreviation = 'COL' WHERE name = 'Colorado Rockies'")

	fmt.Printf("Done. Mapped %d abbreviations.\n", mapped)
}