package handlers

import (
	"context"
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
			"MyTeamIDs":     teamIDs,
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
				// Load full roster for each team so JS can display players
				fullTeam, err := store.GetTeamWithRoster(db, t.ID)
				if err == nil {
					filteredTeams = append(filteredTeams, *fullTeam)
				} else {
					filteredTeams = append(filteredTeams, t)
				}
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

		// Check trade deadline
		var leagueID string
		db.QueryRow(c, `SELECT league_id FROM teams WHERE id = $1`, proposerID).Scan(&leagueID)
		if open, msg := store.IsTradeWindowOpen(db, leagueID); !open {
			c.String(http.StatusForbidden, msg)
			return
		}

		err := store.CreateTradeProposal(db, proposerID, receiverID, offered, requested, isbpOffered, isbpRequested)
		if err != nil {
			fmt.Printf("ERROR [SubmitTrade]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		// Email notification to receiving team
		go func() {
			var proposerName, receiverName string
			db.QueryRow(context.Background(), `SELECT name FROM teams WHERE id = $1`, proposerID).Scan(&proposerName)
			db.QueryRow(context.Background(), `SELECT name FROM teams WHERE id = $1`, receiverID).Scan(&receiverName)
			emails, _ := store.GetTeamOwnerEmails(db, receiverID)
			for _, email := range emails {
				body := fmt.Sprintf("<h2>New Trade Proposal</h2><p><strong>%s</strong> has sent a trade proposal to <strong>%s</strong>.</p><p><a href=\"https://frontofficedynastysports.com/trades\">View Trade</a></p>", proposerName, receiverName)
				notification.SendEmail(email, "New Trade Proposal from "+proposerName, body)
			}
		}()

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
			fmt.Printf("ERROR [AcceptTrade]: %v\n", err)
			c.String(http.StatusBadRequest, "Failed to accept trade")
			return
		}

		msg := fmt.Sprintf("🤝 *TRADE COMPLETE!* The trade between *%s* and *%s* has been accepted and processed.", pName, rName)
		notification.SendSlackNotification(db, lID, "transaction", msg)

		c.Redirect(http.StatusFound, "/trades")
	}
}

func RejectTradeHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		tradeID := c.PostForm("trade_id")

		// Verify the user owns one of the teams in this trade
		var proposingTeamID, receivingTeamID string
		err := db.QueryRow(context.Background(),
			`SELECT proposing_team_id, receiving_team_id FROM trades WHERE id = $1 AND status = 'PROPOSED'`,
			tradeID).Scan(&proposingTeamID, &receivingTeamID)
		if err != nil {
			c.String(http.StatusNotFound, "Trade not found")
			return
		}

		isProposer, _ := store.IsTeamOwner(db, proposingTeamID, user.ID)
		isReceiver, _ := store.IsTeamOwner(db, receivingTeamID, user.ID)
		if !isProposer && !isReceiver {
			c.String(http.StatusForbidden, "You are not part of this trade")
			return
		}

		_, err = db.Exec(context.Background(),
			`UPDATE trades SET status = 'REJECTED' WHERE id = $1 AND status = 'PROPOSED'`, tradeID)
		if err != nil {
			fmt.Printf("ERROR [RejectTrade]: %v\n", err)
			c.String(http.StatusInternalServerError, "Failed to reject trade")
			return
		}

		c.Redirect(http.StatusFound, "/trades")
	}
}
