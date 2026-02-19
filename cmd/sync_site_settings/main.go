package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
)

const bridgeURL = "https://frontofficedynastysports.com/wp-json/fod-bridge/v1/site-settings?key=fod-migrate-2026"

type TeamBalance struct {
	TeamID  string `json:"team_id"`
	Balance any    `json:"balance"`
}

type LuxuryTaxRow struct {
	Year  any `json:"year"`
	Limit any `json:"limit"`
}

type SiteSettings struct {
	ISBP_MLB    []TeamBalance `json:"isbp_mlb"`
	ISBP_AAA    []TeamBalance `json:"isbp_aaa"`
	ISBP_AA     []TeamBalance `json:"isbp_aa"`
	ISBP_HighA  []TeamBalance `json:"isbp_high_a"`
	MILB_MLB    []TeamBalance `json:"milb_mlb"`
	MILB_AAA    []TeamBalance `json:"milb_aaa"`
	MILB_AA     []TeamBalance `json:"milb_aa"`
	MILB_HighA  []TeamBalance `json:"milb_high_a"`
	LuxuryTax   []LuxuryTaxRow `json:"luxury_tax_thresholds"`
}

var leagueMap = map[string]string{
	"MLB":    "11111111-1111-1111-1111-111111111111",
	"AAA":    "22222222-2222-2222-2222-222222222222",
	"AA":     "33333333-3333-3333-3333-333333333333",
	"High A": "44444444-4444-4444-4444-444444444444",
}

func parseNumber(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		var n float64
		fmt.Sscanf(val, "%f", &n)
		return n
	case int:
		return float64(val)
	default:
		return 0
	}
}

func main() {
	database := db.InitDB()
	defer database.Close()

	// Build team abbreviation -> UUID lookup
	teamLookup := make(map[string]string) // "leagueUUID_abbr" -> teamUUID
	tRows, _ := database.Query(context.Background(),
		"SELECT id, abbreviation, league_id FROM teams WHERE abbreviation IS NOT NULL")
	for tRows.Next() {
		var id, abbr, lID string
		tRows.Scan(&id, &abbr, &lID)
		teamLookup[lID+"_"+abbr] = id
	}
	tRows.Close()
	fmt.Printf("Cached %d teams\n", len(teamLookup))

	// Fetch site settings from bridge
	fmt.Println("Fetching site settings from WordPress bridge...")
	resp, err := http.Get(bridgeURL)
	if err != nil {
		fmt.Printf("ERROR fetching bridge: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("ERROR: HTTP %d — %s\n", resp.StatusCode, string(body))
		return
	}

	body, _ := io.ReadAll(resp.Body)
	var settings SiteSettings
	if err := json.Unmarshal(body, &settings); err != nil {
		fmt.Printf("ERROR parsing JSON: %v\n", err)
		return
	}

	// Sync ISBP balances
	fmt.Println("\n--- Syncing ISBP Balances ---")
	isbpMap := map[string][]TeamBalance{
		leagueMap["MLB"]:    settings.ISBP_MLB,
		leagueMap["AAA"]:    settings.ISBP_AAA,
		leagueMap["AA"]:     settings.ISBP_AA,
		leagueMap["High A"]: settings.ISBP_HighA,
	}
	isbpCount := 0
	for leagueUUID, balances := range isbpMap {
		for _, tb := range balances {
			teamUUID := teamLookup[leagueUUID+"_"+tb.TeamID]
			if teamUUID == "" {
				continue
			}
			bal := parseNumber(tb.Balance)
			_, err := database.Exec(context.Background(),
				"UPDATE teams SET isbp_balance = $1 WHERE id = $2",
				bal, teamUUID)
			if err != nil {
				fmt.Printf("  ERROR updating ISBP for %s: %v\n", tb.TeamID, err)
			} else {
				isbpCount++
			}
		}
	}
	fmt.Printf("Updated %d ISBP balances\n", isbpCount)

	// Sync MILB balances (add column if needed)
	fmt.Println("\n--- Syncing MILB Balances ---")
	database.Exec(context.Background(),
		"ALTER TABLE teams ADD COLUMN IF NOT EXISTS milb_balance NUMERIC(12,2) DEFAULT 0.00")

	milbMap := map[string][]TeamBalance{
		leagueMap["MLB"]:    settings.MILB_MLB,
		leagueMap["AAA"]:    settings.MILB_AAA,
		leagueMap["AA"]:     settings.MILB_AA,
		leagueMap["High A"]: settings.MILB_HighA,
	}
	milbCount := 0
	for leagueUUID, balances := range milbMap {
		for _, tb := range balances {
			teamUUID := teamLookup[leagueUUID+"_"+tb.TeamID]
			if teamUUID == "" {
				continue
			}
			bal := parseNumber(tb.Balance)
			_, err := database.Exec(context.Background(),
				"UPDATE teams SET milb_balance = $1 WHERE id = $2",
				bal, teamUUID)
			if err != nil {
				fmt.Printf("  ERROR updating MILB for %s: %v\n", tb.TeamID, err)
			} else {
				milbCount++
			}
		}
	}
	fmt.Printf("Updated %d MILB balances\n", milbCount)

	// Sync Luxury Tax Thresholds
	fmt.Println("\n--- Syncing Luxury Tax Thresholds ---")
	taxCount := 0
	allLeagueUUIDs := []string{
		leagueMap["MLB"], leagueMap["AAA"], leagueMap["AA"], leagueMap["High A"],
	}
	for _, row := range settings.LuxuryTax {
		year := int(parseNumber(row.Year))
		threshold := parseNumber(row.Limit)
		if year == 0 || threshold == 0 {
			continue
		}
		for _, lID := range allLeagueUUIDs {
			_, err := database.Exec(context.Background(), `
				INSERT INTO league_settings (league_id, year, luxury_tax_limit)
				VALUES ($1, $2, $3)
				ON CONFLICT (league_id, year) DO UPDATE SET luxury_tax_limit = $3
			`, lID, year, threshold)
			if err != nil {
				fmt.Printf("  ERROR setting tax %d/%s: %v\n", year, lID[:8], err)
			} else {
				taxCount++
			}
		}
	}
	fmt.Printf("Updated %d luxury tax entries\n", taxCount)

	fmt.Println("\nSite settings sync complete!")
}
