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

type DeadCapPenalty struct {
	Year   any     `json:"penalty_year"`
	Amount any     `json:"penalty_amount"`
	TeamID string  `json:"dead_cap_team_id"`
	Type   string  `json:"penalty_type"`
}

type ACFData struct {
	FantasyTeamID    string           `json:"fantasy_team_id"`
	Position         string           `json:"position"`
	MLBTeam          string           `json:"mlb_team"`
	LeagueID         string           `json:"league_id"`
	Status40Man      string           `json:"status_40_man"`
	Status26Man      any              `json:"status_26_man"`
	StatusIL         string           `json:"status_il"`
	OptionYears      any              `json:"option_years_used"`
	Contract2026     string           `json:"contract_2026"`
	Contract2027     string           `json:"contract_2027"`
	Contract2028     string           `json:"contract_2028"`
	Contract2029     string           `json:"contract_2029"`
	Contract2030     string           `json:"contract_2030"`
	Contract2031     string           `json:"contract_2031"`
	Contract2032     string           `json:"contract_2032"`
	Contract2033     string           `json:"contract_2033"`
	Contract2034     string           `json:"contract_2034"`
	Contract2035     string           `json:"contract_2035"`
	Contract2036     string           `json:"contract_2036"`
	FaStatus         string           `json:"fa_status"`
	IsIFA            any              `json:"international_free_agent"`
	Rule5Year        any              `json:"rule_5_eligibility_year"`
	DeadCapPenalties []DeadCapPenalty `json:"dead_cap_penalties"`
}

func parseIntish(v any) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case string:
		if val == "" {
			return 0
		}
		n, _ := strconv.Atoi(val)
		return n
	}
	return 0
}

