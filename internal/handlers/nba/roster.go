package nba

import (
	"fmt"
	"net/http"

	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/sport"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	nbastore "github.com/dwes123/fantasy-baseball-go/internal/store/nba"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RosterHandler renders a single team's roster, split into standard / two-way / inactive sections.
func RosterHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		teamID := c.Param("id")

		team, err := nbastore.GetTeamWithRoster(nbaDB, teamID)
		if err != nil {
			fmt.Printf("ERROR [NBA Roster]: %v\n", err)
			c.String(http.StatusNotFound, "Team not found")
			return
		}

		isOwner, _ := nbastore.IsTeamOwner(nbaDB, teamID, user.ID)
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)

		// Bucket players for template-side sectioning
		var standard, twoWay, inactive []nbastore.Player
		for _, p := range team.Roster {
			switch {
			case p.OnTwoWay:
				twoWay = append(twoWay, p)
			case !p.OnActiveRoster:
				inactive = append(inactive, p)
			default:
				standard = append(standard, p)
			}
		}

		handlers.RenderTemplate(c, "nba/roster.html", gin.H{
			"Sport":     sport.SportNBA,
			"User":      user,
			"IsCommish": len(adminLeagues) > 0 || user.Role == "admin",
			"IsOwner":   isOwner,
			"Team":      team,
			"Standard":  standard,
			"TwoWay":    twoWay,
			"Inactive":  inactive,
			"StandardLimit": sport.NBAStandardRosterLimit,
			"TwoWayLimit":   sport.NBATwoWayLimit,
		})
	}
}
