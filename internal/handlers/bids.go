package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dwes123/fantasy-baseball-go/internal/notification"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func SubmitBidHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		playerID := c.PostForm("player_id")
		yearsStr := c.PostForm("years")
		aavStr := c.PostForm("aav")

		years, _ := strconv.Atoi(yearsStr)
		aav, _ := strconv.ParseFloat(aavStr, 64)

		// 1. Calculate Bid Points
		multipliers := map[int]float64{1: 2.0, 2: 1.8, 3: 1.6, 4: 1.4, 5: 1.2}
		multiplier := multipliers[years]
		bidPoints := (float64(years) * aav * multiplier) / 1000000

		// 2. Get User's Team
		user := c.MustGet("user").(*store.User)
		
		var teamID, teamName, leagueID string
		err := db.QueryRow(context.Background(), 
			"SELECT id, name, league_id FROM teams WHERE user_id = $1 LIMIT 1", user.ID).Scan(&teamID, &teamName, &leagueID)
		
		if err != nil {
			c.String(http.StatusBadRequest, "You do not own a team and cannot bid.")
			return
		}

		// 3. Check Current Bid
		var currentPoints float64
		var currentStatus, playerName string
		err = db.QueryRow(context.Background(), 
			"SELECT COALESCE(pending_bid_amount, 0), fa_status, first_name || ' ' || last_name FROM players WHERE id = $1", 
			playerID).Scan(&currentPoints, &currentStatus, &playerName)

		if currentStatus == "pending_bid" && bidPoints < currentPoints+1 {
			c.String(http.StatusBadRequest, "Bid too low. Must beat current bid by at least 1 point.")
			return
		}

		// 4. Update Player with New Bid
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
			c.String(http.StatusInternalServerError, "Failed to submit bid: %v", err)
			return
		}

		// --- SLACK NOTIFICATION ---
		msg := fmt.Sprintf("⚾ *New Bid!* %s has bid %.2f points on *%s* (%d years @ $%s AAV). Auction ends in 24 hours.", 
			teamName, bidPoints, playerName, years, strconv.FormatFloat(aav, 'f', 0, 64))
		notification.SendSlackNotification(db, leagueID, "transaction", msg)

		c.Redirect(http.StatusFound, "/player/"+playerID)
	}
}
