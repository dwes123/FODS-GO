package handlers

import (
	"net/http"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TeamFinancialsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID := c.Param("id")
		user := c.MustGet("user").(*store.User)

		team, err := store.GetTeamWithRoster(db, teamID)
		if err != nil {
			c.String(http.StatusNotFound, "Team not found")
			return
		}

		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "team_financials.html", gin.H{
			"User":      user,
			"Team":      team,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}
