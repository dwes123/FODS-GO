package nba

import (
	"github.com/dwes123/fantasy-baseball-go/internal/handlers"
	"github.com/dwes123/fantasy-baseball-go/internal/sport"
	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
)

// ComingSoonHandler renders a generic placeholder for NBA routes that are wired into the nav
// but not yet implemented (bids in Slice 3, trades/waivers in Slice 4, agent in Slice 5, etc.).
//
// Title and description are passed in so the same template can serve every stub route with
// distinct copy. Once a slice ships, swap the route over to its real handler.
func ComingSoonHandler(title, description string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userVal, _ := c.Get("user")

		var isCommish bool
		if u, ok := userVal.(*store.User); ok && u != nil && u.Role == "admin" {
			isCommish = true
		}

		handlers.RenderTemplate(c, "nba/coming_soon.html", gin.H{
			"Sport":       sport.SportNBA,
			"User":        userVal,
			"IsCommish":   isCommish,
			"PageTitle":   title,
			"Description": description,
		})
	}
}
