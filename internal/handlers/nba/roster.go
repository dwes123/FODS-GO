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

		// Pre-compute capacity meter percentages (0-100, clamped). Templates can't divide.
		pct := func(n, limit int) int {
			if limit <= 0 {
				return 0
			}
			p := (n * 100) / limit
			if p > 100 {
				p = 100
			}
			return p
		}
		standardPct := pct(len(standard), sport.NBAStandardRosterLimit)
		twoWayPct := pct(len(twoWay), sport.NBATwoWayLimit)
		// Inactive has no fixed cap; treat 8 as "a lot" for visual purposes.
		inactivePct := pct(len(inactive), 8)

		handlers.RenderTemplate(c, "nba/roster.html", gin.H{
			"Sport":         sport.SportNBA,
			"User":          user,
			"IsCommish":     len(adminLeagues) > 0 || user.Role == "admin",
			"IsOwner":       isOwner,
			"Team":          team,
			"Standard":      standard,
			"TwoWay":        twoWay,
			"Inactive":      inactive,
			"StandardLimit": sport.NBAStandardRosterLimit,
			"TwoWayLimit":   sport.NBATwoWayLimit,
			"StandardPct":   standardPct,
			"TwoWayPct":     twoWayPct,
			"InactivePct":   inactivePct,
		})
	}
}
