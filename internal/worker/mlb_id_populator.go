package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"
)

type mlbIDResult struct {
	Matched   int
	Skipped   int
	Ambiguous int
	NotFound  int
	Errors    int
}

type unmatchedPlayer struct {
	FirstName string
	LastName  string
}

// ProcessMLBIDPopulation searches the MLB API to fill in mlb_id for rostered players missing it.
// Exported so it can be triggered from the admin handler.
func ProcessMLBIDPopulation(ctx context.Context, db *pgxpool.Pool) {
	dbCtx := context.Background()

	// Get unique (first_name, last_name) combos for rostered players without mlb_id
	rows, err := db.Query(dbCtx, `
		SELECT DISTINCT first_name, last_name
		FROM players
		WHERE (mlb_id IS NULL OR mlb_id = 0)
		AND team_id IS NOT NULL
		AND team_id <> '00000000-0000-0000-0000-000000000000'
		ORDER BY last_name, first_name
	`)
	if err != nil {
		fmt.Printf("ERROR [MLBIDPopulator]: query players: %v\n", err)
		return
	}

	var players []unmatchedPlayer
	for rows.Next() {
		var p unmatchedPlayer
		if err := rows.Scan(&p.FirstName, &p.LastName); err != nil {
			continue
		}
		players = append(players, p)
	}
	rows.Close()

	fmt.Printf("MLB ID Populator: searching for %d unique rostered players\n", len(players))

	result := mlbIDResult{}

	for i, p := range players {
		select {
		case <-ctx.Done():
			fmt.Println("MLB ID Populator: cancelled")
			return
		default:
		}

		mlbID, status := searchMLBPlayer(p.FirstName, p.LastName)

		switch status {
		case "matched":
			_, err := db.Exec(dbCtx,
				`UPDATE players SET mlb_id = $1 WHERE first_name = $2 AND last_name = $3 AND (mlb_id IS NULL OR mlb_id = 0)`,
				mlbID, p.FirstName, p.LastName)
			if err != nil {
				fmt.Printf("ERROR [MLBIDPopulator]: update %s %s: %v\n", p.FirstName, p.LastName, err)
				result.Errors++
			} else {
				result.Matched++
			}
		case "ambiguous":
			result.Ambiguous++
		case "not_found":
			result.NotFound++
		case "error":
			result.Errors++
		}

		// Progress log every 100 players
		if (i+1)%100 == 0 {
			fmt.Printf("MLB ID Populator: %d/%d processed (matched=%d, ambiguous=%d, not_found=%d)\n",
				i+1, len(players), result.Matched, result.Ambiguous, result.NotFound)
		}

		// Rate limit: 1 request per second
		time.Sleep(1 * time.Second)
	}

	fmt.Printf("MLB ID Populator: DONE — matched=%d, ambiguous=%d, not_found=%d, errors=%d, total=%d\n",
		result.Matched, result.Ambiguous, result.NotFound, result.Errors, len(players))
}

// searchMLBPlayer queries the MLB Stats API people search for a player by name.
// Returns (mlbID, status) where status is "matched", "ambiguous", "not_found", or "error".
func searchMLBPlayer(firstName, lastName string) (int, string) {
	searchName := fmt.Sprintf("%s %s", firstName, lastName)
	apiURL := fmt.Sprintf("https://statsapi.mlb.com/api/v1/people/search?names=%s&hydrate=currentTeam",
		url.QueryEscape(searchName))

	resp, err := http.Get(apiURL)
	if err != nil {
		fmt.Printf("ERROR [MLBIDPopulator]: API call for %s: %v\n", searchName, err)
		return 0, "error"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "error"
	}

	var apiResp struct {
		People []struct {
			ID        int    `json:"id"`
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
			FullName  string `json:"fullFMLName"`
		} `json:"people"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return 0, "error"
	}

	if len(apiResp.People) == 0 {
		return 0, "not_found"
	}

	// Filter to exact name matches (case-insensitive, accent-insensitive)
	var exactMatches []int
	for _, person := range apiResp.People {
		if strings.EqualFold(stripAccents(person.FirstName), stripAccents(firstName)) &&
			strings.EqualFold(stripAccents(person.LastName), stripAccents(lastName)) {
			exactMatches = append(exactMatches, person.ID)
		}
	}

	if len(exactMatches) == 0 {
		return 0, "not_found"
	}

	if len(exactMatches) == 1 {
		return exactMatches[0], "matched"
	}

	// Multiple exact matches — ambiguous
	return 0, "ambiguous"
}

// stripAccents removes Unicode diacritical marks (é→e, í→i, ñ→n, etc.)
func stripAccents(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
