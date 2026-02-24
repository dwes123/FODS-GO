package handlers

import (
	"context"
	"fmt"
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
		ifaOnly := c.Query("ifa") == "1"

		if leagueID == "" {
			leagueID = "11111111-1111-1111-1111-111111111111"
		}

		filter := store.PlayerSearchFilter{
			Search:   search,
			Position: pos,
			LeagueID: leagueID,
			IFAOnly:  ifaOnly,
		}

		players, err := store.GetFreeAgents(db, filter)
		if err != nil {
			fmt.Printf("ERROR [FreeAgents]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "free_agents.html", gin.H{
			"Players":   players,
			"Search":    search,
			"Pos":       pos,
			"LeagueID":  leagueID,
			"IFA":       ifaOnly,
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

		// Also check team_owners table for multi-owner support
		isRostered := teamID != ""
		isOwner := false
		if isRostered {
			isOwner, _ = store.IsTeamOwner(db, teamID, user.ID)
		}

		bidHistory := store.GetPlayerBidHistory(db, id)
		playerDeadCap, _ := store.GetPlayerDeadCap(db, id)

		// Get user's team ISBP balance for IFA bidding
		var userIsbpBalance float64
		if !isRostered && player.IsIFA {
			var userTeamID string
			err := db.QueryRow(context.Background(),
				"SELECT t.id FROM teams t JOIN team_owners town ON t.id = town.team_id WHERE town.user_id = $1 AND t.league_id = $2 LIMIT 1",
				user.ID, player.LeagueID).Scan(&userTeamID)
			if err == nil {
				db.QueryRow(context.Background(),
					"SELECT COALESCE(isbp_balance, 0) FROM teams WHERE id = $1", userTeamID).Scan(&userIsbpBalance)
			}
		}

		RenderTemplate(c, "player_profile.html", gin.H{
			"Player":          player,
			"User":            user,
			"IsOwner":         isOwner,
			"IsRostered":      isRostered,
			"TeamName":        teamName,
			"TeamID":          teamID,
			"IsCommish":       len(adminLeagues) > 0,
			"BidHistory":      bidHistory,
			"DeadCap":         playerDeadCap,
			"IsbpBalance":     userIsbpBalance,
		})
	}
}

func TradeBlockHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagueID := c.Query("league_id")

		players, _ := store.GetTradeBlockPlayers(db, leagueID)
		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		// Group players by team
		type TeamGroup struct {
			TeamID   string
			TeamName string
			Players  []store.TradeBlockPlayer
		}
		teamMap := make(map[string]*TeamGroup)
		var teamOrder []string
		for _, p := range players {
			if _, ok := teamMap[p.TeamID]; !ok {
				teamMap[p.TeamID] = &TeamGroup{TeamID: p.TeamID, TeamName: p.TeamName}
				teamOrder = append(teamOrder, p.TeamID)
			}
			teamMap[p.TeamID].Players = append(teamMap[p.TeamID].Players, p)
		}
		var groups []TeamGroup
		for _, tid := range teamOrder {
			groups = append(groups, *teamMap[tid])
		}

		// Check which teams the current user owns
		ownedTeams := make(map[string]bool)
		ownedRows, _ := db.Query(context.Background(), "SELECT team_id FROM team_owners WHERE user_id = $1", user.ID)
		if ownedRows != nil {
			for ownedRows.Next() {
				var tid string
				ownedRows.Scan(&tid)
				ownedTeams[tid] = true
			}
			ownedRows.Close()
		}

		RenderTemplate(c, "trade_block.html", gin.H{
			"User":       user,
			"Groups":     groups,
			"Leagues":    leagues,
			"LeagueID":   leagueID,
			"OwnedTeams": ownedTeams,
			"IsCommish":  len(adminLeagues) > 0,
		})
	}
}
