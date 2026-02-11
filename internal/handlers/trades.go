package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dwes123/fantasy-baseball-go/internal/notification"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TradeCenterHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
		
		myTeams, _ := store.GetManagedTeams(db, user.ID)
		var teamIDs []string
		for _, t := range myTeams {
			teamIDs = append(teamIDs, t.ID)
		}

		pendingTrades, _ := store.GetPendingTrades(db, teamIDs)

		RenderTemplate(c, "trades.html", gin.H{
			"User":          user,
			"MyTeams":       myTeams,
			"PendingTrades": pendingTrades,
			"IsCommish":     len(adminLeagues) > 0,
		})
	}
}

func NewTradeHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		targetTeamID := c.Query("team_id")
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		targetTeam, err := store.GetTeamWithRoster(db, targetTeamID)
		if err != nil {
			c.String(http.StatusNotFound, "Target team not found")
			return
		}

		allMyTeams, _ := store.GetManagedTeams(db, user.ID)
		var filteredTeams []store.TeamDetail
		for _, t := range allMyTeams {
			if t.LeagueID == targetTeam.LeagueID {
				filteredTeams = append(filteredTeams, t)
			}
		}

		if len(filteredTeams) == 0 {
			c.String(http.StatusForbidden, "You do not have a team in the " + targetTeam.Name + " league.")
			return
		}
		
		myTeamsJSON, _ := json.Marshal(filteredTeams)

		RenderTemplate(c, "trade_new.html", gin.H{
			"User":        user,
			"MyTeams":     filteredTeams,
			"MyTeamsJSON": string(myTeamsJSON),
			"TargetTeam":  targetTeam,
			"IsCommish":   len(adminLeagues) > 0,
		})
	}
}

func SubmitTradeHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		
		proposerID := c.PostForm("proposer_team_id")
		receiverID := c.PostForm("receiver_team_id")
		offered := c.PostFormArray("offered_players")
		requested := c.PostFormArray("requested_players")
		
		isbpOffered, _ := strconv.Atoi(c.PostForm("isbp_offered"))
		isbpRequested, _ := strconv.Atoi(c.PostForm("isbp_requested"))

		isOwner, _ := store.IsTeamOwner(db, proposerID, user.ID)
		if !isOwner {
			c.String(http.StatusForbidden, "Unauthorized")
			return
		}

		err := store.CreateTradeProposal(db, proposerID, receiverID, offered, requested, isbpOffered, isbpRequested)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error creating trade: %v", err)
			return
		}

		c.Redirect(http.StatusFound, "/trades")
	}
}

func AcceptTradeHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		tradeID := c.PostForm("trade_id")

		var pName, rName, lID string
		db.QueryRow(c, `
			SELECT tp.name, tr.name, t.league_id 
			FROM trades t 
			JOIN teams tp ON t.proposing_team_id = tp.id 
			JOIN teams tr ON t.receiving_team_id = tr.id 
			WHERE t.id = $1`, tradeID).Scan(&pName, &rName, &lID)

		err := store.AcceptTrade(db, tradeID, user.ID)
		if err != nil {
			c.String(http.StatusBadRequest, "Failed to accept trade: %v", err)
			return
		}

		msg := fmt.Sprintf("🤝 *TRADE COMPLETE!* The trade between *%s* and *%s* has been accepted and processed.", pName, rName)
		notification.SendSlackNotification(db, lID, "transaction", msg)

		c.Redirect(http.StatusFound, "/trades")
	}
}
