package handlers

import (
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func HomeHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Get All Leagues
		leagues, _ := store.GetLeaguesWithTeams(db)

		// 2. Get My Managed Teams
		var myTeams []store.TeamDetail
		var isCommish bool
		userVal, exists := c.Get("user")
		if exists {
			user := userVal.(*store.User)
			myTeams, _ = store.GetManagedTeams(db, user.ID)
			adminLeagues, _ := store.GetAdminLeagues(db, user.ID)
			isCommish = len(adminLeagues) > 0 || user.Role == "admin"
		}

		// 3. Get Recent Activity
		activities, _ := store.GetRecentActivity(db, 5, "")

		// 4. Get Upcoming Dates (Filtered by league query param)
		selectedLeague := c.Query("league")
		keyDates, _ := store.GetKeyDates(db, selectedLeague)

		data := gin.H{
			"Leagues":        leagues,
			"MyTeams":        myTeams,
			"User":           userVal,
			"IsCommish":      isCommish,
			"Activities":     activities,
			"KeyDates":       keyDates,
			"SelectedLeague": selectedLeague,
		}

		RenderTemplate(c, "home.html", data)
	}
}
