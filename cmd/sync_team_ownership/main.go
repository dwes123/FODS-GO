package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
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
	Email    string    `json:"email"`
	ACF      WPUserACF `json:"acf"`
}

func main() {
	dbUrl := "postgres://admin:password123@localhost:5433/fantasy_db"
	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	wpUser := "djwes487@gmail.com"
	wpPass := "ab4H TPEh vyrc 9lOL T91Z Zt5L"
	auth := base64.StdEncoding.EncodeToString([]byte(wpUser + ":" + wpPass))

	leagueMap := map[string]string{
		"MLB":    "11111111-1111-1111-1111-111111111111",
		"AAA":    "22222222-2222-2222-2222-222222222222",
		"AA":     "33333333-3333-3333-3333-333333333333",
		"High A": "44444444-4444-4444-4444-444444444444",
	}

	fmt.Println("Starting Enhanced Team Ownership Sync...")

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
			// Match by WP ID OR Username
			db.QueryRow(context.Background(), 
				"SELECT id FROM users WHERE wp_id = $1 OR username = $2", 
				wpu.ID, wpu.Username).Scan(&goUserID)

			if goUserID == "" {
				continue
			}

			for _, t := range wpu.ACF.ManagedTeams {
				lUUID, ok := leagueMap[t.LeagueID]
				if !ok {
					continue
				}

				teamIdentifier := strings.TrimSpace(t.FantasyTeamID)
				
				// Try matching by abbreviation first, then by name
				result, err := db.Exec(context.Background(), `
					UPDATE teams 
					SET user_id = $1, owner_name = $2 
					WHERE league_id = $3 AND (abbreviation = $4 OR name = $4)
				`, goUserID, wpu.Name, lUUID, teamIdentifier)

				if err == nil && result.RowsAffected() > 0 {
					fmt.Printf("Linked %s to %s (%s)\n", teamIdentifier, wpu.Username, t.LeagueID)
					totalLinked++
				}
			}
		}
		page++
	}

	fmt.Printf("\nDone! Linked %d teams to their correct managers.\n", totalLinked)
}
