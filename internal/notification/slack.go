package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SlackPayload struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

func SendSlackNotification(db *pgxpool.Pool, leagueID, notifyType, message string) error {
	ctx := context.Background()
	var token, channelID string

	column := "slack_channel_transactions"
	if notifyType == "trade_block" {
		column = "slack_channel_trade_block"
	}

	query := fmt.Sprintf("SELECT slack_bot_token, %s FROM league_integrations WHERE league_id = $1", column)
	err := db.QueryRow(ctx, query, leagueID).Scan(&token, &channelID)

	if err != nil || token == "" || channelID == "" {
		return nil // Not configured, skip
	}

	payload := SlackPayload{
		Channel: channelID,
		Text:    message,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("slack api error: %d", resp.StatusCode)
	}

	return nil
}
