package handlers

import (
	"github.com/gin-gonic/gin"
)

// PortalHandler renders the sport-picker landing page. Public — visible to
// anonymous visitors, but the "Enter" CTAs land on auth-gated routes that
// will bounce to /login when needed.
func PortalHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		userVal, _ := c.Get("user")
		RenderTemplate(c, "portal.html", gin.H{
			"Sport": "portal",
			"User":  userVal,
		})
	}
}
