package nba

import (
	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/sport"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HomeHandler renders the NBA-side home page.
//
// userDB: baseball/auth pool (fantasy_db) — used to load the current user / commissioner role.
// nbaDB:  basketball pool (fantasy_basketball_db) — reserved for Slice 2+ when this page
//         starts showing real teams, activity, etc.
//
// Slice 1 keeps this minimal: a placeholder welcome page so the route resolves end-to-end.
func HomeHandler(userDB, nbaDB *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		userVal, _ := c.Get("user")

		var isCommish bool
		if user, ok := userVal.(*store.User); ok && user != nil {
			adminLeagues, _ := store.GetAdminLeaguesForSport(userDB, user.ID, sport.SportNBA)
			isCommish = len(adminLeagues) > 0 || user.Role == "admin"
		}

		data := gin.H{
			"User":      userVal,
			"Sport":     sport.SportNBA,
			"IsCommish": isCommish,
		}

		handlers.RenderTemplate(c, "nba/home.html", data)
	}
}
