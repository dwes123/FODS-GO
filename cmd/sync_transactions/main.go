package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/db"
)

type WPTransaction struct {
	ID    int `json:"id"`
	Date  string `json:"date"`
	Title struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
}

func main() {
	database := db.InitDB()
	defer database.Close()

	teamLookup := make(map[string]string)
	tRows, _ := database.Query(context.Background(), "SELECT id, abbreviation FROM teams WHERE abbreviation IS NOT NULL")
	for tRows.Next() {
		var id, abbr string
		tRows.Scan(&id, &abbr)
		teamLookup[abbr] = id
	}
	tRows.Close()

	fmt.Println("🚀 Starting Enhanced Legacy Transaction Sync...")

	database.Exec(context.Background(), "DELETE FROM transactions")

	page := 1
	totalImported := 0

	for {
		url := "https://frontofficedynastysports.com/wp-json/wp/v2/transaction?per_page=100&page=" + fmt.Sprintf("%d", page)
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode >= 400 { break }

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var wpTrans []WPTransaction
		json.Unmarshal(body, &wpTrans)
		if len(wpTrans) == 0 { break }

		for _, t := range wpTrans {
			title := t.Title.Rendered
			tType := "COMMISSIONER"

			// Keyword Matching — valid types: ADD, DROP, TRADE, COMMISSIONER
			lowTitle := strings.ToLower(title)

			if strings.Contains(lowTitle, "trade") {
				tType = "TRADE"
			} else if strings.Contains(lowTitle, "signed") || strings.Contains(lowTitle, "add") || strings.Contains(lowTitle, "purchased") || strings.Contains(lowTitle, "claimed") {
				tType = "ADD"
			} else if strings.Contains(lowTitle, "dfa") || strings.Contains(lowTitle, "drop") || strings.Contains(lowTitle, "released") || strings.Contains(lowTitle, "waived") {
				tType = "DROP"
			} else if strings.Contains(lowTitle, "promoted") || strings.Contains(lowTitle, "optioned") || strings.Contains(lowTitle, "il") || strings.Contains(lowTitle, "move") || strings.Contains(lowTitle, "reinstated") {
				tType = "COMMISSIONER"
			}

			var teamUUID *string
			for abbr, uuid := range teamLookup {
				if strings.Contains(title, "by "+abbr) || strings.Contains(title, "to "+abbr) || strings.Contains(title, "from "+abbr) || strings.Contains(title, "for "+abbr) {
					id := uuid
					teamUUID = &id
					break
				}
			}

			createdAt, err := time.Parse("2006-01-02T15:04:05", t.Date)
			if err != nil {
				createdAt, err = time.Parse(time.RFC3339, t.Date)
			}
			if err != nil {
				createdAt = time.Now()
			}

			_, err = database.Exec(context.Background(), `
				INSERT INTO transactions (transaction_type, status, created_at, summary, team_id)
				VALUES ($1, $2, $3, $4, $5)
			`, tType, "COMPLETED", createdAt, title, teamUUID)

			if err == nil {
				totalImported++
			}
		}
		page++
		fmt.Printf("Page %d synced...\n", page-1)
	}

	fmt.Printf("\n🏆 Finished! Imported %d legacy transactions with improved categorization.\n", totalImported)
}