package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func SubmitBidHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		playerID := c.PostForm("player_id")
		yearsStr := c.PostForm("years")
		aavStr := c.PostForm("aav")
		isIFAForm := c.PostForm("ifa") == "1"

		years, _ := strconv.Atoi(yearsStr)
		aav, _ := strconv.ParseFloat(aavStr, 64)

		// Get player's league, then find user's team in that league via team_owners
		user := c.MustGet("user").(*store.User)

		var playerLeagueID string
		err := db.QueryRow(context.Background(),
			"SELECT league_id FROM players WHERE id = $1", playerID).Scan(&playerLeagueID)
		if err != nil {
			c.String(http.StatusBadRequest, "Player not found.")
			return
		}

		var teamID, teamName, leagueID string
		err = db.QueryRow(context.Background(),
			`SELECT t.id, t.name, t.league_id FROM teams t
			 JOIN team_owners to2 ON t.id = to2.team_id
			 WHERE to2.user_id = $1 AND t.league_id = $2 LIMIT 1`,
			user.ID, playerLeagueID).Scan(&teamID, &teamName, &leagueID)

		if err != nil {
			c.String(http.StatusBadRequest, "You do not own a team in this player's league.")
			return
		}

		// Check if player is actually IFA
		var isIFA bool
		db.QueryRow(context.Background(),
			"SELECT COALESCE(is_international_free_agent, FALSE) FROM players WHERE id = $1",
			playerID).Scan(&isIFA)

		if isIFA && isIFAForm {
			// --- IFA SIGNING (ISBP) ---
			signingBonus := aav
			if signingBonus <= 0 {
				c.String(http.StatusBadRequest, "Signing bonus must be greater than $0.")
				return
			}

			// Check IFA window
			if open, msg := store.IsWithinDateWindow(db, leagueID, time.Now().Year(), "ifa_window_open", "ifa_window_close"); !open {
				c.String(http.StatusForbidden, "IFA signing window is closed. "+msg)
				return
			}

			// Check ISBP balance
			var isbpBalance float64
			db.QueryRow(context.Background(),
				"SELECT COALESCE(isbp_balance, 0) FROM teams WHERE id = $1", teamID).Scan(&isbpBalance)
			if signingBonus > isbpBalance {
				c.String(http.StatusBadRequest, fmt.Sprintf("Insufficient ISBP balance. Signing bonus: $%.0f, Available: $%.0f", signingBonus, isbpBalance))
				return
			}

			// Check current bid — must outbid by any amount
			var currentAmount float64
			var currentStatus string
			db.QueryRow(context.Background(),
				"SELECT COALESCE(pending_bid_amount, 0), COALESCE(fa_status, '') FROM players WHERE id = $1",
				playerID).Scan(&currentAmount, &currentStatus)

			if currentStatus == "pending_bid" && signingBonus <= currentAmount {
				c.String(http.StatusBadRequest, fmt.Sprintf("Bid too low. Current bid is $%.0f. You must offer more.", currentAmount))
				return
			}

			// Submit IFA bid
			endTime := time.Now().Add(24 * time.Hour)
			_, err = db.Exec(context.Background(), `
				UPDATE players SET
					fa_status = 'pending_bid',
					pending_bid_amount = $1,
					pending_bid_years = 1,
					pending_bid_aav = $1,
					pending_bid_team_id = $2,
					pending_bid_manager_id = $3,
					bid_start_time = NOW(),
					bid_end_time = $4,
					bid_type = 'ifa'
				WHERE id = $5
			`, signingBonus, teamID, user.ID, endTime, playerID)

			if err != nil {
				fmt.Printf("ERROR [SubmitBid-IFA]: %v\n", err)
				c.String(http.StatusInternalServerError, "Failed to submit IFA bid")
				return
			}

			store.AppendBidHistory(db, playerID, teamID, signingBonus, 1, signingBonus)
			c.Redirect(http.StatusFound, "/player/"+playerID)
			return
		}

		// --- STANDARD FREE AGENT BIDDING ---

		// Contract year cap: 1-5 years only
		if years < 1 || years > 5 {
			c.String(http.StatusBadRequest, "Contract length must be between 1 and 5 years.")
			return
		}

		// AAV minimum: $1,000,000
		if aav < 1000000 {
			c.String(http.StatusBadRequest, "Minimum AAV is $1,000,000.")
			return
		}

		// Calculate Bid Points
		multipliers := map[int]float64{1: 2.0, 2: 1.8, 3: 1.6, 4: 1.4, 5: 1.2}
		multiplier := multipliers[years]
		bidPoints := (float64(years) * aav * multiplier) / 1000000

		// Minimum bid points: 1.0
		if bidPoints < 1.0 {
			c.String(http.StatusBadRequest, "Bid must be worth at least 1 bid point.")
			return
		}

		// Check for MiLB FA window
		var faStatus string
		db.QueryRow(context.Background(),
			"SELECT COALESCE(fa_status, '') FROM players WHERE id = $1", playerID).Scan(&faStatus)

		if faStatus == "milb_fa" {
			if open, msg := store.IsWithinDateWindow(db, leagueID, time.Now().Year(), "milb_fa_window_open", "milb_fa_window_close"); !open {
				c.String(http.StatusForbidden, "MiLB FA signing window is closed. "+msg)
				return
			}
		}

		// Check Current Bid
		var currentPoints float64
		var currentStatus, playerName string
		err = db.QueryRow(context.Background(),
			"SELECT COALESCE(pending_bid_amount, 0), fa_status, first_name || ' ' || last_name FROM players WHERE id = $1",
			playerID).Scan(&currentPoints, &currentStatus, &playerName)

		if currentStatus == "pending_bid" && bidPoints < currentPoints+1 {
			c.String(http.StatusBadRequest, "Bid too low. Must beat current bid by at least 1 point.")
			return
		}

		// Update Player with New Bid
		endTime := time.Now().Add(24 * time.Hour)

		_, err = db.Exec(context.Background(), `
			UPDATE players SET
				fa_status = 'pending_bid',
				pending_bid_amount = $1,
				pending_bid_years = $2,
				pending_bid_aav = $3,
				pending_bid_team_id = $4,
				pending_bid_manager_id = $5,
				bid_start_time = NOW(),
				bid_end_time = $6,
				bid_type = 'standard'
			WHERE id = $7
		`, bidPoints, years, aav, teamID, user.ID, endTime, playerID)

		if err != nil {
			fmt.Printf("ERROR [SubmitBid]: %v\n", err)
			c.String(http.StatusInternalServerError, "Failed to submit bid")
			return
		}

		// Append to bid_history JSONB
		store.AppendBidHistory(db, playerID, teamID, bidPoints, years, aav)

		// --- SLACK NOTIFICATION ---
		// msg := fmt.Sprintf("⚾ *New Bid!* %s has bid %.2f points on *%s* (%d years @ $%s AAV). Auction ends in 24 hours.", 
		// 	teamName, bidPoints, playerName, years, strconv.FormatFloat(aav, 'f', 0, 64))
		// notification.SendSlackNotification(db, leagueID, "transaction", msg)

		c.Redirect(http.StatusFound, "/player/"+playerID)
	}
}

func PendingBidsHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagueID := c.Query("league_id")

		pendingBids, err := store.GetPendingBids(db, leagueID)
		if err != nil {
			fmt.Printf("ERROR [PendingBids]: %v\n", err)
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "pending_bids.html", gin.H{
			"User":        user,
			"PendingBids": pendingBids,
			"Leagues":     leagues,
			"LeagueID":    leagueID,
			"IsCommish":   len(adminLeagues) > 0 || user.Role == "admin",
		})
	}
}

func BidHistoryHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		leagueID := c.Query("league_id")
		teamID := c.Query("team_id")

		records, _ := store.GetBidHistory(db, leagueID, teamID)
		leagues, _ := store.GetLeaguesWithTeams(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "bid_history.html", gin.H{
			"User":       user,
			"BidRecords": records,
			"Leagues":    leagues,
			"LeagueID":   leagueID,
			"TeamID":     teamID,
			"IsCommish":  len(adminLeagues) > 0,
		})
	}
}
