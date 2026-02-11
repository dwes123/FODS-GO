package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RotationsDashboardHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		
		// 1. Setup Week and League
		leagueID := c.Query("league_id")
		if leagueID == "" {
			leagueID = "11111111-1111-1111-1111-111111111111" // Default MLB
		}

		year, week := time.Now().ISOWeek()
		currentWeek := fmt.Sprintf("%d-%02d", year, week)
		
		selectedWeek := c.Query("week")
		if selectedWeek == "" {
			selectedWeek = currentWeek
		}

		// 2. Fetch Data
		submissions, _ := store.GetWeeklyRotations(db, leagueID, selectedWeek)
		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "rotations.html", gin.H{
			"User":         user,
			"Submissions":  submissions,
			"Leagues":      leagues,
			"SelectedLID":  leagueID,
			"SelectedWeek": selectedWeek,
			"CurrentWeek":  currentWeek,
			"Days":         []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
			"IsCommish":    len(adminLeagues) > 0,
		})
	}
}

func RotationsSubmitPageHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		
		myTeams, _ := store.GetManagedTeams(db, user.ID)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "rotations_submit.html", gin.H{
			"User":      user,
			"MyTeams":   myTeams,
			"Days":      []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"},
			"IsCommish": len(adminLeagues) > 0,
		})
	}
}

func SubmitRotationHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		
		teamID := c.PostForm("team_id")
		day := c.PostForm("day")
		p1ID := c.PostForm("p1_id")
		p1Date := c.PostForm("p1_date")
		p2ID := c.PostForm("p2_id")
		p2Date := c.PostForm("p2_date")

		// Verify ownership
		isOwner, _ := store.IsTeamOwner(db, teamID, user.ID)
		if !isOwner {
			c.String(http.StatusForbidden, "Unauthorized")
			return
		}

		// Get league ID for the team
		var leagueID string
		db.QueryRow(c, "SELECT league_id FROM teams WHERE id = $1", teamID).Scan(&leagueID)

		year, week := time.Now().ISOWeek()
		weekStr := fmt.Sprintf("%d-%02d", year, week)

		err := store.SubmitRotation(db, teamID, leagueID, weekStr, day, p1ID, p1Date, p2ID, p2Date)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error: %v", err)
			return
		}

		c.Redirect(http.StatusFound, "/rotations")
	}
}

func GetTeamPitchersHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID := c.Query("team_id")
		pitchers, err := store.GetPitchersForTeam(db, teamID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, pitchers)
	}
}