type WPPlayer struct {
	ID    int `json:"id"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	ACF ACFData `json:"acf"`
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

	fmt.Println("Pre-caching teams by abbreviation...")
	// Map "leagueUUID_abbreviation" -> team UUID
	teamLookup := make(map[string]string)

	tRows, _ := database.Query(context.Background(), "SELECT id, abbreviation, league_id FROM teams WHERE abbreviation IS NOT NULL")
	for tRows.Next() {
		var id, abbr, lID string
		tRows.Scan(&id, &abbr, &lID)
		teamLookup[lID+"_"+abbr] = id
	}
	tRows.Close()

	allLeagueUUIDs := []string{
		"11111111-1111-1111-1111-111111111111",
		"22222222-2222-2222-2222-222222222222",
		"33333333-3333-3333-3333-333333333333",
		"44444444-4444-4444-4444-444444444444",
	}

	fmt.Println("Starting Player Sync with Dead Cap...")

	page := 1
	totalImported := 0

	for {
		url := "https://frontofficedynastysports.com/wp-json/wp/v2/playerdata?per_page=100&page=" + strconv.Itoa(page)
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode >= 400 { break }

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var wpPlayers []WPPlayer
		json.Unmarshal(body, &wpPlayers)
		if len(wpPlayers) == 0 { break }

		for _, p := range wpPlayers {
			lID, exists := leagueMap[p.ACF.LeagueID]
			if !exists { continue }

			// Split name into first/last
			name := strings.TrimSpace(p.Title.Rendered)
			parts := strings.SplitN(name, " ", 2)
			firstName := parts[0]
			lastName := ""
			if len(parts) > 1 {
				lastName = parts[1]
			}

			isOn26Man := false
			switch v := p.ACF.Status26Man.(type) {
			case bool: isOn26Man = v
			case string: isOn26Man = (v == "1")
			}

			isIFA := false
			switch v := p.ACF.IsIFA.(type) {
			case bool: isIFA = v
			case string: isIFA = (v == "1")
			}

			optionYears := parseIntish(p.ACF.OptionYears)
			rule5Year := parseIntish(p.ACF.Rule5Year)

			// Resolve team_id from abbreviation + league
			var teamID *string
			if p.ACF.FantasyTeamID != "" {
				if tID, ok := teamLookup[lID+"_"+p.ACF.FantasyTeamID]; ok {
					teamID = &tID
				}
			}

			var playerUUID string
			err := database.QueryRow(context.Background(), `
				INSERT INTO players (
					wp_id, first_name, last_name, position, mlb_team, league_id, team_id,
					status_40_man, status_26_man, status_il, fa_status, option_years_used,
					contract_2026, contract_2027, contract_2028, contract_2029, contract_2030,
					contract_2031, contract_2032, contract_2033, contract_2034, contract_2035,
					contract_2036,
					raw_fantasy_team_id, is_international_free_agent, rule_5_eligibility_year
				)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
				ON CONFLICT (wp_id) DO UPDATE SET
					first_name = EXCLUDED.first_name,
					last_name = EXCLUDED.last_name,
					position = EXCLUDED.position,
					mlb_team = EXCLUDED.mlb_team,
					league_id = EXCLUDED.league_id,
					team_id = EXCLUDED.team_id,
					status_40_man = EXCLUDED.status_40_man,
					status_26_man = EXCLUDED.status_26_man,
					status_il = EXCLUDED.status_il,
					fa_status = EXCLUDED.fa_status,
					option_years_used = EXCLUDED.option_years_used,
					contract_2026 = EXCLUDED.contract_2026,
					contract_2027 = EXCLUDED.contract_2027,
					contract_2028 = EXCLUDED.contract_2028,
					contract_2029 = EXCLUDED.contract_2029,
					contract_2030 = EXCLUDED.contract_2030,
					contract_2031 = EXCLUDED.contract_2031,
					contract_2032 = EXCLUDED.contract_2032,
					contract_2033 = EXCLUDED.contract_2033,
					contract_2034 = EXCLUDED.contract_2034,
					contract_2035 = EXCLUDED.contract_2035,
					contract_2036 = EXCLUDED.contract_2036,
					raw_fantasy_team_id = EXCLUDED.raw_fantasy_team_id,
					is_international_free_agent = EXCLUDED.is_international_free_agent,
					rule_5_eligibility_year = EXCLUDED.rule_5_eligibility_year
				RETURNING id
			`,
				p.ID, firstName, lastName, p.ACF.Position, p.ACF.MLBTeam, lID, teamID,
				p.ACF.Status40Man == "X", isOn26Man, p.ACF.StatusIL, p.ACF.FaStatus, optionYears,
				p.ACF.Contract2026, p.ACF.Contract2027, p.ACF.Contract2028, p.ACF.Contract2029, p.ACF.Contract2030,
				p.ACF.Contract2031, p.ACF.Contract2032, p.ACF.Contract2033, p.ACF.Contract2034, p.ACF.Contract2035,
				p.ACF.Contract2036,
				p.ACF.FantasyTeamID, isIFA, rule5Year,
			).Scan(&playerUUID)

			if err != nil {
				fmt.Printf("❌ Error saving player %s (WP ID %d): %v\n", name, p.ID, err)
			}

			if playerUUID != "" && len(p.ACF.DeadCapPenalties) > 0 {
				database.Exec(context.Background(), "DELETE FROM dead_cap_penalties WHERE player_id = $1", playerUUID)
				for _, dc := range p.ACF.DeadCapPenalties {
					// Try player's league first, then all leagues
					tUUID := ""
					if id, ok := teamLookup[lID+"_"+dc.TeamID]; ok {
						tUUID = id
					} else {
						for _, lid := range allLeagueUUIDs {
							if id, ok := teamLookup[lid+"_"+dc.TeamID]; ok {
								tUUID = id
								break
							}
						}
					}
					if tUUID == "" { continue }

					var yr int
					switch v := dc.Year.(type) {
					case float64: yr = int(v)
					case string: yr, _ = strconv.Atoi(v)
					}

					var amt float64
					switch v := dc.Amount.(type) {
					case float64: amt = v
					case string: amt, _ = strconv.ParseFloat(v, 64)
					}

					if yr >= 2026 {
						database.Exec(context.Background(), `
							INSERT INTO dead_cap_penalties (team_id, player_id, amount, year, note)
							VALUES ($1, $2, $3, $4, $5)
						`, tUUID, playerUUID, amt, yr, dc.Type)
					}
				}
			}
			totalImported++
		}
		page++
		fmt.Printf("Page %d synced...\n", page-1)
	}
	fmt.Printf("Done. %d players processed.\n", totalImported)
}