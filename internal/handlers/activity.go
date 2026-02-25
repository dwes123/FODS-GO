package handlers

import (
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ActivityFeedHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("user")
		leagueID := c.Query("league_id")
		transactionType := c.Query("transaction_type")

		activities, err := store.GetTransactionLog(db, 500, leagueID, transactionType)
		if err != nil {
			c.String(500, "Error fetching activity")
			return
		}

		leagues, _ := store.GetLeaguesWithTeams(db)
		transactionTypes, _ := store.GetDistinctTransactionTypes(db)
		adminLeagues, _ := store.GetAdminLeagues(db, user.(*store.User).ID)

		RenderTemplate(c, "activity.html", gin.H{
			"User":              user,
			"Activities":        activities,
			"Leagues":           leagues,
			"SelectedLID":       leagueID,
			"SelectedType":      transactionType,
			"TransactionTypes":  transactionTypes,
			"IsCommish":         len(adminLeagues) > 0,
		})
	}
}
