package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WPPage struct {
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
}

func main() {
	dbUrl := "postgres://admin:password123@localhost:5433/fantasy_db"
	db, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	leagueMap := map[string]string{
		"MLB":    "11111111-1111-1111-1111-111111111111",
		"AAA":    "22222222-2222-2222-2222-222222222222",
		"AA":     "33333333-3333-3333-3333-333333333333",
		"High A": "44444444-4444-4444-4444-444444444444",
	}

	fmt.Println("🚀 Checking Legacy Home Page for League Dates...")

	resp, err := http.Get("https://frontofficedynastysports.com/wp-json/wp/v2/pages/26411")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var page WPPage
	json.Unmarshal(body, &page)

	html := page.Content.Rendered
	
	// Clear old dates
	db.Exec(context.Background(), "DELETE FROM key_dates")

	for lName, lID := range leagueMap {
		header := fmt.Sprintf("<h3>Key Dates (%s)</h3>", lName)
		if !strings.Contains(html, header) {
			fmt.Printf("⚠️  Header not found for %s\n", lName)
			continue
		}

		fmt.Printf("✅ Processing %s...\n", lName)
		
		start := strings.Index(html, header) + len(header)
		tableEnd := strings.Index(html[start:], "</table>")
		if tableEnd == -1 { continue }
		
		tableHtml := html[start : start+tableEnd]
		
		rows := strings.Split(tableHtml, "<tr>")
		rowCount := 0
		for _, row := range rows {
			if !strings.Contains(row, "<td>") { continue }
			cells := strings.Split(row, "<td>")
			if len(cells) < 3 { continue }
			
			date := strings.TrimSpace(strings.Split(cells[1], "</td>")[0])
			event := strings.TrimSpace(strings.Split(cells[2], "</td>")[0])
			
			if date != "" && event != "" && date != "Date" && event != "Event" {
				db.Exec(context.Background(), `
					INSERT INTO key_dates (league_id, event_date, event_name)
					VALUES ($1, $2, $3)
				`, lID, date, event)
				rowCount++
			}
		}
		fmt.Printf("   Found %d events for %s\n", rowCount, lName)
	}

	fmt.Println("🏆 Sync Complete!")
}
