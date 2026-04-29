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

// MyTeamHandler renders the user's owned NBA team(s) with full contract detail.
//
// Behavior by ownership count:
//   0 teams:     empty-state page suggesting they contact the commissioner
//   1 team:      full roster of that team (mirrors RosterHandler output)
//   2+ teams:    full roster of the first team + a switcher card listing the others
//
// The ?team=<id> query param lets a multi-team owner pick which team to view.
func MyTeamHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
		isCommish := len(adminLeagues) > 0 || user.Role == "admin"

		ownedTeams, err := nbastore.GetManagedNBATeams(nbaDB, user.ID)
		if err != nil {
			fmt.Printf("ERROR [NBA MyTeam]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		// No teams owned: render the empty state.
		if len(ownedTeams) == 0 {
			handlers.RenderTemplate(c, "nba/my_team.html", gin.H{
				"Sport":     sport.SportNBA,
				"User":      user,
				"IsCommish": isCommish,
				"NoTeam":    true,
			})
			return
		}

		// Pick the active team. Default = first owned; ?team=<id> to switch.
		selectedID := c.Query("team")
		if selectedID == "" {
			selectedID = ownedTeams[0].ID
		}

		// Validate the selected team is actually owned by this user
		var active *nbastore.Team
		var others []nbastore.Team
		for i := range ownedTeams {
			if ownedTeams[i].ID == selectedID {
				active = &ownedTeams[i]
			} else {
				others = append(others, ownedTeams[i])
			}
		}
		if active == nil {
			// Fallback: if ?team=<id> didn't match owned, use first
			active = &ownedTeams[0]
			others = ownedTeams[1:]
		}

		// Hydrate the active team with its roster
		fullTeam, err := nbastore.GetTeamWithRoster(nbaDB, active.ID)
		if err != nil {
			fmt.Printf("ERROR [NBA MyTeam roster]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		// Bucket players (same logic as RosterHandler)
		var standard, twoWay, gleague, inactive []nbastore.Player
		for _, p := range fullTeam.Roster {
			switch {
			case p.OnTwoWay:
				twoWay = append(twoWay, p)
			case p.OnGLeague:
				gleague = append(gleague, p)
			case !p.OnActiveRoster:
				inactive = append(inactive, p)
			default:
				standard = append(standard, p)
			}
		}

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

		handlers.RenderTemplate(c, "nba/my_team.html", gin.H{
			"Sport":         sport.SportNBA,
			"User":          user,
			"IsCommish":     isCommish,
			"NoTeam":        false,
			"Team":          fullTeam,
			"OtherTeams":    others,
			"OwnedCount":    len(ownedTeams),
			"Standard":      standard,
			"TwoWay":        twoWay,
			"GLeague":       gleague,
			"Inactive":      inactive,
			"StandardLimit": sport.NBAStandardRosterLimit,
			"TwoWayLimit":   sport.NBATwoWayLimit,
			"GLeagueLimit":  10,
			"StandardPct":   pct(len(standard)+len(twoWay), sport.NBAStandardRosterLimit),
			"TwoWayPct":     pct(len(twoWay), sport.NBATwoWayLimit),
			"GLeaguePct":    pct(len(gleague), 10),
			"InactivePct":   pct(len(inactive), 8),
		})
	}
}
