package middleware

import (
	"net/http"
	"os"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func AuthMiddleware(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie("session_token")
		if err != nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		user, err := store.GetUserBySessionToken(db, token)
		if err != nil || user == nil {
			secure := os.Getenv("GIN_MODE") == "release"
			domain := ""
			if secure {
				domain = "frontofficedynastysports.com"
			}
			c.SetCookie("session_token", "", -1, "/", domain, secure, true)
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}

		// Store user in context for handlers to access
		c.Set("user", user)
		c.Next()
	}
}
