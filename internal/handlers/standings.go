package handlers

import (
	"context"
	"fmt"

	"github.com/dwes123/fantasy-baseball-go/internal/fantrax"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func StandingsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagueID := c.Query("league_id")

		if leagueID == "" {
			db.QueryRow(context.Background(), "SELECT t.league_id FROM teams t JOIN team_owners to2 ON t.id = to2.team_id WHERE to2.user_id = $1 LIMIT 1", user.ID).Scan(&leagueID)
		}
		if leagueID == "" {
			leagueID = "11111111-1111-1111-1111-111111111111" // Default MLB
		}

		var url string
		var leagueName string
		db.QueryRow(context.Background(), "SELECT name, fantrax_url FROM leagues WHERE id = $1", leagueID).Scan(&leagueName, &url)

		var standings []fantrax.Standing
		var errMsg string
		switch {
		case url == "":
			errMsg = "This league is not linked to Fantrax yet. Ask a commissioner to configure it."
		default:
			s, err := fantrax.Fetch(url)
			if err != nil {
				fmt.Printf("ERROR [StandingsHandler]: %v\n", err)
				errMsg = "Couldn't reach Fantrax right now. Try again in a few minutes."
			} else if len(s) == 0 {
				errMsg = "No standings data available yet for this league."
			} else {
				standings = s
			}
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "standings.html", gin.H{
			"User":       user,
			"Standings":  standings,
			"Leagues":    leagues,
			"LeagueID":   leagueID,
			"LeagueName": leagueName,
			"IsCommish":  len(adminLeagues) > 0 || user.Role == "admin",
			"Error":      errMsg,
		})
	}
}
