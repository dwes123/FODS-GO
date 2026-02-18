package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
)

type WPTransaction struct {
	ID    int    `json:"id"`
	Date  string `json:"date"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
}

type BidHistoryEntry struct {
	TeamID    string  `json:"history_team_id"`
	Amount    float64 `json:"history_bid_amount"`
	Years     int     `json:"history_bid_years"`
	AAV       float64 `json:"history_bid_aav"`
	Timestamp string  `json:"history_timestamp"`
}

func main() {
	database := db.InitDB()
	defer database.Close()
	ctx := context.Background()

	// Build team lookup: abbreviation -> team UUID (per league)
	teamLookup := make(map[string]string) // "MLB_NYY" -> UUID
	leagueMap := map[string]string{
		"11111111-1111-1111-1111-111111111111": "MLB",
		"22222222-2222-2222-2222-222222222222": "AAA",
		"33333333-3333-3333-3333-333333333333": "AA",
		"44444444-4444-4444-4444-444444444444": "High A",
	}

	tRows, _ := database.Query(ctx, "SELECT id, abbreviation, league_id FROM teams WHERE abbreviation IS NOT NULL")
	for tRows.Next() {
		var id, abbr, lID string
		tRows.Scan(&id, &abbr, &lID)
		lKey := leagueMap[lID]
		if lKey != "" {
			teamLookup[lKey+"_"+abbr] = id
		}
	}
	tRows.Close()

	// Alias map for abbreviation mismatches between WordPress and DB
	abbrAliases := map[string]string{"CHW": "CWS", "CWS": "CHW"}
	for alias, canonical := range abbrAliases {
		for _, lKey := range []string{"MLB", "AAA", "AA", "High A"} {
			if id, ok := teamLookup[lKey+"_"+canonical]; ok {
				teamLookup[lKey+"_"+alias] = id
			}
		}
	}

	// Also build name-based lookup for team matching
	teamNameLookup := make(map[string]string) // "MLB_New York Yankees" -> UUID
	tnRows, _ := database.Query(ctx, "SELECT id, name, league_id FROM teams")
	for tnRows.Next() {
		var id, name, lID string
		tnRows.Scan(&id, &name, &lID)
		lKey := leagueMap[lID]
		if lKey != "" {
			teamNameLookup[lKey+"_"+name] = id
		}
	}
	tnRows.Close()

	// Build player lookup: lowercase name + league -> player UUID
	type playerKey struct {
		name     string
		leagueID string
	}
	playerLookup := make(map[playerKey]string)
	pRows, _ := database.Query(ctx, "SELECT id, first_name || ' ' || last_name, league_id FROM players")
	for pRows.Next() {
		var id, name, lID string
		pRows.Scan(&id, &name, &lID)
		playerLookup[playerKey{strings.ToLower(strings.TrimSpace(name)), lID}] = id
	}
	pRows.Close()

	// Pattern: "Free Agent Bid: The NYY have placed a bid on Josh Naylor (5 years, $50,000,000 AAV)."
	bidPattern := regexp.MustCompile(`Free Agent Bid: The (.+?) have placed a bid on (.+?) \((\d+) years?, \$([0-9,]+) AAV\)`)

	// Collect bids per player: playerUUID -> []BidHistoryEntry
	playerBids := make(map[string][]BidHistoryEntry)
	totalParsed := 0
	totalSkipped := 0

	page := 1
	for {
		url := fmt.Sprintf("https://frontofficedynastysports.com/wp-json/wp/v2/transaction?per_page=100&page=%d&search=Free+Agent+Bid", page)
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode >= 400 {
			break
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var txns []WPTransaction
		json.Unmarshal(body, &txns)
		if len(txns) == 0 {
			break
		}

		for _, tx := range txns {
			matches := bidPattern.FindStringSubmatch(tx.Title.Rendered)
			if matches == nil {
				totalSkipped++
				continue
			}

			teamAbbr := matches[1]
			playerName := strings.TrimSpace(matches[2])
			years, _ := strconv.Atoi(matches[3])
			aavStr := strings.ReplaceAll(matches[4], ",", "")
			aav, _ := strconv.ParseFloat(aavStr, 64)
			bidDate := tx.Date

			// Find team UUID - try all leagues
			var teamUUID string
			for _, lKey := range []string{"MLB", "AAA", "AA", "High A"} {
				if id, ok := teamLookup[lKey+"_"+teamAbbr]; ok {
					teamUUID = id
					break
				}
				// Also try name match
				if id, ok := teamNameLookup[lKey+"_"+teamAbbr]; ok {
					teamUUID = id
					break
				}
			}

			if teamUUID == "" {
				fmt.Printf("  SKIP (no team): %s -> %q\n", teamAbbr, tx.Title.Rendered)
				totalSkipped++
				continue
			}

			// Find player UUID - look up team's league first
			var teamLeagueID string
			database.QueryRow(ctx, "SELECT league_id FROM teams WHERE id = $1", teamUUID).Scan(&teamLeagueID)

			playerUUID := playerLookup[playerKey{strings.ToLower(playerName), teamLeagueID}]
			if playerUUID == "" {
				// Try all leagues
				for lID := range leagueMap {
					if id := playerLookup[playerKey{strings.ToLower(playerName), lID}]; id != "" {
						playerUUID = id
						break
					}
				}
			}

			if playerUUID == "" {
				fmt.Printf("  SKIP (no player): %q in league %s\n", playerName, teamLeagueID)
				totalSkipped++
				continue
			}

			entry := BidHistoryEntry{
				TeamID:    teamUUID,
				Amount:    aav * float64(years),
				Years:     years,
				AAV:       aav,
				Timestamp: bidDate,
			}
			playerBids[playerUUID] = append(playerBids[playerUUID], entry)
			totalParsed++
		}

		fmt.Printf("Page %d processed (%d transactions)\n", page, len(txns))
		page++
	}

	// Write to DB
	updated := 0
	for playerUUID, entries := range playerBids {
		jsonData, err := json.Marshal(entries)
		if err != nil {
			continue
		}
		_, err = database.Exec(ctx,
			"UPDATE players SET bid_history = $1 WHERE id = $2",
			jsonData, playerUUID)
		if err != nil {
			fmt.Printf("  DB error for player %s: %v\n", playerUUID, err)
		} else {
			updated++
		}
	}

	fmt.Printf("\nDone. %d bids parsed, %d skipped, %d players updated.\n", totalParsed, totalSkipped, updated)
}
