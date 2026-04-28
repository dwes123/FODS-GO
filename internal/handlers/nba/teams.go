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

// LeagueRostersHandler renders an at-a-glance view of every NBA team's roster,
// grouped by conference → division.
//
// Two queries: list teams, then one roster fetch per team. Acceptable up to 30 teams.
func LeagueRostersHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)

		teams, err := nbastore.ListTeams(nbaDB)
		if err != nil {
			fmt.Printf("ERROR [NBA LeagueRosters]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		// Hydrate each team with its roster. Iteration is sequential (no nested rows).
		for i := range teams {
			roster, err := nbastore.GetTeamRoster(nbaDB, teams[i].ID)
			if err != nil {
				fmt.Printf("ERROR [NBA roster fetch %s]: %v\n", teams[i].ID, err)
				continue
			}
			teams[i].Roster = roster
		}

		// Bucket: conference → division → teams. Use canonical orderings so the page
		// always renders East/West top-down with divisions in the same sequence.
		type divisionGroup struct {
			Name  string
			Teams []nbastore.Team
		}
		type confGroup struct {
			Name      string
			Divisions []divisionGroup
		}
		confOrder := []string{"East", "West"}
		divOrder := map[string][]string{
			"East": {"Atlantic", "Central", "Southeast"},
			"West": {"Northwest", "Pacific", "Southwest"},
		}

		bucket := map[string]map[string][]nbastore.Team{}
		var unassigned []nbastore.Team
		for _, t := range teams {
			if t.Conference == "" || t.Division == "" {
				unassigned = append(unassigned, t)
				continue
			}
			if _, ok := bucket[t.Conference]; !ok {
				bucket[t.Conference] = map[string][]nbastore.Team{}
			}
			bucket[t.Conference][t.Division] = append(bucket[t.Conference][t.Division], t)
		}

		var conferences []confGroup
		for _, conf := range confOrder {
			byDiv, ok := bucket[conf]
			if !ok {
				continue
			}
			cg := confGroup{Name: conf}
			for _, div := range divOrder[conf] {
				if ts := byDiv[div]; len(ts) > 0 {
					cg.Divisions = append(cg.Divisions, divisionGroup{Name: div, Teams: ts})
				}
			}
			conferences = append(conferences, cg)
		}

		handlers.RenderTemplate(c, "nba/league_rosters.html", gin.H{
			"Sport":       sport.SportNBA,
			"User":        user,
			"IsCommish":   len(adminLeagues) > 0 || user.Role == "admin",
			"Conferences": conferences,
			"Unassigned":  unassigned,
			"TotalTeams":  len(teams),
		})
	}
}
