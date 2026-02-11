package handlers

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strings"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CSV Importer
func AdminCSVImporterHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if user.Role != "admin" {
			c.String(http.StatusForbidden, "Admin Only")
			return
		}
		RenderTemplate(c, "admin_csv_importer.html", gin.H{"User": user, "IsCommish": true})
	}
}

func AdminProcessCSVHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, _, err := c.Request.FormFile("csv_file")
		if err != nil {
			c.String(http.StatusBadRequest, "Error uploading file")
			return
		}
		defer file.Close()

		reader := csv.NewReader(file)
		headers, _ := reader.Read()
		
		headerMap := make(map[string]int)
		for i, h := range headers {
			headerMap[strings.ToLower(strings.TrimSpace(h))] = i
		}

		records, _ := reader.ReadAll()
		count := 0
		for _, row := range records {
			firstName := row[headerMap["first_name"]]
			lastName := row[headerMap["last_name"]]
			pos := row[headerMap["position"]]
			mlb := row[headerMap["mlb_team"]]
			leagueID := row[headerMap["league_id"]]
			
			// Simple upsert by name + league
			_, err := db.Exec(context.Background(), `
				INSERT INTO players (first_name, last_name, position, mlb_team, league_id)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (first_name, last_name, league_id) DO UPDATE 
				SET position = EXCLUDED.position, mlb_team = EXCLUDED.mlb_team
			`, firstName, lastName, pos, mlb, leagueID)
			
			if err == nil {
				count++
			}
		}

		c.String(http.StatusOK, fmt.Sprintf("Successfully processed %d players", count))
	}
}

// Manual Player Assignment
func AdminPlayerAssignHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagues, _ := store.GetLeaguesWithTeams(db)
		RenderTemplate(c, "admin_player_assign.html", gin.H{"User": user, "Leagues": leagues, "IsCommish": true})
	}
}

func AdminProcessAssignHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		playerID := c.PostForm("player_id")
		teamID := c.PostForm("team_id")

		if teamID == "none" {
			_, err := db.Exec(context.Background(), "UPDATE players SET team_id = NULL WHERE id = $1", playerID)
			if err != nil {
				c.String(500, "Error: %v", err)
				return
			}
		} else {
			_, err := db.Exec(context.Background(), "UPDATE players SET team_id = $1 WHERE id = $2", teamID, playerID)
			if err != nil {
				c.String(500, "Error: %v", err)
				return
			}
		}
		c.Redirect(http.StatusFound, "/admin/player-assign?success=1")
	}
}

// Trade Review Queue
func AdminTradeReviewHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		
		rows, _ := db.Query(context.Background(), `
			SELECT t.id, tp.name as proposer, tr.name as receiver, t.status, t.created_at
			FROM trades t
			JOIN teams tp ON t.proposer_team_id = tp.id
			JOIN teams tr ON t.receiver_team_id = tr.id
			WHERE t.status = 'accepted'
			ORDER BY t.created_at DESC
		`)
		defer rows.Close()

		type TradeReview struct {
			ID       string
			Proposer string
			Receiver string
			Status   string
			Date     string
		}
		var trades []TradeReview
		for rows.Next() {
			var tr TradeReview
			var dt interface{}
			rows.Scan(&tr.ID, &tr.Proposer, &tr.Receiver, &tr.Status, &dt)
			trades = append(trades, tr)
		}

		RenderTemplate(c, "admin_trade_review.html", gin.H{"User": user, "Trades": trades, "IsCommish": true})
	}
}
