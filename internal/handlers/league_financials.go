package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func LeagueFinancialsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagueID := c.Query("league_id")
		yearStr := c.Query("year")

		year, err := strconv.Atoi(yearStr)
		if err != nil {
			year = 2026 // Default year
		}

		if leagueID == "" {
			// Try to find user's team league
			db.QueryRow(context.Background(), "SELECT league_id FROM teams WHERE user_id = $1 LIMIT 1", user.ID).Scan(&leagueID)
		}
		if leagueID == "" {
			leagueID = "11111111-1111-1111-1111-111111111111" // Default MLB
		}

		var leagueName string
		db.QueryRow(context.Background(), "SELECT name FROM leagues WHERE id = $1", leagueID).Scan(&leagueName)

		// Fetch all teams in the league — collect first, then close rows
		// to avoid holding a connection while making nested queries
		rows, err := db.Query(context.Background(), "SELECT id, name, COALESCE(owner_name, '') FROM teams WHERE league_id = $1 ORDER BY name", leagueID)
		if err != nil {
			c.String(http.StatusInternalServerError, "Database error: %v", err)
			return
		}

		type TeamFinancialRow struct {
			TeamID   string
			TeamName string
			Owner    string
			Summary  store.SalaryYearSummary
		}

		var teamRows []TeamFinancialRow
		for rows.Next() {
			var tr TeamFinancialRow
			if err := rows.Scan(&tr.TeamID, &tr.TeamName, &tr.Owner); err == nil {
				teamRows = append(teamRows, tr)
			}
		}
		rows.Close()

		// Now calculate summaries without holding the outer connection
		for i := range teamRows {
			teamRows[i].Summary = store.CalculateYearlySummary(db, teamRows[i].TeamID, leagueID, year)
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "league_financials.html", gin.H{
			"User":       user,
			"TeamRows":   teamRows,
			"Leagues":    leagues,
			"LeagueID":   leagueID,
			"LeagueName": leagueName,
			"Year":       year,
			"IsCommish":  len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}
