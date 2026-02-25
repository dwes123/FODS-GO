package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/dwes123/fantasy-baseball-go/internal/worker"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StatsLeaderboardHandler renders the pitching leaderboard page.
func StatsLeaderboardHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)

		leagueID := c.Query("league")
		startDate := c.Query("start")
		endDate := c.Query("end")

		// Default to current season: Apr 1 to today
		now := time.Now()
		if startDate == "" {
			startDate = fmt.Sprintf("%d-04-01", now.Year())
		}
		if endDate == "" {
			endDate = now.Format("2006-01-02")
		}

		leaders, err := store.GetPitchingLeaderboard(db, leagueID, startDate, endDate, 100)
		if err != nil {
			fmt.Printf("ERROR [StatsLeaderboardHandler]: %v\n", err)
			leaders = []store.StatsLeaderEntry{}
		}

		leagues, _ := store.GetLeaguesWithTeams(db)

		categories, _ := store.GetScoringCategories(db, "pitching")

		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "stats_leaderboard.html", gin.H{
			"User":           user,
			"Leaders":        leaders,
			"Leagues":        leagues,
			"SelectedLeague": leagueID,
			"StartDate":      startDate,
			"EndDate":        endDate,
			"Categories":     categories,
			"IsCommish":      len(adminLeagues) > 0,
		})
	}
}

// HittingLeaderboardHandler renders the hitting leaderboard page.
func HittingLeaderboardHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)

		leagueID := c.Query("league")
		startDate := c.Query("start")
		endDate := c.Query("end")

		now := time.Now()
		if startDate == "" {
			startDate = fmt.Sprintf("%d-04-01", now.Year())
		}
		if endDate == "" {
			endDate = now.Format("2006-01-02")
		}

		leaders, err := store.GetHittingLeaderboard(db, leagueID, startDate, endDate, 100)
		if err != nil {
			fmt.Printf("ERROR [HittingLeaderboardHandler]: %v\n", err)
			leaders = []store.HittingLeaderEntry{}
		}

		leagues, _ := store.GetLeaguesWithTeams(db)

		categories, _ := store.GetScoringCategories(db, "hitting")

		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "stats_hitting_leaderboard.html", gin.H{
			"User":           user,
			"Leaders":        leaders,
			"Leagues":        leagues,
			"SelectedLeague": leagueID,
			"StartDate":      startDate,
			"EndDate":        endDate,
			"Categories":     categories,
			"IsCommish":      len(adminLeagues) > 0,
		})
	}
}

// PlayerGameLogHandler returns JSON game log for a player (AJAX).
func PlayerGameLogHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		playerID := c.Param("id")
		statType := c.DefaultQuery("type", "pitching")

		logs, err := store.GetPlayerGameLog(db, playerID, statType, 50)
		if err != nil {
			fmt.Printf("ERROR [PlayerGameLogHandler]: %v\n", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load game log"})
			return
		}
		if logs == nil {
			logs = []store.DailyPlayerStats{}
		}

		c.JSON(http.StatusOK, gin.H{"games": logs})
	}
}

// AdminBackfillStatsHandler triggers stats processing for a specific date (admin only).
func AdminBackfillStatsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		if user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin only"})
			return
		}

		date := c.PostForm("date")
		if date == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Date is required"})
			return
		}

		// Validate date format
		if _, err := time.Parse("2006-01-02", date); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format (use YYYY-MM-DD)"})
			return
		}

		// Run in background goroutine with detached context (request context cancels on response)
		go worker.ProcessDateStats(context.Background(), db, date)

		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Backfill started for %s (pitching + hitting)", date)})
	}
}
