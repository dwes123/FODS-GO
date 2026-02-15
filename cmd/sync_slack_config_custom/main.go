package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
)

type SlackConfigRow struct {
	LeagueID                 string `json:"league_id"`
	BotToken                 string `json:"bot_token"`
	ChannelID                string `json:"channel_id"`
	StatAlertsChannelID      string `json:"stat_alerts_channel_id"`
}

func main() {
	database := db.InitDB()
	defer database.Close()

	// ... rest of code using database instead of db ...

	wpUser := "djwes487@gmail.com"
	wpPass := "ab4H TPEh vyrc 9lOL T91Z Zt5L"
	auth := base64.StdEncoding.EncodeToString([]byte(wpUser + ":" + wpPass))

	fmt.Println("🚀 Fetching Slack Config from Custom Endpoint...")

	url := "https://frontofficedynastysports.com/wp-json/fod/v1/slack-config"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Basic "+auth)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	
	leagueMap := map[string]string{
		"MLB":    "11111111-1111-1111-1111-111111111111",
		"AAA":    "22222222-2222-2222-2222-222222222222",
		"AA":     "33333333-3333-3333-3333-333333333333",
		"High A": "44444444-4444-4444-4444-444444444444",
	}

	var configs []SlackConfigRow
	if err := json.Unmarshal(body, &configs); err != nil {
		fmt.Println("❌ Failed to parse JSON:", err)
		fmt.Println("Response:", string(body))
		return
	}

	if len(configs) == 0 {
		fmt.Println("❌ No config found.")
		return
	}

	for _, row := range configs {
		leagueUUID := leagueMap[row.LeagueID]
		if leagueUUID == "" {
			continue
		}

		_, err := database.Exec(context.Background(), `
			INSERT INTO league_integrations (league_id, slack_bot_token, slack_channel_trade_block, slack_channel_transactions)
			VALUES ($1, $2, $3, $3)
			ON CONFLICT (league_id) DO UPDATE SET
				slack_bot_token = EXCLUDED.slack_bot_token,
				slack_channel_trade_block = EXCLUDED.slack_channel_trade_block,
				slack_channel_transactions = EXCLUDED.slack_channel_transactions;
		`, leagueUUID, row.BotToken, row.ChannelID)

		if err == nil {
			fmt.Printf("✅ Synced Slack Config for %s\n", row.LeagueID)
		} else {
			fmt.Printf("❌ Error syncing %s: %v\n", row.LeagueID, err)
		}
	}
}