package handlers

import (
	"net/http"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func FreeAgentHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		search := c.Query("q")
		pos := c.Query("pos")
		leagueID := c.Query("league_id")

		if leagueID == "" {
			leagueID = "11111111-1111-1111-1111-111111111111"
		}

		filter := store.PlayerSearchFilter{
			Search:   search,
			Position: pos,
			LeagueID: leagueID,
		}

		players, err := store.GetFreeAgents(db, filter)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error fetching players: %v", err)
			return
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "free_agents.html", gin.H{
			"Players":   players,
			"Search":    search,
			"Pos":       pos,
			"LeagueID":  leagueID,
			"Leagues":   leagues,
			"User":      user,
			"IsCommish": len(adminLeagues) > 0,
		})
	}
}

func PlayerProfileHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		player, err := store.GetPlayerByID(db, id)
		if err != nil {
			c.String(http.StatusNotFound, "Player not found")
			return
		}

		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		
		var teamOwnerID, teamName, teamID string
		db.QueryRow(c, "SELECT id, name, user_id FROM teams WHERE id = (SELECT team_id FROM players WHERE id = $1)", player.ID).Scan(&teamID, &teamName, &teamOwnerID)
		
		isRostered := teamID != ""
		isOwner := (isRostered && teamOwnerID == user.ID)

		RenderTemplate(c, "player_profile.html", gin.H{
			"Player":     player,
			"User":       user,
			"IsOwner":    isOwner,
			"IsRostered": isRostered,
			"TeamName":   teamName,
			"TeamID":     teamID,
			"IsCommish":  len(adminLeagues) > 0,
		})
	}
}
