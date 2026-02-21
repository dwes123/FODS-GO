package handlers

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
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
		user := c.MustGet("user").(*store.User)
		if user.Role != "admin" {
			c.String(http.StatusForbidden, "Admin Only")
			return
		}

		// Limit upload to 5 MB
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 5<<20)

		file, _, err := c.Request.FormFile("csv_file")
		if err != nil {
			c.String(http.StatusBadRequest, "Error uploading file")
			return
		}
		defer file.Close()

		reader := csv.NewReader(file)
		headers, err := reader.Read()
		if err != nil {
			c.String(http.StatusBadRequest, "Error reading CSV headers")
			return
		}

		headerMap := make(map[string]int)
		for i, h := range headers {
			headerMap[strings.ToLower(strings.TrimSpace(h))] = i
		}

		// Validate required columns
		required := []string{"first_name", "last_name", "position", "mlb_team", "league_id"}
		for _, col := range required {
			if _, ok := headerMap[col]; !ok {
				c.String(http.StatusBadRequest, "Missing required column: %s", col)
				return
			}
		}

		records, err := reader.ReadAll()
		if err != nil {
			c.String(http.StatusBadRequest, "Error reading CSV data")
			return
		}

		count := 0
		for _, row := range records {
			if len(row) <= headerMap["league_id"] {
				continue
			}
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
		searchQuery := c.Query("q")
		searchResults := []store.RosterPlayer{}
		if searchQuery != "" {
			searchResults, _ = store.SearchAllPlayers(db, searchQuery)
		}
		RenderTemplate(c, "admin_player_assign.html", gin.H{
			"User":          user,
			"Leagues":       leagues,
			"IsCommish":     true,
			"SearchQuery":   searchQuery,
			"SearchResults": searchResults,
		})
	}
}

func AdminProcessAssignHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		playerID := c.PostForm("player_id")
		teamID := c.PostForm("team_id")

		if teamID == "none" {
			_, err := db.Exec(context.Background(), "UPDATE players SET team_id = NULL WHERE id = $1", playerID)
			if err != nil {
				fmt.Printf("ERROR [AdminProcessAssign]: %v\n", err)
				c.String(http.StatusInternalServerError, "Internal server error")
				return
			}
		} else {
			_, err := db.Exec(context.Background(), "UPDATE players SET team_id = $1 WHERE id = $2", teamID, playerID)
			if err != nil {
				fmt.Printf("ERROR [AdminProcessAssign]: %v\n", err)
				c.String(http.StatusInternalServerError, "Internal server error")
				return
			}
		}
		c.Redirect(http.StatusFound, "/admin/player-assign?success=1")
	}
}

// --- Trade Reversal (Feature 5) ---
func AdminReverseTradeHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}
		tradeID := c.PostForm("trade_id")
		err := store.ReverseTrade(db, tradeID)
		if err != nil {
			fmt.Printf("ERROR [AdminReverseTrade]: %v\n", err)
			c.String(http.StatusBadRequest, "Error reversing trade")
			return
		}
		c.Redirect(http.StatusFound, "/admin/trades")
	}
}

// --- Fantrax Toggle (Feature 6) ---
func ToggleFantraxHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Commissioner Only"})
			return
		}
		id := c.PostForm("transaction_id")
		_, err := db.Exec(context.Background(),
			"UPDATE transactions SET fantrax_processed = NOT COALESCE(fantrax_processed, FALSE) WHERE id = $1", id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Toggled"})
	}
}

// --- Bid Export CSV (Feature 11) ---
func BidExportHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}
		leagueID := c.Query("league_id")
		teamID := c.Query("team_id")

		records, err := store.GetBidHistory(db, leagueID, teamID)
		if err != nil {
			fmt.Printf("ERROR [BidExport]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", "attachment; filename=bid_history.csv")

		writer := csv.NewWriter(c.Writer)
		writer.Write([]string{"Player", "League", "Team", "Bid Points", "Years", "AAV", "Date"})
		for _, r := range records {
			writer.Write([]string{
				r.PlayerName, r.LeagueName, r.TeamName,
				fmt.Sprintf("%.2f", r.Amount),
				strconv.Itoa(r.Years),
				fmt.Sprintf("%.0f", r.AAV),
				r.BidDate,
			})
		}
		writer.Flush()
	}
}

// --- Fantrax Queue ---
func AdminFantraxQueueHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		if len(adminLeagues) == 0 && user.Role != "admin" {
			c.String(http.StatusForbidden, "Commissioner Only")
			return
		}

		leagueID := c.Query("league_id")
		showCompleted := c.Query("show_completed") == "1"

		queue, err := store.GetFantraxQueue(db, leagueID, showCompleted)
		if err != nil {
			fmt.Printf("ERROR [AdminFantraxQueue]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		leagues, _ := store.GetLeaguesWithTeams(db)

		RenderTemplate(c, "admin_fantrax_queue.html", gin.H{
			"User":          user,
			"IsCommish":     true,
			"Queue":         queue,
			"Leagues":       leagues,
			"SelectedLeague": leagueID,
			"ShowCompleted": showCompleted,
		})
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
