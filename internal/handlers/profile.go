package handlers

import (
	"fmt"
	"net/http"

	"github.com/dwes123/fantasy-baseball-go/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// ...

func UpdatePasswordHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		newPassword := c.PostForm("new_password")
		confirmPassword := c.PostForm("confirm_password")

		if newPassword == "" || newPassword != confirmPassword {
			c.String(http.StatusBadRequest, "Passwords must match and cannot be empty")
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to hash password")
			return
		}

		err = store.UpdateUserPassword(db, user.ID, string(hashedPassword))
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to update password")
			return
		}

		c.Redirect(http.StatusFound, "/profile?success=password_updated")
	}
}

func ProfileHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)

		// Get My Teams
		myTeams, err := store.GetManagedTeams(db, user.ID)
		if err != nil {
			fmt.Printf("ERROR [Profile]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		// Get Available Teams
		availableTeams, err := store.GetUnassignedTeams(db)
		if err != nil {
			fmt.Printf("ERROR [Profile]: %v\n", err)
			c.String(http.StatusInternalServerError, "Internal server error")
			return
		}

		adminLeagues, _ := store.GetAdminLeagues(db, user.ID)

		RenderTemplate(c, "profile.html", gin.H{
			"User":           user,
			"MyTeams":        myTeams,
			"AvailableTeams": availableTeams,
			"IsCommish":      len(adminLeagues) > 0,
		})
	}
}

func ClaimTeamHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(*store.User)
		teamID := c.PostForm("team_id")

		if teamID == "" {
			c.String(http.StatusBadRequest, "Team ID is required")
			return
		}

		err := store.ClaimTeam(db, teamID, user.ID, user.Username)
		if err != nil {
			c.String(http.StatusBadRequest, "Failed to claim team. It may already be owned.")
			return
		}

		c.Redirect(http.StatusFound, "/profile")
	}
}
