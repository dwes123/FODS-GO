package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ArbitrationHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		targetYear := time.Now().Year() // Default to current year, could be param

		// Get user's teams
		myTeams, _ := store.GetManagedTeams(db, user.ID)
		
		// For MVP, just pick the first team or handle all. 
		// Ideally we'd have a league/team selector like the trade page.
		// Let's allow selecting a team via query param, default to first.
		selectedTeamID := c.Query("team_id")
		var selectedTeam store.TeamDetail
		
		if len(myTeams) > 0 {
			if selectedTeamID == "" {
				selectedTeam = myTeams[0]
			} else {
				for _, t := range myTeams {
					if t.ID == selectedTeamID {
						selectedTeam = t
						break
					}
				}
			}
		}

		var players []store.ArbitrationPlayer
		if selectedTeam.ID != "" {
			players, _ = store.GetArbitrationEligiblePlayers(db, selectedTeam.ID, targetYear)
		}

		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "arbitration.html", gin.H{
			"User":         user,
			"MyTeams":      myTeams,
			"SelectedTeam": selectedTeam,
			"TargetYear":   targetYear,
			"Players":      players,
			"IsCommish":    len(adminLeagues) > 0,
		})
	}
}

func SubmitArbitrationHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		playerID := c.PostForm("player_id")
		teamID := c.PostForm("team_id")
		leagueID := c.PostForm("league_id")
		year, _ := strconv.Atoi(c.PostForm("year"))
		
		decline := c.PostForm("decline") == "true"
		amount, _ := strconv.ParseFloat(c.PostForm("amount"), 64)

		err := store.SubmitArbitrationDecision(db, playerID, teamID, leagueID, year, amount, decline)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error: %v", err)
			return
		}

		c.Redirect(http.StatusFound, "/arbitration?team_id="+teamID)
	}
}

func SubmitArbExtensionHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		playerID := c.PostForm("player_id")
		teamID := c.PostForm("team_id")
		leagueID := c.PostForm("league_id")

		salaries := make(map[string]float64)
		for y := 2026; y <= 2030; y++ {
			val := c.PostForm(fmt.Sprintf("salary_%d", y))
			if val != "" {
				amt, _ := strconv.ParseFloat(val, 64)
				salaries[fmt.Sprintf("%d", y)] = amt
			}
		}

		err := store.SubmitExtension(db, playerID, teamID, leagueID, salaries)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error: %v", err)
			return
		}

		c.Redirect(http.StatusFound, "/arbitration?team_id="+teamID)
	}
}
