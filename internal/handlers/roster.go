package handlers

import (
	"net/http"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RosterHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		teamID := c.Param("id")

		team, err := store.GetTeamWithRoster(db, teamID)
		if err != nil {
			c.String(http.StatusNotFound, "Team not found")
			return
		}

		posOrder := []string{"C", "1B", "2B", "SS", "3B", "OF", "SP", "RP"}

		// Categorize players
		roster26 := make(map[string][]store.RosterPlayer)
		roster40 := make(map[string][]store.RosterPlayer)
		minors := make(map[string][]store.RosterPlayer)

		for _, p := range team.Players {
			if p.Status26Man {
				roster26[p.Position] = append(roster26[p.Position], p)
			} else if p.Status40Man {
				roster40[p.Position] = append(roster40[p.Position], p)
			} else {
				minors[p.Position] = append(minors[p.Position], p)
			}
		}

		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		data := gin.H{
			"Team":           team,
			"Roster26":       roster26,
			"Roster40":       roster40,
			"Minors":         minors,
			"PosOrder":       posOrder,
			"User":           user,
			"IsCommish":      len(adminLeagues) > 0,
		}

		RenderTemplate(c, "roster.html", data)
	}
}
