package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FantraxStanding struct {
	Rank          int     `json:"rank"`
	TeamName      string  `json:"teamName"`
	WinPercentage float64 `json:"winPercentage"`
	GamesBack     float64 `json:"gamesBack"`
}

func StandingsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagueID := c.Query("league_id")

		if leagueID == "" {
			// Try to find user's team league
			db.QueryRow(context.Background(), "SELECT t.league_id FROM teams t JOIN team_owners to2 ON t.id = to2.team_id WHERE to2.user_id = $1 LIMIT 1", user.ID).Scan(&leagueID)
		}
		if leagueID == "" {
			leagueID = "11111111-1111-1111-1111-111111111111" // Default MLB
		}

		var url string
		var leagueName string
		db.QueryRow(context.Background(), "SELECT name, fantrax_url FROM leagues WHERE id = $1", leagueID).Scan(&leagueName, &url)

		var standings []FantraxStanding
		if url != "" {
			client := &http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(url)
			if err == nil {
				defer resp.Body.Close()
				json.NewDecoder(resp.Body).Decode(&standings)
			}
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "standings.html", gin.H{
			"User":        user,
			"Standings":   standings,
			"Leagues":     leagues,
			"LeagueID":    leagueID,
			"LeagueName":  leagueName,
			"IsCommish":   len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}
